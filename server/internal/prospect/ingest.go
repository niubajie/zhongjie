package prospect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"housing/internal/secure"
)

// 批次 3：采集入库编排。
// 职责：按源限速拉取 → 联系方式哈希与去重 → opt_out 过滤 → 原文加密 → 写入 status=new。

// LeadStore 是 Ingestor 依赖的存储能力（*Repository 实现）。
type LeadStore interface {
	ListEnabledSources(ctx context.Context) ([]LeadSource, error)
	ListDueSources(ctx context.Context, now time.Time) ([]LeadSource, error)
	GetSetting(ctx context.Context, key string) (string, error)
	OptOutExists(ctx context.Context, contactHash []byte, leadType string) (bool, error)
	LeadExistsByContactHash(ctx context.Context, contactHash []byte, leadType string) (bool, error)
	CreateLead(ctx context.Context, lead ProspectLead, rawTextEnc, contactHash []byte) (int64, string, error)
	RecordCollectResult(ctx context.Context, sourceID int64, success bool, startedAt, nextAt time.Time, stats CollectRunStats, errMsg string) (CollectResult, error)
	CreateCollectFailureNotifications(ctx context.Context, collectRunID int64, title, body string) (int64, error)
}

// RunStats 一轮采集的统计，便于日志与后续可观测性。
type RunStats struct {
	Halted  bool
	Sources int
	CollectRunStats
}

// 失败退避：首次失败 5 分钟后重试，按 5/10/20/40/60 递增，不超过采集间隔。
const (
	collectRetryBase             = 5 * time.Minute
	collectRetryMax              = 60 * time.Minute
	collectFailureAlertThreshold = 3
)

// Ingestor 负责一轮采集：逐源拉取并入库。
type Ingestor struct {
	store      LeadStore
	pii        *secure.PII
	collectors map[string]Collector
	logf       func(format string, args ...any)

	mu       sync.Mutex
	limiters map[int64]*rateLimiter
}

func NewIngestor(store LeadStore, pii *secure.PII, collectors map[string]Collector, logf func(string, ...any)) *Ingestor {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Ingestor{store: store, pii: pii, collectors: collectors, logf: logf}
}

// NewDefaultIngestor 用默认采集器（自有留资 + 公开 API）组装采集器。
func NewDefaultIngestor(repo *Repository, pii *secure.PII, logf func(string, ...any)) *Ingestor {
	return NewIngestor(repo, pii, newDefaultCollectors(repo), logf)
}

// RunOnce 执行一轮全量采集（不写调度状态）。
// 停机开关每轮从数据库读取，确保管理站切换后独立运行的 worker 进程立即生效。
func (i *Ingestor) RunOnce(ctx context.Context) (RunStats, error) {
	var stats RunStats

	halted, err := i.store.GetSetting(ctx, SettingKeyGlobalHalt)
	if err != nil {
		return stats, fmt.Errorf("read kill switch: %w", err)
	}
	if parseBoolSetting(halted) {
		stats.Halted = true
		return stats, nil
	}

	sources, err := i.store.ListEnabledSources(ctx)
	if err != nil {
		return stats, err
	}
	stats.Sources = len(sources)

	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if _, ok := i.collectors[source.Kind]; !ok {
			i.logf("prospect: 跳过未启用的信息源类型 kind=%s name=%s", source.Kind, source.Name)
			continue
		}
		st, err := i.collectSource(ctx, source)
		if err != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			stats.Failed++
			i.logf("prospect: 采集失败 source=%s: %v", source.Name, err)
			continue
		}
		stats.add(st)
	}
	return stats, nil
}

// RunDue 执行一轮定时采集：只处理已到期的信息源，并把调度状态与运行记录写回。
// 成功后按采集间隔排下一次；失败按 5/10/20/40/60 分钟退避重试。
func (i *Ingestor) RunDue(ctx context.Context, now time.Time) (RunStats, error) {
	var stats RunStats

	halted, err := i.store.GetSetting(ctx, SettingKeyGlobalHalt)
	if err != nil {
		return stats, fmt.Errorf("read kill switch: %w", err)
	}
	if parseBoolSetting(halted) {
		stats.Halted = true
		return stats, nil
	}

	sources, err := i.store.ListDueSources(ctx, now)
	if err != nil {
		return stats, err
	}
	stats.Sources = len(sources)

	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		st, runErr := i.collectSource(ctx, source)
		if runErr != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			st.Failed++
			stats.Failed++
			i.logf("prospect: 采集失败 source=%s: %v", source.Name, runErr)
			next := nextCollectAt(now, source, false)
			result, err := i.store.RecordCollectResult(ctx, source.ID, false, now, next, st, runErr.Error())
			if err != nil {
				i.logf("prospect: 记录采集失败结果失败 source=%s: %v", source.Name, err)
				continue
			}
			if result.ConsecutiveFailures == collectFailureAlertThreshold {
				title, body := collectFailureNotification(source, result.ConsecutiveFailures, runErr)
				if _, err := i.store.CreateCollectFailureNotifications(ctx, result.RunID, title, body); err != nil {
					i.logf("prospect: 写入采集失败告警失败 source=%s: %v", source.Name, err)
				}
			}
			continue
		}
		stats.add(st)
		next := nextCollectAt(now, source, true)
		if _, err := i.store.RecordCollectResult(ctx, source.ID, true, now, next, st, ""); err != nil {
			i.logf("prospect: 记录采集结果失败 source=%s: %v", source.Name, err)
		}
	}
	return stats, nil
}

