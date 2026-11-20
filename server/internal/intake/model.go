package intake

import "errors"

var (
	ErrLandlordNotFound = errors.New("未找到该手机号对应的房东")
	ErrBuildingNotFound = errors.New("未识别到已收录的工业园区/写字楼")
	ErrMissingFields    = errors.New("缺少楼宇、面积或租金")
)

type TextInput struct {
	Text          string `json:"text"`
	LandlordPhone string `json:"landlordPhone"`
}

type Result struct {
	JobID        string `json:"jobId"`
	PropertyUUID string `json:"propertyId,omitempty"`
	Status       string `json:"status"`
	LandlordUUID string `json:"landlordId,omitempty"`
	BuildingName string `json:"buildingName,omitempty"`
	Message      string `json:"message,omitempty"`
}
