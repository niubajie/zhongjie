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
	GetSetting(ctx context.Context, key string) (string, error)
	OptOutExists(ctx context.Context, contactHash []byte, leadType string) (bool, error)
	LeadExistsByContactHash(ctx context.Context, contactHash []byte, leadType string) (bool, error)
	CreateLead(ctx context.Context, lead ProspectLead, rawTextEnc, contactHash []byte) (int64, string, error)
}

// RunStats 一轮采集的统计，便于日志与后续可观测性。
type RunStats struct {
	Halted    bool
	Sources   int
	Fetched   int
	Ingested  int
	Duplicate int
	OptedOut  int
	Invalid   int
	Failed    int
}

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

// RunOnce 执行一轮采集。停机开关每轮从数据库读取，确保管理站切换后
// 独立运行的 worker 进程立即生效（内存态只在 api 进程内）。
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
		collector, ok := i.collectors[source.Kind]
		if !ok {
			i.logf("prospect: 跳过未启用的信息源类型 kind=%s name=%s", source.Kind, source.Name)
			continue
		}
		if err := i.limiterFor(source).Wait(ctx); err != nil {
			return stats, err
		}
		posts, err := collector.Fetch(ctx, source)
		if err != nil {
			stats.Failed++
			i.logf("prospect: 采集失败 source=%s: %v", source.Name, err)
			continue
		}
		stats.Fetched += len(posts)
		for _, post := range posts {
			i.ingestOne(ctx, source, post, &stats)
		}
	}
	return stats, nil
}

func (i *Ingestor) ingestOne(ctx context.Context, source LeadSource, post RawPost, stats *RunStats) {
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
