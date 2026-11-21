package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"housing/internal/modelgw"
	"housing/internal/storage"
)

var (
	budgetRange = regexp.MustCompile(`(\d{3,7})\s*[-~到至]\s*(\d{3,7})`)
	budgetOne   = regexp.MustCompile(`(?:预算|月租|租金)\s*(\d{3,7})`)
	areaRange   = regexp.MustCompile(`(\d{1,4})\s*[-~到至]\s*(\d{1,4})\s*(?:平|㎡|平米)`)
	areaOne     = regexp.MustCompile(`(\d{1,4})\s*(?:平|㎡|平米)`)
)

type Service struct {
	repo  *Repository
	gw    *modelgw.Gateway
	media *storage.Service
}

func NewService(repo *Repository, gw *modelgw.Gateway, media *storage.Service) *Service {
	return &Service{repo: repo, gw: gw, media: media}
}

type VoiceStatus struct {
	Engine string `json:"engine"`
	ASR    bool   `json:"asr"`
	TTS    bool   `json:"tts"`
	LLM    bool   `json:"llm"`
	Hint   string `json:"hint"`
}

func (s *Service) VoiceStatus() VoiceStatus {
	status := VoiceStatus{Engine: "half-duplex", Hint: "文字与小亿推荐可用；语音需要配置模型网关"}
	if s.gw == nil {
		return status
	}
	st := s.gw.Status()
	status.ASR, status.TTS, status.LLM = st.ASR, st.TTS, st.LLM
	if st.ASR && st.TTS {
		status.Hint = "语音已就绪，可随时打断"
	} else {
		status.Hint = "语音未配置完整，请设置 MODELGW_ASR_* 与 MODELGW_TTS_*"
	}
	return status
}

func (s *Service) Transcribe(ctx context.Context, audio []byte, mime string) (string, error) {
	if s.gw == nil {
		return "", modelgw.ErrNotConfigured
	}
	return s.gw.Transcribe(ctx, audio, mime)
}

// Speak synthesizes one sentence and returns a playable URL.
func (s *Service) Speak(ctx context.Context, seq int, text string) (string, error) {
	if s.gw == nil {
		return "", modelgw.ErrNotConfigured
	}
	if s.media == nil {
		return "", errors.New("media storage is not configured")
	}
	audio, _, err := s.gw.Synthesize(ctx, text)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("assistant/tts/%d_%d.mp3", seq, len(text))
	if err := s.media.Put(ctx, key, audio, "audio/mpeg"); err != nil {
		return "", err
	}
	return s.media.URL(key), nil
}

func (s *Service) CreateSession(ctx context.Context) (Session, error) {
	uuid, err := s.repo.CreateSession(ctx)
	if err != nil {
		return Session{}, err
	}
	return Session{UUID: uuid, Slots: map[string]any{}}, nil
}

func (s *Service) Message(ctx context.Context, in MessageInput) (MessageReply, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return MessageReply{}, errors.New("说话或输入内容不能为空")
	}
	if len([]rune(text)) > 200 {
		text = string([]rune(text)[:200])
	}
	raw, err := s.repo.GetSession(ctx, in.SessionID)
	if err != nil {
		return MessageReply{}, err
	}
	slots := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &slots)
	}
	parseSlots(text, slots)
	ready := containsAny(text, []string{"没有了", "就这些", "开始找", "找吧", "开始推荐"})
	intent := "collect"
	if ready {
		intent = "recommend"
	}
	if err := s.repo.UpdateSession(ctx, in.SessionID, slots, intent); err != nil {
		return MessageReply{}, err
	}
	reply, action := buildReply(slots, ready)
	if s.gw != nil && s.gw.Status().LLM {
		polished, err := s.gw.Chat(ctx, []modelgw.Message{
			{Role: "system", Content: "你是房产导购小亿，语气亲切、简短，一次只说一到两句，不要编造房源，不要询问或复述手机号。"},
			{Role: "user", Content: text},
		})
		if err == nil && strings.TrimSpace(polished) != "" {
			if r := []rune(strings.TrimSpace(polished)); len(r) > 120 {
				polished = string(r[:120])
			}
			reply = polished
		}
	}
	return MessageReply{
		SessionID: in.SessionID,
		Reply:     reply,
		Slots:     slots,
		Ready:     ready,
		Action:    action,
	}, nil
}

