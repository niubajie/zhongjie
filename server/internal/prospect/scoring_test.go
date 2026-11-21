package prospect

import (
	"context"
	"errors"
	"testing"

	"housing/internal/modelgw"
)

type updatedIntent struct {
	uuid         string
	intentStatus string
	status       string
	score        int
}

type fakeScoringStore struct {
	leads   []UnscoredLead
	updated []updatedIntent
}

func (f *fakeScoringStore) ListUnscoredLeads(context.Context, int) ([]UnscoredLead, error) {
	return f.leads, nil
}

func (f *fakeScoringStore) UpdateLeadIntent(_ context.Context, uuid, intentStatus, status string, score int) error {
	f.updated = append(f.updated, updatedIntent{uuid: uuid, intentStatus: intentStatus, status: status, score: score})
	return nil
}

type fakeLLM struct {
	outputs []string
	errs    []error
	calls   int
}

func (f *fakeLLM) Chat(context.Context, []modelgw.Message) (string, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return "", f.errs[i]
	}
	if i < len(f.outputs) {
		return f.outputs[i], nil
	}
	return `{"intent":"unknown","score":0}`, nil
}

func makeUnscoredLead(t *testing.T, leadType, rawText string) UnscoredLead {
	t.Helper()
	pii := testPII(t)
	enc, err := pii.EncryptString(rawText)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return UnscoredLead{
		UUID: "uuid-1", LeadType: leadType, RawTextEnc: enc,
		ContactChannel: ChannelPhone, CityCode: "330100", DistrictCode: "330106",
		BudgetCents: 450000, AreaSqm: 89.5, Layout: "2室1厅", CreatedAt: "2026-11-22T08:00:00Z",
	}
}

func runScorer(t *testing.T, store *fakeScoringStore, llm LLMClient) ScoringStats {
	t.Helper()
	s := NewScorer(store, llm, testPII(t), nil)
	stats, err := s.ScoreBatch(context.Background(), 50)
	if err != nil {
		t.Fatalf("ScoreBatch: %v", err)
	}
	return stats
}

func TestScorerQueuesHighScoreTenant(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "求租两室一厅")}}
	llm := &fakeLLM{outputs: []string{`{"intent":"seeking","score":85}`}}
	stats := runScorer(t, store, llm)
	if stats.Queued != 1 || stats.Ignored != 0 || stats.Failed != 0 {
		t.Fatalf("高分有效意向应进复核队列: %+v", stats)
	}
	if len(store.updated) != 1 {
		t.Fatalf("应写入 1 条, got %d", len(store.updated))
	}
	u := store.updated[0]
	if u.intentStatus != IntentSeeking || u.status != StatusNew || u.score != 85 {
		t.Errorf("写入值错误: %+v", u)
	}
}

func TestScorerIgnoresLowScore(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "求租")}}
	llm := &fakeLLM{outputs: []string{`{"intent":"seeking","score":50}`}}
	stats := runScorer(t, store, llm)
	if stats.Ignored != 1 || stats.Queued != 0 {
		t.Fatalf("低分应 ignored: %+v", stats)
	}
	if store.updated[0].status != StatusIgnored {
		t.Errorf("status 应为 ignored, got %s", store.updated[0].status)
	}
}

func TestScorerIgnoresInvalidIntent(t *testing.T) {
	for _, intent := range []string{IntentRented, IntentAgent, IntentIrrelevant, IntentUnknown} {
		store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "x")}}
		llm := &fakeLLM{outputs: []string{`{"intent":"` + intent + `","score":90}`}}
		stats := runScorer(t, store, llm)
		if stats.Ignored != 1 || stats.Queued != 0 {
			t.Errorf("intent=%s 应 ignored: %+v", intent, stats)
		}
		if store.updated[0].status != StatusIgnored {
			t.Errorf("intent=%s status 应为 ignored, got %s", intent, store.updated[0].status)
		}
	}
}

func TestScorerQueuesLandlord(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeLandlord, "个人房东直租")}}
	llm := &fakeLLM{outputs: []string{`{"intent":"landlord_listing","score":80}`}}
	stats := runScorer(t, store, llm)
	if stats.Queued != 1 {
		t.Fatalf("landlord 高分应进队列: %+v", stats)
	}
	if store.updated[0].status != StatusNew {
		t.Errorf("status 应为 new, got %s", store.updated[0].status)
	}
}

func TestScorerTenantLandlordIntentMismatch(t *testing.T) {
	// tenant 线索被判为 landlord_listing → 无效 → ignored
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "x")}}
	llm := &fakeLLM{outputs: []string{`{"intent":"landlord_listing","score":90}`}}
	stats := runScorer(t, store, llm)
	if stats.Ignored != 1 {
		t.Fatalf("tenant 线索的 landlord_listing 意向应 ignored: %+v", stats)
	}
}

func TestScorerFailedKeepsLead(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "求租")}}
	llm := &fakeLLM{errs: []error{errors.New("upstream timeout")}}
	stats := runScorer(t, store, llm)
	if stats.Failed != 1 || len(store.updated) != 0 {
		t.Fatalf("LLM 失败不应改库: stats=%+v updated=%d", stats, len(store.updated))
	}
}

func TestScorerParseFailure(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "求租")}}
	llm := &fakeLLM{outputs: []string{"这不是 JSON"}}
	stats := runScorer(t, store, llm)
	if stats.Failed != 1 || len(store.updated) != 0 {
		t.Fatalf("解析失败不应改库: stats=%+v updated=%d", stats, len(store.updated))
	}
}

func TestScorerToleratesMarkdownWrap(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "求租")}}
	llm := &fakeLLM{outputs: []string{"```json\n{\"intent\":\"seeking\",\"score\":88}\n```"}}
	stats := runScorer(t, store, llm)
	if stats.Queued != 1 {
		t.Fatalf("markdown 包裹应可解析: %+v", stats)
	}
}

func TestScorerSkipsEmptyText(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{{UUID: "u1", LeadType: LeadTypeTenant}}}
	llm := &fakeLLM{}
	stats := runScorer(t, store, llm)
	if stats.Ignored != 1 || llm.calls != 0 {
		t.Fatalf("空原文应 ignored 且不调 LLM: stats=%+v calls=%d", stats, llm.calls)
	}
}

func TestScorerNilLLM(t *testing.T) {
	store := &fakeScoringStore{leads: []UnscoredLead{makeUnscoredLead(t, LeadTypeTenant, "x")}}
	s := NewScorer(store, nil, testPII(t), nil)
	stats, err := s.ScoreBatch(context.Background(), 50)
	if err != nil {
		t.Fatalf("ScoreBatch: %v", err)
	}
	if stats.Total != 0 {
		t.Fatalf("LLM 未配置应直接返回: %+v", stats)
	}
}

func TestDecideStatus(t *testing.T) {
	cases := []struct {
		leadType, intent string
		score            int
		want             string
	}{
		{LeadTypeTenant, IntentSeeking, 70, StatusNew},
		{LeadTypeTenant, IntentSeeking, 69, StatusIgnored},
		{LeadTypeTenant, IntentRented, 100, StatusIgnored},
		{LeadTypeLandlord, IntentLandlordListing, 70, StatusNew},
		{LeadTypeLandlord, IntentSeeking, 90, StatusIgnored},
		{LeadTypeTenant, IntentUnknown, 90, StatusIgnored},
	}
	for _, c := range cases {
		if got := decideStatus(c.leadType, c.intent, c.score); got != c.want {
			t.Errorf("decideStatus(%s,%s,%d)=%s want %s", c.leadType, c.intent, c.score, got, c.want)
		}
	}
}
