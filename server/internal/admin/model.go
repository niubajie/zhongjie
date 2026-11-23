package admin

import "time"

type Admin struct {
	ID            int64
	UUID          string
	Username      string
	PasswordHash  string
	Role          string
	Status        string
	TOTPEnabled   bool
	TOTPSecretEnc []byte
	LastLoginAt   *time.Time
}

type PublicAdmin struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	TOTPEnabled bool   `json:"totpEnabled"`
}

type LoginResult struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expiresAt"`
	Admin     PublicAdmin `json:"admin"`
}

type Stats struct {
	ActiveProperties  int64 `json:"activeProperties"`
	PendingProperties int64 `json:"pendingProperties"`
	Users             int64 `json:"users"`
	Landlords         int64 `json:"landlords"`
	ContactRequests   int64 `json:"contactRequests"`
}

type PropertyRow struct {
	UUID         string  `json:"uuid"`
	Title        string  `json:"title"`
	Status       string  `json:"status"`
	DistrictName string  `json:"districtName"`
	StreetName   string  `json:"streetName"`
	AreaSqm      float64 `json:"areaSqm"`
	RentCents    int64   `json:"rentCents"`
	CreatedAt    string  `json:"createdAt"`
}

type ImportJobRow struct {
	UUID        string `json:"uuid"`
	FileName    string `json:"fileName"`
	FileType    string `json:"fileType"`
	Status      string `json:"status"`
	TotalRows   int    `json:"totalRows"`
	SuccessRows int    `json:"successRows"`
	ErrorRows   int    `json:"errorRows"`
	CreatedAt   string `json:"createdAt"`
}

type BuildingRow struct {
	UUID         string  `json:"uuid"`
	Name         string  `json:"name"`
	DistrictName string  `json:"districtName"`
	StreetName   string  `json:"streetName"`
	BuildingType string  `json:"buildingType"`
	ReviewStatus string  `json:"reviewStatus"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
}

type CreateBuildingInput struct {
	Name         string  `json:"name"`
	DistrictCode string  `json:"districtCode"`
	DistrictName string  `json:"districtName"`
	StreetCode   string  `json:"streetCode"`
	StreetName   string  `json:"streetName"`
	BuildingType string  `json:"buildingType"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
}

type ContactRow struct {
	UUID            string `json:"uuid"`
	TenantSurname   string `json:"tenantSurname"`
	TenantPhoneMask string `json:"tenantPhoneMask"`
	PropertyTitle   string `json:"propertyTitle"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
}

type ContactDetail struct {
	ContactRow
	Note string `json:"note,omitempty"`
}

type AuditLogRow struct {
	ID         int64  `json:"id"`
	ActorType  string `json:"actorType"`
	Action     string `json:"action"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type FreshnessRow struct {
	ID           int64  `json:"id"`
	PropertyUUID string `json:"propertyUuid"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
}

func (a Admin) Public() PublicAdmin {
	return PublicAdmin{UUID: a.UUID, Username: a.Username, Role: a.Role, TOTPEnabled: a.TOTPEnabled}
}
