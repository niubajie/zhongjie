package property

import (
	"testing"

	"housing/internal/community"
)

func TestPublishDecisionRequiresCommunity(t *testing.T) {
	communityID := int64(7)
	buildingID := int64(3)
	cases := []struct {
		name   string
		place  PublishPlace
		status string
		reason string
	}{
		{
			name:   "matched community auto-publishes",
			place:  PublishPlace{CommunityID: &communityID, CommunityName: "幸福里"},
			status: "active",
			reason: "",
		},
		{
			name:   "legacy building without community stays pending",
			place:  PublishPlace{BuildingID: &buildingID, BuildingName: "某写字楼", BuildingType: "office_building"},
			status: "pending",
			reason: "missing_community_name",
		},
		{
			name:   "no place stays pending",
			place:  PublishPlace{},
			status: "pending",
			reason: "missing_community_name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := publishDecision(tc.place)
			if status != tc.status || reason != tc.reason {
				t.Fatalf("publishDecision = (%q, %q), want (%q, %q)", status, reason, tc.status, tc.reason)
			}
		})
	}
}

func TestPlaceFromCommunityCopiesLocationAndType(t *testing.T) {
	c := community.Community{
		ID:           9,
		UUID:         "0193f0c4-0000-7000-8000-000000000009",
		CityCode:     "440300",
		DistrictCode: "440303",
		DistrictName: "罗湖区",
		StreetCode:   "440303001",
		StreetName:   "桂园街道",
		Name:         "幸福里",
		Type:         "residential",
		Lat:          22.5431,
		Lng:          114.0579,
	}
	place := placeFromCommunity(c)
	if place.CommunityID == nil || *place.CommunityID != c.ID {
		t.Fatalf("CommunityID = %v, want %d", place.CommunityID, c.ID)
	}
	if place.CommunityName != c.Name || place.BuildingName != c.Name {
		t.Fatalf("community/building name = (%q, %q), want %q", place.CommunityName, place.BuildingName, c.Name)
	}
	if place.BuildingType != c.Type {
		t.Fatalf("BuildingType = %q, want %q", place.BuildingType, c.Type)
	}
	if place.DistrictCode != c.DistrictCode || place.StreetName != c.StreetName {
		t.Fatalf("location = (%q, %q), want (%q, %q)", place.DistrictCode, place.StreetName, c.DistrictCode, c.StreetName)
	}
	if place.Lat != c.Lat || place.Lng != c.Lng {
		t.Fatalf("lat/lng = (%v, %v), want (%v, %v)", place.Lat, place.Lng, c.Lat, c.Lng)
	}
}
