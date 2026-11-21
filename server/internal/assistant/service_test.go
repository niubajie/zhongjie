package assistant

import "testing"

func TestVoiceTokenBoundToSession(t *testing.T) {
	s := &Service{voiceSecret: []byte("test-voice-secret")}
	token := s.voiceToken("session-a")
	if !s.ValidateVoiceToken("session-a", token) {
		t.Fatal("valid session token rejected")
	}
	if s.ValidateVoiceToken("session-b", token) {
		t.Fatal("token accepted for another session")
	}
	if s.ValidateVoiceToken("session-a", "wrong") {
		t.Fatal("wrong token accepted")
	}
	if s.ValidateVoiceToken("session-a", "") {
		t.Fatal("empty token accepted")
	}
}
