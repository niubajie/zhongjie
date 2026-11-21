package intake

import "errors"

var (
	ErrLandlordNotFound  = errors.New("未找到该手机号对应的房东")
	ErrCommunityNotFound = errors.New("未识别到已收录的小区")
	ErrMissingFields     = errors.New("缺少小区、面积或租金")
	ErrJobNotFound       = errors.New("录入任务不存在")
	ErrJobNotEditable    = errors.New("录入任务当前状态不可修改")
	ErrUnsupportedMedia  = errors.New("仅支持 jpg/png/webp 图片、mp4/mov 视频或 docx/txt/md 文档")
)

type TextInput struct {
	Text          string `json:"text"`
	LandlordPhone string `json:"landlordPhone"`
}

type Result struct {
	JobID         string `json:"jobId"`
	PropertyUUID  string `json:"propertyId,omitempty"`
	Status        string `json:"status"`
	LandlordUUID  string `json:"landlordId,omitempty"`
	CommunityName string `json:"communityName,omitempty"`
	Message       string `json:"message,omitempty"`
}

type MediaResult struct {
	JobID        string    `json:"jobId"`
	Status       string    `json:"status"`
	MatchStatus  string    `json:"matchStatus"`
	MediaURLs    []string  `json:"mediaUrls"`
	Extracted    Extracted `json:"extracted"`
	MaskedPhone  string    `json:"maskedPhone,omitempty"`
	LandlordUUID string    `json:"landlordId,omitempty"`
	Confidence   float64   `json:"confidence"`
	Message      string    `json:"message,omitempty"`
	OCRText      string    `json:"ocrText,omitempty"`
}

type Job struct {
	ID           int64
	UUID         string
	InputType    string
	Status       string
	MatchStatus  string
	Extracted    Extracted
	MediaKeys    []string
	OCRText      []byte
	Transcript   []byte
	LandlordID   int64
	LandlordUUID string
	PropertyUUID string
	Confidence   float64
	ErrorMessage string
}

// JobSummary is a lightweight draft row for the admin console list.
type JobSummary struct {
	UUID          string  `json:"uuid"`
	InputType     string  `json:"inputType"`
	Status        string  `json:"status"`
	MatchStatus   string  `json:"matchStatus"`
	CommunityName string  `json:"communityName,omitempty"`
	LandlordUUID  string  `json:"landlordId,omitempty"`
	PropertyUUID  string  `json:"propertyId,omitempty"`
	Confidence    float64 `json:"confidence"`
	ErrorMessage  string  `json:"errorMessage,omitempty"`
	AreaSqm       float64 `json:"areaSqm"`
	RentYuan      float64 `json:"rentYuan"`
	Title         string  `json:"title,omitempty"`
	CreatedAt     string  `json:"createdAt"`
}

type MatchInput struct {
	LandlordUUID string `json:"landlordId"`
	Reason       string `json:"reason"`
}

type PublishInput struct {
	CommunityUUID string   `json:"communityUuid"`
	CommunityName string   `json:"communityName"`
	AreaSqm       float64  `json:"areaSqm"`
	RentCents     int64    `json:"rentCents"`
	Layout        string   `json:"layout"`
	Floor         string   `json:"floor"`
	Orientation   string   `json:"orientation"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	ImageKeys     []string `json:"imageKeys"`
}