func (s *Service) Recommendations(ctx context.Context, sessionUUID string) (RecommendationResult, error) {
	raw, err := s.repo.GetSession(ctx, sessionUUID)
	if err != nil {
		return RecommendationResult{}, err
	}
	slots := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &slots)
	}
	cityCode := asString(slots["cityCode"])
	if cityCode == "" {
		cityCode = "440300"
	}
	candidates, err := s.repo.Candidates(ctx, cityCode)
	if err != nil {
		return RecommendationResult{}, err
	}

	budgetMax := asFloat(slots["budgetMaxCents"])
	areaMin := asFloat(slots["areaMin"])
	areaMax := asFloat(slots["areaMax"])
	district := asString(slots["district"])
	buildingType := asString(slots["buildingType"])

	type scored struct {
		item  RecommendationItem
		score int
	}
	all := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if district != "" && c.DistrictName != district {
			continue
		}
		if buildingType != "" && c.BuildingType != buildingType {
			continue
		}
		if areaMin > 0 && c.AreaSqm < areaMin*0.8 {
			continue
		}
		if areaMax > 0 && c.AreaSqm > areaMax*1.2 {
			continue
		}
		if budgetMax > 0 && float64(c.RentCents) > budgetMax*1.2 {
			continue
		}
		score := 0
		reasons := make([]string, 0, 4)
		if budgetMax > 0 {
			switch {
			case float64(c.RentCents) <= budgetMax:
				score += 30
				reasons = append(reasons, "在预算内")
			case float64(c.RentCents) <= budgetMax*1.1:
				score += 15
				reasons = append(reasons, "略超预算")
			}
		} else {
			score += 15
		}
		if areaMin > 0 || areaMax > 0 {
			if (areaMin == 0 || c.AreaSqm >= areaMin) && (areaMax == 0 || c.AreaSqm <= areaMax) {
				score += 20
				reasons = append(reasons, "面积匹配")
			}
		} else {
			score += 10
		}
		if district != "" {
			score += 20
			reasons = append(reasons, district)
		}
		if buildingType != "" && c.BuildingType == buildingType {
			score += 10
			reasons = append(reasons, buildingTypeLabel(buildingType))
		}
		if c.ImageCount > 0 {
			score += 5
			reasons = append(reasons, "有实景图")
		}
		all = append(all, scored{
			item: RecommendationItem{
				UUID: c.UUID, Title: c.Title, DistrictName: c.DistrictName, StreetName: c.StreetName,
				AreaSqm: c.AreaSqm, RentCents: c.RentCents, Layout: c.Layout,
				Score: score, Reasons: reasons, ImageCount: c.ImageCount,
			},
			score: score,
		})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].score == all[j].score {
			return all[i].item.RentCents < all[j].item.RentCents
		}
		return all[i].score > all[j].score
	})
	if len(all) > 20 {
		all = all[:20]
	}
	items := make([]RecommendationItem, 0, len(all))
	for _, s := range all {
		items = append(items, s.item)
	}
	filterJSON, _ := json.Marshal(slots)
	resultJSON, _ := json.Marshal(items)
	_ = s.repo.SaveRecommendation(ctx, sessionUUID, filterJSON, resultJSON)
	message := ""
	if len(items) == 0 {
		message = "这个预算和面积下暂时没有符合的房源，可以放宽预算或扩大面积。"
	}
	return RecommendationResult{SessionID: sessionUUID, Items: items, Message: message}, nil
}

func (s *Service) Narration(ctx context.Context, propertyUUID string) (Narration, error) {
	summary, err := s.repo.PropertySummary(ctx, propertyUUID)
	if err != nil {
		return Narration{}, err
	}
	images, err := s.repo.Images(ctx, propertyUUID)
	if err != nil {
		return Narration{}, err
	}
	videos, err := s.repo.Videos(ctx, propertyUUID)
	if err != nil {
		return Narration{}, err
	}

	segments := []NarrationSegment{}
	add := func(scene, text, action, imageID, mediaID string) {
		segments = append(segments, NarrationSegment{
			Seq: len(segments) + 1, Scene: scene, Text: text, Action: action,
			ImageID: imageID, MediaID: mediaID,
		})
	}
	add("overview", fmt.Sprintf("这套是%s，位于%s %s，%.0f㎡，月租%d元。",
		summary.Title, summary.DistrictName, summary.StreetName, summary.AreaSqm, summary.RentCents/100), "point", "", "")
	for i, key := range images {
		if i >= 6 {
			break
		}
		add("gallery", fmt.Sprintf("这是第%d张实景图。", i+1), "show_image", key, "")
	}
	if len(videos) > 0 {
		add("video", "这套有视频，带你看看实际空间。", "play_video", "", videos[0])
	}
	add("surroundings", fmt.Sprintf("再看周边，%s %s 的交通和配套。", summary.DistrictName, summary.StreetName), "open_map3d", "", "")
	add("contact", "如果满意，我可以帮你联系房东。", "show_contact", "", "")
	if err := s.repo.SaveNarration(ctx, propertyUUID, mustJSON(segments)); err != nil {
		return Narration{}, err
	}
	return Narration{PropertyUUID: propertyUUID, Segments: segments}, nil
}

