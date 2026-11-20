package contact

import "errors"

var (
	ErrConsent          = errors.New("需要确认联系房东")
	ErrPropertyNotFound = errors.New("房源不存在或已下架")
	ErrPhoneRequired    = errors.New("请先绑定手机号")
	ErrAlreadyContacted = errors.New("7 天内已经联系过该房东")
	ErrDailyLimit       = errors.New("今天联系次数已达上限")
)

type CreateInput struct {
	PropertyUUID string `json:"propertyUuid"`
	Consent      bool   `json:"consent"`
	Note         string `json:"note"`
}

type Reveal struct {
	RequestUUID        string `json:"requestUuid"`
	Phone              string `json:"phone,omitempty"`
	WechatID           string `json:"wechatId,omitempty"`
	ContactAvailable   bool   `json:"contactAvailable"`
	NotificationStatus string `json:"notificationStatus"`
	Message            string `json:"message,omitempty"`
}
