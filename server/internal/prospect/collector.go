package prospect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// 批次 3：采集器。
// 采集只读取信息源公开发布的内容；联系方式的明文只在内存中出现，
// 落库前一律哈希（见 ingest.go）。第三方 HTML 抓取源待业务方确认后实现。

// RawPost 是采集器从信息源取出的一条原始信息。
type RawPost struct {
	ExternalPostID string // 源内唯一 ID，与 source_id 组成精确去重键
	LeadType       string // tenant / landlord
	Platform       string // 平台标识，空则由信息源名称填充
	ContactValue   string // 明文联系方式，由 Ingestor 哈希后落库
	ContactHash    []byte // 已有的联系方式哈希（自有数据），优先于 ContactValue
	ContactChannel string
	CityCode       string
	DistrictCode   string
	BudgetCents    int64
	AreaSqm        float64
	Layout         string
	RawText        string // 原文，落库前 AES-GCM 加密
}

// Collector 采集器接口：从单个信息源取出原始信息。
type Collector interface {
	Kind() string
	Fetch(ctx context.Context, source LeadSource) ([]RawPost, error)
}

const (
	defaultRateLimit  = 3 * time.Second
	apiMaxBodyBytes   = 4 << 20
	apiRequestTimeout = 10 * time.Second
)

// rateLimiter 保证同一信息源两次请求的最小间隔（方案要求 ≥ rate_limit_ms）。
// 并发上限由采集调度顺序执行保证（单源同时至多 1 个请求，≤ 2）。
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
}

func newRateLimiter(interval time.Duration) *rateLimiter {
	if interval <= 0 {
		interval = defaultRateLimit
	}
	return &rateLimiter{
		interval: interval,
		now:      time.Now,
		sleep:    sleepCtx,
	}
}

// Wait 在需要时阻塞到下一次允许发请求的时刻；ctx 取消时立即返回。
func (l *rateLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := l.now()
	wait := time.Duration(0)
	if !l.last.IsZero() {
		if elapsed := now.Sub(l.last); elapsed < l.interval {
			wait = l.interval - elapsed
		}
	}
	l.last = now.Add(wait)
	l.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	return l.sleep(ctx, wait)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ---- 自有留资源（零合规风险）----

type selfOwnedStore interface {
	ListRecentSelfOwned(ctx context.Context, limit int) ([]RawPost, error)
}

type selfOwnedCollector struct {
	store selfOwnedStore
	limit int
}

func (c *selfOwnedCollector) Kind() string { return SourceKindSelfOwned }

func (c *selfOwnedCollector) Fetch(ctx context.Context, _ LeadSource) ([]RawPost, error) {
	return c.store.ListRecentSelfOwned(ctx, c.limit)
}

// ---- 公开 API 源 ----

// apiCollector 从 source.base_url 拉取 JSON 数组。字段约定为 camelCase，
// 与站内接口一致；真实合规 API 接入时只需按其返回结构调整 apiPost。
type apiCollector struct {
	client *http.Client
}

func (c *apiCollector) Kind() string { return SourceKindPublicAPI }

type apiPost struct {
	ExternalPostID string  `json:"externalPostId"`
	LeadType       string  `json:"leadType"`
	Contact        string  `json:"contact"`
	ContactChannel string  `json:"contactChannel"`
	CityCode       string  `json:"cityCode"`
	DistrictCode   string  `json:"districtCode"`
	BudgetCents    int64   `json:"budgetCents"`
	AreaSqm        float64 `json:"areaSqm"`
	Layout         string  `json:"layout"`
	RawText        string  `json:"rawText"`
}

func (c *apiCollector) Fetch(ctx context.Context, source LeadSource) ([]RawPost, error) {
	if source.BaseURL == "" {
		return nil, fmt.Errorf("信息源 %s 未配置 base_url", source.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.BaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	client := c.client
	if client == nil {
		client = &http.Client{Timeout: apiRequestTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch public api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch public api: status %d", resp.StatusCode)
	}
	var items []apiPost
	if err := json.NewDecoder(io.LimitReader(resp.Body, apiMaxBodyBytes)).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode public api response: %w", err)
	}
	out := make([]RawPost, 0, len(items))
	for _, it := range items {
		out = append(out, RawPost{
			ExternalPostID: it.ExternalPostID,
			LeadType:       it.LeadType,
			Platform:       source.Name,
			ContactValue:   it.Contact,
			ContactChannel: it.ContactChannel,
			CityCode:       it.CityCode,
			DistrictCode:   it.DistrictCode,
			BudgetCents:    it.BudgetCents,
			AreaSqm:        it.AreaSqm,
			Layout:         it.Layout,
			RawText:        it.RawText,
		})
	}
	return out, nil
}

// newDefaultCollectors 注册首版支持的信息源类型。
// third_party_scrape 按 F4 结论留待业务方书面确认后再实现。
func newDefaultCollectors(repo *Repository) map[string]Collector {
	return map[string]Collector{
		SourceKindSelfOwned: &selfOwnedCollector{store: repo, limit: 200},
		SourceKindPublicAPI: &apiCollector{client: &http.Client{Timeout: apiRequestTimeout}},
	}
}
