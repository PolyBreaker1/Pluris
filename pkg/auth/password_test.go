package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword (wrong password) failed: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to not verify")
	}
}

func TestHashPasswordProducesDifferentSaltsEachTime(t *testing.T) {
	h1, _ := HashPassword("same password")
	h2, _ := HashPassword("same password")
	if h1 == h2 {
		t.Fatal("expected different hashes for the same password (random salt)")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash-at-all",
		"$argon2id$v=19$m=19456,t=2,p=1$onlyfourparts",
		"$bcrypt$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA",
		"$argon2id$vBAD$m=19456,t=2,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$mBADSEGMENT$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$!!!notbase64!!!$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$!!!notbase64!!!",
	}
	for _, encoded := range cases {
		ok, err := VerifyPassword("anything", encoded)
		if err == nil && ok {
			t.Errorf("VerifyPassword(%q) unexpectedly succeeded", encoded)
		}
	}
}

func TestHashAndVerifyEmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword(\"\") failed: %v", err)
	}
	ok, err := VerifyPassword("", hash)
	if err != nil || !ok {
		t.Fatalf("expected empty password to verify against its own hash, got ok=%v err=%v", ok, err)
	}
}

func TestHashAndVerifyLongPassword(t *testing.T) {
	long := strings.Repeat("a", 1000)
	hash, err := HashPassword(long)
	if err != nil {
		t.Fatalf("HashPassword(long) failed: %v", err)
	}
	ok, err := VerifyPassword(long, hash)
	if err != nil || !ok {
		t.Fatalf("expected long password to verify, got ok=%v err=%v", ok, err)
	}
}
