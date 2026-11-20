package admin

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("Admin123!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "Admin123!" {
		t.Fatal("password stored in plaintext")
	}
	ok, err := VerifyPassword("Admin123!", hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword correct: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword wrong: %v", err)
	}
	if ok {
		t.Fatal("wrong password accepted")
	}
}

func TestRejectShortPassword(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password error")
	}
}
