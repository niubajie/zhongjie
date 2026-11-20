package freshness

import "errors"

var (
	ErrNotFound      = errors.New("property not found")
	ErrNotOwner      = errors.New("not the property owner")
	ErrInvalidStatus = errors.New("status must be active|rented|offline")
)

type RespondInput struct {
	Status string `json:"status"`
}

type DueResult struct {
	Created int64 `json:"created"`
}
