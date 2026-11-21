package prospect

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
	optOuts        map[string]bool
	existingHashes map[string]bool
	created        []createdLead
	createErr      error
}

func hashKey(hash []byte, leadType string) string {
	return hex.EncodeToString(hash) + "|" + leadType
}

func (f *fakeStore) GetSetting(context.Context, string) (string, error) { return f.halted, nil }

func (f *fakeStore) ListEnabledSources(context.Context) ([]LeadSource, error) { return f.sources, nil }

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