func (s *RunStats) add(st CollectRunStats) {
	s.Fetched += st.Fetched
	s.Ingested += st.Ingested
	s.Duplicate += st.Duplicate
	s.OptedOut += st.OptedOut
	s.Invalid += st.Invalid
	s.Failed += st.Failed
}

// collectFailureNotification 生成采集告警文案，不包含线索正文或联系方式。
func collectFailureNotification(source LeadSource, failures int, runErr error) (title, body string) {
	return "信息源连续采集失败", fmt.Sprintf("信息源「%s」已连续 %d 次采集失败，请检查采集配置。最近错误：%s",
		source.Name, failures, runErr.Error())
}

// nextCollectAt 计算下一次采集时间：成功按间隔，失败按指数退避且不超过间隔。
func nextCollectAt(now time.Time, source LeadSource, success bool) time.Time {
	interval := time.Duration(source.CollectIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = DefaultCollectIntervalMinutes * time.Minute
	}
	if success {
		return now.Add(interval)
	}
	delay := collectRetryBase
	for n := 1; n < source.ConsecutiveFailures+1 && delay < interval; n++ {
		delay *= 2
	}
	if delay > collectRetryMax {
		delay = collectRetryMax
	}
	if delay > interval {
		delay = interval
	}
	return now.Add(delay)
}

// collectSource 拉取并入库单个信息源，返回本次采集的统计；拉取失败返回错误。
func (i *Ingestor) collectSource(ctx context.Context, source LeadSource) (CollectRunStats, error) {
	var st CollectRunStats
	collector, ok := i.collectors[source.Kind]
	if !ok {
		return st, fmt.Errorf("未实现的采集器类型: %s", source.Kind)
	}
	if err := i.limiterFor(source).Wait(ctx); err != nil {
		return st, err
	}
	posts, err := collector.Fetch(ctx, source)
	if err != nil {
		return st, err
	}
	st.Fetched = len(posts)
	for _, post := range posts {
		i.ingestOne(ctx, source, post, &st)
	}
	return st, nil
}

func (i *Ingestor) ingestOne(ctx context.Context, source LeadSource, post RawPost, stats *CollectRunStats) {
	if !validLeadType(post.LeadType) || post.ExternalPostID == "" {
		stats.Invalid++
		return
	}
	channel := post.ContactChannel
	if !validContactChannel(channel) {
		channel = ChannelUnknown
	}

	hash := post.ContactHash
	if len(hash) == 0 && post.ContactValue != "" {
		if channel == ChannelPhone {
			hash = i.pii.HashPhone(post.ContactValue)
		} else {
			hash = i.pii.Hash(post.ContactValue)
		}
	}
	if len(hash) > 0 {
		optedOut, err := i.store.OptOutExists(ctx, hash, post.LeadType)
		if err != nil {
			stats.Failed++
			i.logf("prospect: 查询拒收名单失败 source=%s: %v", source.Name, err)
			return
		}
		if optedOut {
			stats.OptedOut++
			return
		}
		exists, err := i.store.LeadExistsByContactHash(ctx, hash, post.LeadType)
		if err != nil {
			stats.Failed++
			i.logf("prospect: 联系方式去重查询失败 source=%s: %v", source.Name, err)
			return
		}
		if exists {
			stats.Duplicate++
			return
		}
	}

	rawTextEnc, err := i.pii.EncryptString(post.RawText)
	if err != nil {
		stats.Failed++
		i.logf("prospect: 原文加密失败 source=%s: %v", source.Name, err)
		return
	}

	platform := post.Platform
	if platform == "" {
		platform = source.Name
	}
	lead := ProspectLead{
		SourceID:       source.ID,
		LeadType:       post.LeadType,
		Platform:       platform,
		ExternalPostID: post.ExternalPostID,
		ContactChannel: channel,
		CityCode:       post.CityCode,
		DistrictCode:   post.DistrictCode,
		BudgetCents:    post.BudgetCents,
		AreaSqm:        post.AreaSqm,
		Layout:         post.Layout,
		IntentStatus:   IntentUnknown,
		Status:         StatusNew,
	}
	if _, _, err := i.store.CreateLead(ctx, lead, rawTextEnc, hash); err != nil {
		if errors.Is(err, ErrDuplicateLead) {
			stats.Duplicate++
			return
		}
		stats.Failed++
		i.logf("prospect: 线索入库失败 source=%s post=%s: %v", source.Name, post.ExternalPostID, err)
		return
	}
	stats.Ingested++
}

func (i *Ingestor) limiterFor(source LeadSource) *rateLimiter {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.limiters == nil {
		i.limiters = map[int64]*rateLimiter{}
	}
	l, ok := i.limiters[source.ID]
	if !ok {
		l = newRateLimiter(time.Duration(source.RateLimitMs) * time.Millisecond)
		i.limiters[source.ID] = l
	}
	return l
}

func validLeadType(t string) bool {
	return t == LeadTypeTenant || t == LeadTypeLandlord
}

func validContactChannel(ch string) bool {
	switch ch {
	case ChannelInApp, ChannelSMS, ChannelWeChat, ChannelPhone, ChannelUnknown:
		return true
	}
	return false
}
