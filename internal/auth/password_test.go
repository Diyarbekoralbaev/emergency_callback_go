package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword(hash, "secret123") {
		t.Fatal("verify should succeed")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("verify should fail for wrong password")
	}
}

func TestEmptyPassword(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password should error")
	}
}
