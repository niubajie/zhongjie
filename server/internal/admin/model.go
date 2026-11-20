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
	UUID     string `json:"uuid"`
	Username string `json:"username"`
	Role     string `json:"role"`
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

func (a Admin) Public() PublicAdmin {
	return PublicAdmin{UUID: a.UUID, Username: a.Username, Role: a.Role}
}
