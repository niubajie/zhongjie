package assistant

type Session struct {
	UUID       string         `json:"sessionId"`
	VoiceToken string         `json:"voiceToken"`
	Slots      map[string]any `json:"slots"`
}

type MessageInput struct {
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
}

type MessageReply struct {
	SessionID string         `json:"sessionId"`
	Reply     string         `json:"reply"`
	Slots     map[string]any `json:"slots"`
	Ready     bool           `json:"ready"`
	Action    string         `json:"action,omitempty"`
}

type RecommendationItem struct {
	UUID         string   `json:"uuid"`
	Title        string   `json:"title"`
	DistrictName string   `json:"districtName"`
	StreetName   string   `json:"streetName"`
	AreaSqm      float64  `json:"areaSqm"`
	RentCents    int64    `json:"rentCents"`
	Layout       string   `json:"layout,omitempty"`
	Score        int      `json:"score"`
	Reasons      []string `json:"reasons"`
	ImageCount   int      `json:"imageCount"`
}

type RecommendationResult struct {
	SessionID string               `json:"sessionId"`
	Items     []RecommendationItem `json:"items"`
	Message   string               `json:"message,omitempty"`
}

type NarrationSegment struct {
	Seq     int    `json:"seq"`
	Scene   string `json:"scene"`
	Text    string `json:"text"`
	Action  string `json:"action"`
	ImageID string `json:"imageId,omitempty"`
	MediaID string `json:"mediaId,omitempty"`
}

type Narration struct {
	PropertyUUID string             `json:"propertyId"`
	Segments     []NarrationSegment `json:"segments"`
}

type Tour struct {
	PropertyUUID string   `json:"propertyId"`
	Images       []string `json:"images"`
	Videos       []string `json:"videos"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
}
