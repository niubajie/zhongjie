package auth

import (
	"strings"
	"testing"
	"time"
)

func TestIssueAndParse(t *testing.T) {
	m, err := NewManager(strings.Repeat("x", 32), time.Minute)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	token, exp, err := m.Issue(42, "0190f0b0-0000-7000-8000-000000000001", "tenant")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if exp.Before(time.Now()) {
		t.Fatal("token already expired")
	}
	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Subject != "42" || claims.Role != "tenant" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestRejectWrongIssuerOrSecret(t *testing.T) {
	m, _ := NewManager(strings.Repeat("a", 32), time.Minute)
	token, _, _ := m.Issue(1, "uuid", "landlord")
	other, _ := NewManager(strings.Repeat("b", 32), time.Minute)
	if _, err := other.Parse(token); err == nil {
		t.Fatal("expected parse error with different secret")
	}
}
