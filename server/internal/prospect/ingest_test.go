package prospect

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"housing/internal/secure"
)

func testPII(t *testing.T) *secure.PII {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	p, err := secure.NewPII(key, "test-hash-key")
	if err != nil {
		t.Fatalf("NewPII: %v", err)
	}
	return p
}

type createdLead struct {
	lead       ProspectLead
	rawTextEnc []byte
	contactHsh []byte
}

type fakeStore struct {
	halted         string
	sources        []LeadSource
	dueSources     []LeadSource
	optOuts        map[string]bool
	existingHashes map[string]bool
	created        []createdLead
	createErr      error
	collects       []recordedCollect
	failureCounts  map[int64]int
	collectAlerts  []collectFailureAlert
}

type recordedCollect struct {
	sourceID  int64
	success   bool
	startedAt time.Time
	nextAt    time.Time
	stats     CollectRunStats
	errMsg    string
}

func hashKey(hash []byte, leadType string) string {
	return hex.EncodeToString(hash) + "|" + leadType
}

func (f *fakeStore) GetSetting(context.Context, string) (string, error) { return f.halted, nil }

func (f *fakeStore) ListEnabledSources(context.Context) ([]LeadSource, error) { return f.sources, nil }

func (f *fakeStore) ListDueSources(context.Context, time.Time) ([]LeadSource, error) {
	return f.dueSources, nil
}

type collectFailureAlert struct {
	collectRunID int64
	title        string
	body         string
}

func (f *fakeStore) RecordCollectResult(_ context.Context, sourceID int64, success bool, startedAt, nextAt time.Time, stats CollectRunStats, errMsg string) (CollectResult, error) {
	f.collects = append(f.collects, recordedCollect{
		sourceID:  sourceID,
		success:   success,
		startedAt: startedAt,
		nextAt:    nextAt,
		stats:     stats,
		errMsg:    errMsg,
	})
	if f.failureCounts == nil {
		f.failureCounts = map[int64]int{}
	}
	if success {
		f.failureCounts[sourceID] = 0
	} else {
		f.failureCounts[sourceID]++
	}
	return CollectResult{RunID: int64(len(f.collects)), ConsecutiveFailures: f.failureCounts[sourceID]}, nil
}

func (f *fakeStore) CreateCollectFailureNotifications(_ context.Context, collectRunID int64, title, body string) (int64, error) {
	f.collectAlerts = append(f.collectAlerts, collectFailureAlert{collectRunID: collectRunID, title: title, body: body})
	return 1, nil
}

func (f *fakeStore) OptOutExists(_ context.Context, hash []byte, leadType string) (bool, error) {
	return f.optOuts[hashKey(hash, leadType)], nil
}

func (f *fakeStore) LeadExistsByContactHash(_ context.Context, hash []byte, leadType string) (bool, error) {
	return f.existingHashes[hashKey(hash, leadType)], nil
}

func (f *fakeStore) CreateLead(_ context.Context, lead ProspectLead, rawTextEnc, contactHash []byte) (int64, string, error) {
	if f.createErr != nil {
		return 0, "", f.createErr
	}
	f.created = append(f.created, createdLead{lead: lead, rawTextEnc: rawTextEnc, contactHsh: contactHash})
	return int64(len(f.created)), "uuid", nil
}

type fakeCollector struct {
	kind  string
	posts []RawPost
	err   error
	calls int
}

func (c *fakeCollector) Kind() string { return c.kind }

func (c *fakeCollector) Fetch(context.Context, LeadSource) ([]RawPost, error) {
	c.calls++
	return c.posts, c.err
}

func newTestIngestor(t *testing.T, store LeadStore, fc *fakeCollector) *Ingestor {
	t.Helper()
	return NewIngestor(store, testPII(t), map[string]Collector{fc.Kind(): fc}, nil)
}

func testSource() LeadSource {
	return LeadSource{ID: 1, Name: "mock-source", Kind: SourceKindPublicAPI, RateLimitMs: 1}
}

func TestIngestorSkipsWhenHalted(t *testing.T) {
	store := &fakeStore{halted: "true", sources: []LeadSource{testSource()}}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{{ExternalPostID: "p1", LeadType: LeadTypeTenant}}}
	stats, err := newTestIngestor(t, store, fc).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !stats.Halted || fc.calls != 0 || len(store.created) != 0 {
		t.Fatalf("停机时不应采集: halted=%v calls=%d created=%d", stats.Halted, fc.calls, len(store.created))
	}
}

