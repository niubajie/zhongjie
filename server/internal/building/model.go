package building

import "errors"

var ErrNotFound = errors.New("building not found")

type Building struct {
	ID           int64
	UUID         string
	CityCode     string
	Name         string
	DistrictCode string
	DistrictName string
	StreetCode   string
	StreetName   string
	BuildingType string
	Lat          float64
	Lng          float64
	ReviewStatus string
}

type SuggestItem struct {
	UUID         string  `json:"uuid"`
	Name         string  `json:"name"`
	DistrictCode string  `json:"districtCode"`
	DistrictName string  `json:"districtName"`
	StreetCode   string  `json:"streetCode"`
	StreetName   string  `json:"streetName"`
	BuildingType string  `json:"buildingType"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
}

func (b Building) Suggest() SuggestItem {
	return SuggestItem{
		UUID:         b.UUID,
		Name:         b.Name,
		DistrictCode: b.DistrictCode,
		DistrictName: b.DistrictName,
		StreetCode:   b.StreetCode,
		StreetName:   b.StreetName,
		BuildingType: b.BuildingType,
		Lat:          b.Lat,
		Lng:          b.Lng,
	}
}
