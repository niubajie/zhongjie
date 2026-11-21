package prospect

import "errors"

// 自主获客模块（方案 3.12 / 3.13，M10）。
// 本包只放数据模型与基础读写；采集 Worker、LLM 评分、人工复核联系、
// 跟进状态机在后续批次的 collector.go / scoring.go / followup.go 中补齐。
// 停机开关默认关闭，主链路零影响。

var (
	ErrSourceNotFound     = errors.New("信息源不存在")
	ErrLeadNotFound       = errors.New("线索不存在")
	ErrDuplicateLead      = errors.New("线索已存在")
	ErrDuplicateSource    = errors.New("信息源名称已存在")
	ErrInvalidSourceInput = errors.New("信息源参数不合法")
	ErrOptOutExists       = errors.New("该联系方式已在拒收名单")
	ErrInvalidStatus      = errors.New("无效的线索状态")
	ErrInvalidTransition  = errors.New("不允许的状态转换")
)

// 线索类型
const (
	LeadTypeTenant   = "tenant"
	LeadTypeLandlord = "landlord"
)

// 线索状态机（方案 4.2）：
//
//	new → reviewed → contacted → following_up → converted / published / opt_out
//	new / reviewed → rejected / ignored
//	contacted / following_up → cold / lost
const (
	StatusNew         = "new"
	StatusReviewed    = "reviewed"
	StatusContacted   = "contacted"
	StatusFollowingUp = "following_up"
	StatusConverted   = "converted"
	StatusPublished   = "published"
	StatusCold        = "cold"
	StatusLost        = "lost"
	StatusOptOut      = "opt_out"
	StatusRejected    = "rejected"
	StatusIgnored     = "ignored"
)

// 意向分类结果（批次 4 LLM 写入）
const (
	IntentSeeking         = "seeking"
	IntentRented          = "rented"
	IntentLandlordListing = "landlord_listing"
	IntentAgent           = "agent"
	IntentIrrelevant      = "irrelevant"
	IntentUnknown         = "unknown"
)

// 联系渠道与方向（首条联系 100% 人工审核）
const (
	ChannelInApp   = "in_app"
	ChannelSMS     = "sms"
	ChannelWeChat  = "wechat"
	ChannelPhone   = "phone"
	ChannelUnknown = "unknown"

	DirectionOutbound = "outbound"
	DirectionInbound  = "inbound"
)

// 信息源类型（与 lead_sources.kind 的 CHECK 约束一致）。
const (
	SourceKindSelfOwned        = "self_owned"
	SourceKindPublicAPI        = "public_api"
	SourceKindThirdPartyScrape = "third_party_scrape"
)

// 终态：进入后不可再转换
var terminalStatuses = map[string]bool{
	StatusConverted: true,
	StatusPublished: true,
	StatusCold:      true,
	StatusLost:      true,
	StatusOptOut:    true,
	StatusRejected:  true,
	StatusIgnored:   true,
}

// allowedTransitions 定义合法的状态转换（方案 4.2）。
var allowedTransitions = map[string]map[string]bool{
	StatusNew: {
		StatusReviewed: true,
		StatusRejected: true,
		StatusIgnored:  true,
		StatusOptOut:   true,
	},
	StatusReviewed: {
		StatusContacted: true,
		StatusRejected:  true,
		StatusIgnored:   true,
		StatusOptOut:    true,
	},
	StatusContacted: {
		StatusFollowingUp: true,
		StatusConverted:   true,
		StatusPublished:   true,
		StatusCold:        true,
		StatusLost:        true,
		StatusOptOut:      true,
	},
	StatusFollowingUp: {
		StatusConverted: true,
		StatusPublished: true,
		StatusCold:      true,
		StatusLost:      true,
		StatusOptOut:    true,
	},
}

// ValidStatus 判断状态值是否在枚举内。
func ValidStatus(s string) bool {
	if _, ok := allowedTransitions[s]; ok {
		return true
	}
	return terminalStatuses[s]
}

