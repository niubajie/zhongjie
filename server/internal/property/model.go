package property

import "errors"

var ErrNotFound = errors.New("property not found")

type MapQuery struct {
	CityCode string
	SWLat    float64
	SWLng    float64
	NELat    float64
	NELng    float64
	Zoom     int
}

type MapCluster struct {
	ID           string  `json:"id"`
	DistrictCode string  `json:"districtCode"`
	DistrictName string  `json:"districtName"`
	StreetCode   string  `json:"streetCode"`
	StreetName   string  `json:"streetName"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	Count        int64   `json:"count"`
	Level        string  `json:"level"`
}

type MapItem struct {
	UUID         string  `json:"uuid"`
	DistrictCode string  `json:"districtCode"`
	DistrictName string  `json:"districtName"`
	StreetCode   string  `json:"streetCode"`
	StreetName   string  `json:"streetName"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	AreaSqm      float64 `json:"areaSqm"`
	RentCents    int64   `json:"rentCents"`
	Layout       string  `json:"layout,omitempty"`
	Title        string  `json:"title"`
}

type MapResult struct {
	CityCode string       `json:"cityCode"`
	Zoom     int          `json:"zoom"`
	Clusters []MapCluster `json:"clusters"`
	Items    []MapItem    `json:"items"`
}

type Image struct {
	ObjectKey string `json:"objectKey"`
	URL       string `json:"url,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Sort      int    `json:"sort"`
}

// MineItem is one row of the landlord's own listing list.
type MineItem struct {
	UUID            string  `json:"uuid"`
	Title           string  `json:"title"`
	CommunityName   string  `json:"communityName,omitempty"`
	Status          string  `json:"status"`
	PendingReason   string  `json:"pendingReason,omitempty"`
	AreaSqm         float64 `json:"areaSqm"`
	RentCents       int64   `json:"rentCents"`
	LastConfirmedAt string  `json:"lastConfirmedAt,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	NeedsConfirm    bool    `json:"needsConfirm"`
}

type CreateInput struct {
	CommunityUUID string   `json:"communityUuid"`
	CommunityName string   `json:"communityName"`
	BuildingUUID  string   `json:"buildingUuid"`
	AreaSqm       float64  `json:"areaSqm"`
	RentCents     int64    `json:"rentCents"`
	Layout        string   `json:"layout"`
	Floor         string   `json:"floor"`
	Orientation   string   `json:"orientation"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	ImageKeys     []string `json:"imageKeys"`
}

// PublishPlace is the resolved location of a listing. Residential listings set
// CommunityID; legacy non-residential listings may set BuildingID instead.
type PublishPlace struct {
	CityCode      string
	DistrictCode  string
	DistrictName  string
	StreetCode    string
	StreetName    string
	Lat           float64
	Lng           float64
	BuildingID    *int64
	BuildingName  string
	BuildingType  string
	CommunityID   *int64
	CommunityName string
}

type Created struct {
	UUID          string `json:"uuid"`
	Status        string `json:"status"`
	PendingReason string `json:"pendingReason,omitempty"`
}

type Detail struct {
	UUID            string  `json:"uuid"`
	DistrictCode    string  `json:"districtCode"`
	DistrictName    string  `json:"districtName"`
	StreetCode      string  `json:"streetCode"`
	StreetName      string  `json:"streetName"`
	AreaSqm         float64 `json:"areaSqm"`
	RentCents       int64   `json:"rentCents"`
	Layout          string  `json:"layout,omitempty"`
	Floor           string  `json:"floor,omitempty"`
	Orientation     string  `json:"orientation,omitempty"`
	Title           string  `json:"title"`
	Description     string  `json:"description,omitempty"`
	BuildingType    string  `json:"buildingType"`
	PublishedAt     string  `json:"publishedAt,omitempty"`
	LastConfirmedAt string  `json:"lastConfirmedAt,omitempty"`
	Images          []Image `json:"images"`
}
