package community

import "errors"

var ErrNotFound = errors.New("community not found")

type Community struct {
	ID           int64
	UUID         string
	CityCode     string
	DistrictCode string
	DistrictName string
	StreetCode   string
	StreetName   string
	Name         string
	Aliases      []string
	Type         string
	POIID        string
	Source       string
	ReviewStatus string
	Lat          float64
	Lng          float64
}

type SuggestItem struct {
	UUID         string  `json:"uuid"`
	Name         string  `json:"name"`
	DistrictCode string  `json:"districtCode"`
	DistrictName string  `json:"districtName"`
	StreetCode   string  `json:"streetCode"`
	StreetName   string  `json:"streetName"`
	Type         string  `json:"communityType"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
}

func (c Community) Suggest() SuggestItem {
	return SuggestItem{
		UUID:         c.UUID,
		Name:         c.Name,
		DistrictCode: c.DistrictCode,
		DistrictName: c.DistrictName,
		StreetCode:   c.StreetCode,
		StreetName:   c.StreetName,
		Type:         c.Type,
		Lat:          c.Lat,
		Lng:          c.Lng,
	}
}

type CreateInput struct {
	CityCode     string   `json:"cityCode"`
	DistrictCode string   `json:"districtCode"`
	DistrictName string   `json:"districtName"`
	StreetCode   string   `json:"streetCode"`
	StreetName   string   `json:"streetName"`
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	Type         string   `json:"communityType"`
	POIID        string   `json:"poiId"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
}