func TestIngestorIngestsAndEncrypts(t *testing.T) {
	store := &fakeStore{halted: "false", sources: []LeadSource{testSource()}}
	post := RawPost{
		ExternalPostID: "p1",
		LeadType:       LeadTypeTenant,
		ContactValue:   "138 0013 8000",
		ContactChannel: ChannelPhone,
		CityCode:       "330100",
		BudgetCents:    450000,
		RawText:        "求租：两室一厅，预算 4500",
	}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{post}}
	pii := testPII(t)
	ing := NewIngestor(store, pii, map[string]Collector{SourceKindPublicAPI: fc}, nil)

	stats, err := ing.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Ingested != 1 || len(store.created) != 1 {
		t.Fatalf("应入库 1 条: %+v", stats)
	}
	got := store.created[0]
	if !bytes.Equal(got.contactHsh, pii.HashPhone(post.ContactValue)) {
		t.Error("contact_hash 应为规范化后的手机号 HMAC")
	}
	if bytes.Equal(got.rawTextEnc, []byte(post.RawText)) {
		t.Error("raw_text_enc 不应是明文")
	}
	plain, err := pii.DecryptString(got.rawTextEnc)
	if err != nil || plain != post.RawText {
		t.Errorf("原文解密失败: %q, %v", plain, err)
	}
	if got.lead.Status != StatusNew || got.lead.IntentStatus != IntentUnknown {
		t.Errorf("新线索状态应为 new/unknown, got %s/%s", got.lead.Status, got.lead.IntentStatus)
	}
	if got.lead.SourceID != 1 || got.lead.ExternalPostID != "p1" {
		t.Errorf("source_id/external_post_id 错误: %+v", got.lead)
	}
}

func TestIngestorSkipsOptOut(t *testing.T) {
	pii := testPII(t)
	hash := pii.HashPhone("13800138000")
	store := &fakeStore{
		halted:  "false",
		sources: []LeadSource{testSource()},
		optOuts: map[string]bool{hashKey(hash, LeadTypeTenant): true},
	}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{{
		ExternalPostID: "p1", LeadType: LeadTypeTenant,
		ContactValue: "13800138000", ContactChannel: ChannelPhone,
	}}}
	stats, err := newTestIngestor(t, store, fc).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.OptedOut != 1 || len(store.created) != 0 {
		t.Fatalf("命中拒收应丢弃: %+v", stats)
	}
}

func TestIngestorSkipsDuplicateContactHash(t *testing.T) {
	pii := testPII(t)
	hash := pii.HashPhone("13800138000")
	store := &fakeStore{
		halted:         "false",
		sources:        []LeadSource{testSource()},
		existingHashes: map[string]bool{hashKey(hash, LeadTypeTenant): true},
	}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{{
		ExternalPostID: "p2", LeadType: LeadTypeTenant,
		ContactValue: "13800138000", ContactChannel: ChannelPhone,
	}}}
	stats, err := newTestIngestor(t, store, fc).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Duplicate != 1 || len(store.created) != 0 {
		t.Fatalf("跨平台同联系方式应去重: %+v", stats)
	}
}

func TestIngestorCountsDuplicatePost(t *testing.T) {
	store := &fakeStore{halted: "false", sources: []LeadSource{testSource()}, createErr: ErrDuplicateLead}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{{ExternalPostID: "p1", LeadType: LeadTypeTenant}}}
	stats, err := newTestIngestor(t, store, fc).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Duplicate != 1 || stats.Ingested != 0 {
		t.Fatalf("同源同帖应计为重复: %+v", stats)
	}
}

func TestIngestorSkipsInvalidPost(t *testing.T) {
	store := &fakeStore{halted: "false", sources: []LeadSource{testSource()}}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{
		{ExternalPostID: "", LeadType: LeadTypeTenant},
		{ExternalPostID: "p1", LeadType: "bogus"},
	}}
	stats, err := newTestIngestor(t, store, fc).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Invalid != 2 || len(store.created) != 0 {
		t.Fatalf("非法数据应跳过: %+v", stats)
	}
}

func TestRunDueRecordsSuccessAndSchedule(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	src := testSource()
	src.CollectIntervalMinutes = 30
	store := &fakeStore{halted: "false", dueSources: []LeadSource{src}}
	fc := &fakeCollector{kind: SourceKindPublicAPI, posts: []RawPost{{
		ExternalPostID: "p1", LeadType: LeadTypeTenant,
	}}}

	stats, err := newTestIngestor(t, store, fc).RunDue(context.Background(), now)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if stats.Sources != 1 || stats.Ingested != 1 || fc.calls != 1 {
		t.Fatalf("到期源应被采集: stats=%+v calls=%d", stats, fc.calls)
	}
	if len(store.collects) != 1 {
		t.Fatalf("应写一条运行记录, got %d", len(store.collects))
	}
	rec := store.collects[0]
	if !rec.success || rec.sourceID != src.ID || rec.errMsg != "" {
		t.Fatalf("运行记录错误: %+v", rec)
	}
	if want := now.Add(30 * time.Minute); !rec.nextAt.Equal(want) {
		t.Fatalf("成功应按间隔排下一次: want %v got %v", want, rec.nextAt)
	}
	if rec.stats.Fetched != 1 || rec.stats.Ingested != 1 {
		t.Fatalf("运行统计错误: %+v", rec.stats)
	}
}