// CanTransition 判断 from → to 是否为合法转换。
func CanTransition(from, to string) bool {
	if !ValidStatus(from) || !ValidStatus(to) {
		return false
	}
	if from == to {
		return false
	}
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// LeadSource 信息源配置，也是单源停机开关的载体。
type LeadSource struct {
	ID          int64
	UUID        string
	Name        string
	Kind        string
	BaseURL     string
	RobotsOK    bool
	TermsOK     bool
	RateLimitMs int
	Enabled     bool
	ReviewNote  string
	CreatedAt   string
	UpdatedAt   string
}

// ProspectLead 潜在线索。raw_text_enc / contact_hash 为密文与哈希，
// 列表 VO 不返回原文，详情页仅授权管理员解密展示（批次 5）。
type ProspectLead struct {
	ID                int64
	UUID              string
	SourceID          int64
	SourceName        string
	LeadType          string
	Platform          string
	ExternalPostID    string
	ContactChannel    string
	CityCode          string
	DistrictCode      string
	BudgetCents       int64
	AreaSqm           float64
	Layout            string
	IntentScore       int
	IntentStatus      string
	Status            string
	MatchedUserID     int64
	MatchedPropertyID string
	CreatedAt         string
	UpdatedAt         string
}

// FollowUp 跟进记录。
type FollowUp struct {
	ID        int64
	LeadID    int64
	Channel   string
	Direction string
	Status    string
	SentAt    string
	AuditID   int64
	CreatedAt string
}

// OptOut 拒收记录，命中后全局停止联系。
type OptOut struct {
	ID          int64
	ContactHash []byte
	LeadType    string
	Reason      string
	OptedOutAt  string
}

// ---- DTO（API 输入输出，JSON 字段 camelCase）----

type CreateSourceInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	BaseURL     string `json:"baseUrl,omitempty"`
	RateLimitMs int    `json:"rateLimitMs,omitempty"`
	ReviewNote  string `json:"reviewNote,omitempty"`
}

type LeadSourceVO struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	BaseURL     string `json:"baseUrl,omitempty"`
	RobotsOK    bool   `json:"robotsOk"`
	TermsOK     bool   `json:"termsOk"`
	RateLimitMs int    `json:"rateLimitMs"`
	Enabled     bool   `json:"enabled"`
	ReviewNote  string `json:"reviewNote,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ProspectLeadVO 是对外返回的线索视图，不含密文字段。
type ProspectLeadVO struct {
	UUID              string  `json:"uuid"`
	SourceName        string  `json:"sourceName"`
	LeadType          string  `json:"leadType"`
	Platform          string  `json:"platform"`
	ContactChannel    string  `json:"contactChannel"`
	CityCode          string  `json:"cityCode,omitempty"`
	DistrictCode      string  `json:"districtCode,omitempty"`
	BudgetCents       int64   `json:"budgetCents,omitempty"`
	AreaSqm           float64 `json:"areaSqm,omitempty"`
	Layout            string  `json:"layout,omitempty"`
	IntentScore       int     `json:"intentScore,omitempty"`
	IntentStatus      string  `json:"intentStatus"`
	Status            string  `json:"status"`
	MatchedPropertyID string  `json:"matchedPropertyId,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

// ---- 批次 2：全局停机开关与信息源更新 ----

// SettingKeyGlobalHalt 全局停机开关的配置键。
const SettingKeyGlobalHalt = "global_halt"

// ProspectSetting 运行时配置键值对。
type ProspectSetting struct {
	Key       string
	Value     string
	UpdatedAt string
	UpdatedBy int64
}

// KillSwitchState 全局停机状态视图。
type KillSwitchState struct {
	Halted bool `json:"halted"`
}

// UpdateSourceInput 信息源更新（全量覆盖可编辑字段）。
type UpdateSourceInput struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	BaseURL     string `json:"baseUrl,omitempty"`
	RateLimitMs int    `json:"rateLimitMs,omitempty"`
	RobotsOK    bool   `json:"robotsOk"`
	TermsOK     bool   `json:"termsOk"`
	ReviewNote  string `json:"reviewNote,omitempty"`
}
