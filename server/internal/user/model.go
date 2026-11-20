package user

import "time"

type User struct {
	ID               int64
	UUID             string
	OpenID           string
	UnionID          string
	PhoneMask        string
	Surname          string
	Role             string
	Status           string
	ProfileCompleted bool
	CreatedAt        time.Time
}

type AuthResult struct {
	Token            string     `json:"token"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	ProfileCompleted bool       `json:"profileCompleted"`
	User             PublicUser `json:"user"`
}

type PublicUser struct {
	UUID             string `json:"uuid"`
	PhoneMask        string `json:"phoneMask,omitempty"`
	Surname          string `json:"surname,omitempty"`
	Role             string `json:"role,omitempty"`
	Status           string `json:"status"`
	ProfileCompleted bool   `json:"profileCompleted"`
}

func (u User) Public() PublicUser {
	return PublicUser{
		UUID:             u.UUID,
		PhoneMask:        u.PhoneMask,
		Surname:          u.Surname,
		Role:             u.Role,
		Status:           u.Status,
		ProfileCompleted: u.ProfileCompleted,
	}
}