func TestRunDueBacksOffOnFetchFailure(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	src := testSource()
	src.CollectIntervalMinutes = 60
	src.ConsecutiveFailures = 2
	store := &fakeStore{halted: "false", dueSources: []LeadSource{src}}
	fc := &fakeCollector{kind: SourceKindPublicAPI, err: errors.New("boom")}

	stats, err := newTestIngestor(t, store, fc).RunDue(context.Background(), now)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if stats.Failed != 1 || len(store.collects) != 1 {
		t.Fatalf("失败应记一条: stats=%+v records=%d", stats, len(store.collects))
	}
	rec := store.collects[0]
	if rec.success || rec.errMsg != "boom" || rec.stats.Failed != 1 {
		t.Fatalf("失败运行记录错误: %+v", rec)
	}
	if want := now.Add(20 * time.Minute); !rec.nextAt.Equal(want) {
		t.Fatalf("连续第 3 次失败应退避 20 分钟: want %v got %v", want, rec.nextAt)
	}
}

func TestRunDueAlertsOnThirdFailureAndAfterRecovery(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	src := testSource()
	src.CollectIntervalMinutes = 60
	store := &fakeStore{halted: "false", dueSources: []LeadSource{src}}
	fc := &fakeCollector{kind: SourceKindPublicAPI, err: errors.New("boom")}
	ing := newTestIngestor(t, store, fc)

	for n := 1; n <= 4; n++ {
		if _, err := ing.RunDue(context.Background(), now); err != nil {
			t.Fatalf("第 %d 次失败采集: %v", n, err)
		}
		wantAlerts := 0
		if n >= 3 {
			wantAlerts = 1
		}
		if len(store.collectAlerts) != wantAlerts {
			t.Fatalf("第 %d 次失败后告警数应为 %d，实际 %d", n, wantAlerts, len(store.collectAlerts))
		}
	}
	alert := store.collectAlerts[0]
	if alert.collectRunID != 3 || alert.title != "信息源连续采集失败" {
		t.Fatalf("告警内容或关联运行记录错误: %+v", alert)
	}
	if !strings.Contains(alert.body, src.Name) || !strings.Contains(alert.body, "连续 3 次") {
		t.Fatalf("告警应包含来源名称与失败次数: %s", alert.body)
	}

	// 成功采集清零；之后新一轮连续失败达到阈值时允许再次告警。
	fc.err = nil
	if _, err := ing.RunDue(context.Background(), now); err != nil {
		t.Fatalf("恢复采集: %v", err)
	}
	fc.err = errors.New("boom-again")
	for n := 0; n < 3; n++ {
		if _, err := ing.RunDue(context.Background(), now); err != nil {
			t.Fatalf("恢复后第 %d 次失败采集: %v", n+1, err)
		}
	}
	if len(store.collectAlerts) != 2 || store.collectAlerts[1].collectRunID != 8 {
		t.Fatalf("恢复后新一轮第三次失败应重新告警: %+v", store.collectAlerts)
	}
}

func TestRunDueSkipsWhenNoSourceDue(t *testing.T) {
	store := &fakeStore{halted: "false"}
	fc := &fakeCollector{kind: SourceKindPublicAPI}
	stats, err := newTestIngestor(t, store, fc).RunDue(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if stats.Sources != 0 || fc.calls != 0 || len(store.collects) != 0 {
		t.Fatalf("无到期源时不应采集: stats=%+v calls=%d records=%d", stats, fc.calls, len(store.collects))
	}
}

func TestNextCollectAt(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		interval int
		failures int
		success  bool
		want     time.Duration
	}{
		{"成功按采集间隔", 30, 3, true, 30 * time.Minute},
		{"首次失败 5 分钟", 60, 0, false, 5 * time.Minute},
		{"第三次失败 20 分钟", 60, 2, false, 20 * time.Minute},
		{"退避不超过采集间隔", 15, 10, false, 15 * time.Minute},
		{"退避上限 60 分钟", 1440, 10, false, 60 * time.Minute},
		{"间隔为空用默认值", 0, 0, true, DefaultCollectIntervalMinutes * time.Minute},
	}
	for _, c := range cases {
		src := LeadSource{CollectIntervalMinutes: c.interval, ConsecutiveFailures: c.failures}
		got := nextCollectAt(now, src, c.success)
		if !got.Equal(now.Add(c.want)) {
			t.Errorf("%s: want %v got %v", c.name, now.Add(c.want), got)
		}
	}
}

func TestRateLimiterEnforcesInterval(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var slept []time.Duration
	l := &rateLimiter{
		interval: time.Second,
		now:      func() time.Time { return now },
		sleep: func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			now = now.Add(d)
			return nil
		},
	}
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(400 * time.Millisecond)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 || slept[0] != 600*time.Millisecond {
		t.Fatalf("第二次应等待 600ms，实际 %v", slept)
	}
	now = now.Add(5 * time.Second)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 {
		t.Fatalf("间隔已满足时不应等待，实际 %v", slept)
	}
}

func TestRateLimiterHonorsContext(t *testing.T) {
	l := &rateLimiter{
		interval: time.Hour,
		now:      time.Now,
		sleep:    sleepCtx,
	}
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled, got %v", err)
	}
}
