package prospect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"housing/internal/modelgw"
	"housing/internal/secure"
)

// 批次 4：LLM 意向分类 + 评分。
// 对 status=new 且 intent_status=unknown 的线索调用模型网关分类并打分，
// 有效意向且分数达标才保留 new 进人工复核队列，其余标 ignored。
// 所有调用都经过 modelgw（自动写 usage_records）；失败不丢数据，留待下轮重试。

// LLMClient 抽象模型网关的聊天能力（*modelgw.Gateway 实现）。
type LLMClient interface {
	Chat(ctx context.Context, messages []modelgw.Message) (string, error)
}

// ScoringStore 是 Scorer 依赖的存储能力。
type ScoringStore interface {
	ListUnscoredLeads(ctx context.Context, limit int) ([]UnscoredLead, error)
	UpdateLeadIntent(ctx context.Context, uuid, intentStatus, status string, score int) error
}

// ScoringStats 一轮评分统计。
type ScoringStats struct {
	Total   int
	Queued  int // 有效意向且分数达标，进人工复核队列（status=new）
	Ignored int // 无效意向或分数不足（status=ignored）
	Failed  int // LLM 调用失败或输出无法解析，留待下轮重试
}

// Scorer 意向分类与评分器。
type Scorer struct {
	store ScoringStore
	llm   LLMClient
	pii   *secure.PII
	logf  func(format string, args ...any)
}

func NewScorer(store ScoringStore, llm LLMClient, pii *secure.PII, logf func(string, ...any)) *Scorer {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Scorer{store: store, llm: llm, pii: pii, logf: logf}
}

// ScoreBatch 对一批待评分线索做分类与评分。LLM 未配置时直接返回（不视为错误）。
func (s *Scorer) ScoreBatch(ctx context.Context, limit int) (ScoringStats, error) {
	var stats ScoringStats
	if s.llm == nil {
		return stats, nil
	}
	leads, err := s.store.ListUnscoredLeads(ctx, limit)
	if err != nil {
		return stats, err
	}
	stats.Total = len(leads)
	for _, lead := range leads {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		s.scoreOne(ctx, lead, &stats)
	}
	return stats, nil
}

func (s *Scorer) scoreOne(ctx context.Context, lead UnscoredLead, stats *ScoringStats) {
	rawText, err := s.pii.DecryptString(lead.RawTextEnc)
	if err != nil {
		// 解密失败属数据问题，标 ignored 避免反复重试。
		s.logf("prospect: 解密原文失败 uuid=%s: %v", lead.UUID, err)
		_ = s.store.UpdateLeadIntent(ctx, lead.UUID, IntentUnknown, StatusIgnored, 0)
		stats.Ignored++
		return
	}
	if strings.TrimSpace(rawText) == "" {
		// 无原文可分类，标 ignored。
		_ = s.store.UpdateLeadIntent(ctx, lead.UUID, IntentUnknown, StatusIgnored, 0)
		stats.Ignored++
		return
	}

	out, err := s.llm.Chat(ctx, buildScoringMessages(lead, rawText))
	if err != nil {
		// LLM 失败：不改库，留待下轮重试，不丢数据。
		stats.Failed++
		s.logf("prospect: LLM 评分失败 uuid=%s: %v", lead.UUID, err)
		return
	}
	verdict, err := parseVerdict(out)
	if err != nil {
		stats.Failed++
		s.logf("prospect: 解析 LLM 输出失败 uuid=%s: %v", lead.UUID, err)
		return
	}

	intent := normalizeIntent(verdict.Intent)
	score := clampScore(verdict.Score)
	status := decideStatus(lead.LeadType, intent, score)
	if err := s.store.UpdateLeadIntent(ctx, lead.UUID, intent, status, score); err != nil {
		stats.Failed++
		s.logf("prospect: 写入评分失败 uuid=%s: %v", lead.UUID, err)
		return
	}
	if status == StatusNew {
		stats.Queued++
	} else {
		stats.Ignored++
	}
}

// decideStatus 按意向与分数分流：有效意向且 >= 阈值保留 new 进复核队列，否则 ignored。
func decideStatus(leadType, intent string, score int) string {
	effective := (leadType == LeadTypeTenant && intent == IntentSeeking) ||
		(leadType == LeadTypeLandlord && intent == IntentLandlordListing)
	if !effective {
		return StatusIgnored
	}
	if score < ReviewScoreThreshold {
		return StatusIgnored
	}
	return StatusNew
}

func buildScoringMessages(lead UnscoredLead, rawText string) []modelgw.Message {
	sys := `你是房屋中介平台的潜在线索分类器。根据线索类型和原文，判断意向并打分。

意向枚举：
- seeking：正在求租（tenant 有效意向）
- rented：已租到房（无效）
- landlord_listing：个人房东发布房源（landlord 有效意向）
- agent：同行中介发布（无效）
- irrelevant：与租售无关（无效）
- unknown：信息不足无法判断（无效）

评分维度（0-100 整数）：时效性、城市/区域匹配度、字段完整度、联系方式有效性。
只输出 JSON，不要任何解释或 markdown 包裹：
{"intent":"...","score":85}`

	user := fmt.Sprintf("线索类型：%s\n城市：%s 区域：%s\n预算(分)：%d 面积(㎡)：%s 户型：%s\n联系渠道：%s\n发布时间：%s\n\n原文：\n%s",
		lead.LeadType, lead.CityCode, lead.DistrictCode, lead.BudgetCents,
		formatArea(lead.AreaSqm), lead.Layout, lead.ContactChannel, lead.CreatedAt, rawText)

	return []modelgw.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}
}

func formatArea(area float64) string {
	if area <= 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", area), "0"), ".")
}

type llmVerdict struct {
	Intent string `json:"intent"`
	Score  int    `json:"score"`
}

// parseVerdict 从 LLM 输出中提取 JSON（容忍 ```json 包裹或前后解释文字）。
func parseVerdict(raw string) (llmVerdict, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return llmVerdict{}, fmt.Errorf("no JSON object in LLM output")
	}
	var v llmVerdict
	if err := json.Unmarshal([]byte(raw[start:end+1]), &v); err != nil {
		return llmVerdict{}, fmt.Errorf("parse LLM JSON: %w", err)
	}
	return v, nil
}

func normalizeIntent(intent string) string {
	switch intent {
	case IntentSeeking, IntentRented, IntentLandlordListing, IntentAgent, IntentIrrelevant:
		return intent
	}
	return IntentUnknown
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}