func (s *Service) Tour(ctx context.Context, propertyUUID string) (Tour, error) {
	images, err := s.repo.Images(ctx, propertyUUID)
	if err != nil {
		return Tour{}, err
	}
	videos, err := s.repo.Videos(ctx, propertyUUID)
	if err != nil {
		return Tour{}, err
	}
	lat, lng, err := s.repo.Location(ctx, propertyUUID)
	if err != nil {
		return Tour{}, err
	}
	if s.media != nil {
		for i := range images {
			images[i] = s.media.URL(images[i])
		}
		for i := range videos {
			videos[i] = s.media.URL(videos[i])
		}
	}
	return Tour{PropertyUUID: propertyUUID, Images: images, Videos: videos, Lat: lat, Lng: lng}, nil
}

func parseSlots(text string, slots map[string]any) {
	if m := budgetRange.FindStringSubmatch(text); len(m) == 3 {
		min, _ := strconv.ParseFloat(m[1], 64)
		max, _ := strconv.ParseFloat(m[2], 64)
		slots["budgetMinCents"] = min * 100
		slots["budgetMaxCents"] = max * 100
	} else if m := budgetOne.FindStringSubmatch(text); len(m) == 2 {
		max, _ := strconv.ParseFloat(m[1], 64)
		slots["budgetMaxCents"] = max * 100
	}
	if m := areaRange.FindStringSubmatch(text); len(m) == 3 {
		min, _ := strconv.ParseFloat(m[1], 64)
		max, _ := strconv.ParseFloat(m[2], 64)
		slots["areaMin"] = min
		slots["areaMax"] = max
	} else if m := areaOne.FindStringSubmatch(text); len(m) == 2 {
		area, _ := strconv.ParseFloat(m[1], 64)
		slots["areaMin"] = area * 0.8
		slots["areaMax"] = area * 1.2
	}
	districts := []struct{ short, full string }{
		{"南山", "南山区"}, {"福田", "福田区"}, {"罗湖", "罗湖区"},
		{"宝安", "宝安区"}, {"龙岗", "龙岗区"}, {"龙华", "龙华区"},
	}
	for _, d := range districts {
		if strings.Contains(text, d.short) || strings.Contains(text, d.full) {
			slots["district"] = d.full
			break
		}
	}
	switch {
	case strings.Contains(text, "写字楼"), strings.Contains(text, "办公"):
		slots["buildingType"] = "office_building"
	case strings.Contains(text, "工业园"), strings.Contains(text, "厂房"), strings.Contains(text, "园区"):
		slots["buildingType"] = "industrial_park"
	}
}

func buildReply(slots map[string]any, ready bool) (string, string) {
	if ready {
		return "好的，我按这些条件找：预算、面积和区域已记录，马上给你排房源。", "recommend"
	}
	missing := make([]string, 0, 3)
	if asFloat(slots["budgetMaxCents"]) == 0 {
		missing = append(missing, "预算")
	}
	if asFloat(slots["areaMax"]) == 0 {
		missing = append(missing, "面积")
	}
	if asString(slots["district"]) == "" {
		missing = append(missing, "区域")
	}
	if len(missing) > 0 {
		return "我是小亿。还需要你补充：" + strings.Join(missing, "、") + "。", ""
	}
	return "记下了。还有别的要求吗？没有的话说“没有了”，我就开始匹配。", ""
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func buildingTypeLabel(t string) string {
	if t == "industrial_park" {
		return "工业园区"
	}
	return "写字楼"
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
