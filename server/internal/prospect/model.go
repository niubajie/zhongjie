package prospect

import (
	"errors"
	"time"
)

// 自主获客模块（方案 3.12 / 3.13，M10）。
// 本包只放数据模型与基础读写；采集 Worker、LLM 评分、人工复核联系、
// 跟进状态机在后续批次的 collector.go / scoring.go / followup.go 中补齐。
// 停机开关默认关闭，主链路零影响。

var (
	ErrSourceNotFound       = errors.New("信息源不存在")
	ErrLeadNotFound         = errors.New("线索不存在")
	ErrDuplicateLead        = errors.New("线索已存在")
	ErrDuplicateSource      = errors.New("信息源名称已存在")
	ErrInvalidSourceInput   = errors.New("信息源参数不合法")
	ErrOptOutExists         = errors.New("该联系方式已在拒收名单")
	ErrInvalidStatus        = errors.New("无效的线索状态")
	ErrInvalidTransition    = errors.New("不允许的状态转换")
	ErrHalted               = errors.New("全局停机开关已开启，暂停联系")
	ErrInvalidReview        = errors.New("无效的复核结论")
	ErrInvalidContact       = errors.New("联系参数不合法")
	ErrInvalidFollowUp      = errors.New("跟进参数不合法")
	ErrNotificationNotFound = errors.New("通知不存在")
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

// ReviewScoreThreshold 意向分阈值（F5）：>= 该值且意向有效才进人工复核队列。
const ReviewScoreThreshold = 70

// UnscoredLead 待评分线索：status=new 且 intent_status=unknown。
// RawTextEnc 为密文，评分前由 Scorer 解密（只在内存出现明文）。
type UnscoredLead struct {
	UUID           string
	LeadType       string
	RawTextEnc     []byte
	ContactChannel string
	CityCode       string
	DistrictCode   string
	BudgetCents    int64
	AreaSqm        float64
	Layout         string
	CreatedAt      string
}

// 复核结论（批次 5）：通过 → reviewed，拒绝 → rejected。
const (
	ReviewDecisionApprove = "approve"
	ReviewDecisionReject  = "reject"
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

// adminTargetStatuses 管理站可手动流转的目标状态（批次 6）。
// reviewed / contacted 有专用接口，opt_out 必须走拒收接口，均不在此列。
var adminTargetStatuses = map[string]bool{
	StatusFollowingUp: true,
	StatusConverted:   true,
	StatusPublished:   true,
	StatusCold:        true,
	StatusLost:        true,
	StatusRejected:    true,
	StatusIgnored:     true,
}

// ValidAdminTargetStatus 判断目标状态是否允许通过通用流转接口设置。
func ValidAdminTargetStatus(s string) bool {
	return adminTargetStatuses[s]
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
	RawTextEnc        []byte
	ContactHash       []byte
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
	StaleAt           string
	CreatedAt         string
	UpdatedAt         string
}

// FollowUp 跟进记录。
type FollowUp struct {
	ID         int64
	LeadID     int64
	Channel    string
	Direction  string
	Status     string
	ContentEnc []byte
	SentAt     string
	AuditID    int64
	CreatedAt  string
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
	Stale             bool    `json:"stale"`
	StaleAt           string  `json:"staleAt,omitempty"`
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

// ---- 批次 5：人工复核与首条联系 ----

// ReviewLeadInput 复核结论。
type ReviewLeadInput struct {
	Decision string `json:"decision"`
}

// ContactLeadInput 首条联系内容由人工填写，服务端不会自动生成或发送。
type ContactLeadInput struct {
	Channel string `json:"channel"`
	Content string `json:"content"`
}

// FollowUpVO 跟进记录视图，content 为解密后的联系内容。
type FollowUpVO struct {
	ID        int64  `json:"id"`
	Channel   string `json:"channel"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
	Content   string `json:"content,omitempty"`
	SentAt    string `json:"sentAt,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// ProspectLeadDetailVO 线索详情：基础字段 + 解密原文 + 跟进记录。
// 读取即写审计日志。
type ProspectLeadDetailVO struct {
	ProspectLeadVO
	RawText   string       `json:"rawText"`
	FollowUps []FollowUpVO `json:"followUps"`
}

// ---- 批次 6：状态流转与拒收 ----

// TransitionLeadInput 手动流转目标状态。
type TransitionLeadInput struct {
	Status string `json:"status"`
}

// OptOutLeadInput 拒收录入，原因可选。
type OptOutLeadInput struct {
	Reason string `json:"reason,omitempty"`
}

// OptOutVO 拒收记录视图；不返回 contact_hash，避免不可逆标识外泄。
type OptOutVO struct {
	ID         int64  `json:"id"`
	LeadType   string `json:"leadType"`
	Reason     string `json:"reason,omitempty"`
	OptedOutAt string `json:"optedOutAt"`
}

// ---- 批次 8：后续跟进与提醒 ----

// 跟进提醒阈值：3 天未跟进生成待跟进提醒，7 天未跟进标记超期。
const (
	FollowUpDueAfter     = 3 * 24 * time.Hour
	FollowUpOverdueAfter = 7 * 24 * time.Hour
)

// 提醒级别。
const (
	ReminderLevelDue3d     = "due_3d"
	ReminderLevelOverdue7d = "overdue_7d"
)

// AddFollowUpInput 后续跟进记录，内容由人工填写。
type AddFollowUpInput struct {
	Channel   string `json:"channel"`
	Direction string `json:"direction,omitempty"`
	Content   string `json:"content"`
}

// FollowUpCandidate Worker 扫描对象：最后活动时间与当前超期标记。
type FollowUpCandidate struct {
	ID           int64
	UUID         string
	LeadType     string
	Status       string
	LastActivity time.Time
	Stale        bool
}

// Reminder 未处理的跟进提醒。
type Reminder struct {
	ID         int64
	LeadUUID   string
	LeadType   string
	LeadStatus string
	Level      string
	CreatedAt  string
}

// ReminderVO 提醒视图，不返回联系方式等敏感字段。
type ReminderVO struct {
	ID         int64  `json:"id"`
	LeadUUID   string `json:"leadUuid"`
	LeadType   string `json:"leadType"`
	LeadStatus string `json:"leadStatus"`
	Level      string `json:"level"`
	CreatedAt  string `json:"createdAt"`
}

// ReminderStats Worker 单轮扫描统计。
type ReminderStats struct {
	Scanned int `json:"scanned"`
	Due     int `json:"due"`
	Overdue int `json:"overdue"`
	Stale   int `json:"stale"`
}

// ---- 批次 10：获客效果统计 ----

// StatusCount 状态分布。
type StatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// IntentCount 意向分布。
type IntentCount struct {
	IntentStatus string `json:"intentStatus"`
	Count        int64  `json:"count"`
}

// ProspectStatsVO 获客效果统计。转化率 = 已转化 / 至少有一条外呼跟进的线索数。
type ProspectStatsVO struct {
	TotalLeads     int64         `json:"totalLeads"`
	ReviewPending  int64         `json:"reviewPending"`
	ContactedLeads int64         `json:"contactedLeads"`
	ConvertedLeads int64         `json:"convertedLeads"`
	ConversionRate float64       `json:"conversionRate"`
	StaleLeads     int64         `json:"staleLeads"`
	OpenReminders  int64         `json:"openReminders"`
	ByStatus       []StatusCount `json:"byStatus"`
	ByIntent       []IntentCount `json:"byIntent"`
}

// ---- 批次 11：管理员站内通知 ----

// AdminNotification 站内通知实体。
type AdminNotification struct {
	ID        int64
	EventType string
	Level     string
	Title     string
	Body      string
	LeadUUID  string
	Status    string
	CreatedAt string
}

// AdminNotificationVO 站内通知视图。
type AdminNotificationVO struct {
	ID        int64  `json:"id"`
	EventType string `json:"eventType"`
	Level     string `json:"level"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	LeadUUID  string `json:"leadUuid,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// AdminNotificationListVO 通知列表与未读数。
type AdminNotificationListVO struct {
	Items  []AdminNotificationVO `json:"items"`
	Unread int64                 `json:"unread"`
}
