package domain

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testSigner() *Signer {
	return NewSigner([]byte("test-secret-32-bytes-padding!!!"))
}

func TestSigner_IssueAndVerify(t *testing.T) {
	s := testSigner()
	token, expiresAt, err := s.Issue(Subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if time.Until(expiresAt) > AccessTokenTTL || time.Until(expiresAt) < AccessTokenTTL-time.Second {
		t.Errorf("expiresAt not ~1h out: %v", expiresAt)
	}

	claims, err := s.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != Subject {
		t.Errorf("Subject = %q, want %q", claims.Subject, Subject)
	}
	if claims.Scope != ScopeProfileRead {
		t.Errorf("Scope = %q, want %q", claims.Scope, ScopeProfileRead)
	}
}

func TestSigner_Verify_WrongSecret(t *testing.T) {
	s1 := NewSigner([]byte("secret-one-32-bytes-of-padding!"))
	s2 := NewSigner([]byte("secret-two-32-bytes-of-padding!"))
	token, _, err := s1.Issue(Subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := s2.Verify(token); err == nil {
		t.Error("expected verification with a different secret to fail")
	}
}

func TestSigner_Verify_Expired(t *testing.T) {
	s := testSigner()
	claims := Claims{
		Scope: ScopeProfileRead,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := s.Verify(signed); err == nil {
		t.Error("expected an expired token to fail verification")
	}
}

func TestSigner_Verify_WrongScope(t *testing.T) {
	s := testSigner()
	claims := Claims{
		Scope: "profile:write",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := s.Verify(signed); err == nil {
		t.Error("expected a non-profile:read scope to fail verification")
	}
}

func TestSigner_Verify_Garbage(t *testing.T) {
	s := testSigner()
	if _, err := s.Verify("not-a-jwt"); err == nil {
		t.Error("expected garbage input to fail verification")
	}
}

func TestRandomToken_UniqueAndHashed(t *testing.T) {
	p1, h1, err := RandomToken(32)
	if err != nil {
		t.Fatalf("RandomToken: %v", err)
	}
	p2, h2, err := RandomToken(32)
	if err != nil {
		t.Fatalf("RandomToken: %v", err)
	}
	if p1 == p2 || h1 == h2 {
		t.Error("expected distinct random tokens")
	}
	if h1 != HashToken(p1) {
		t.Error("hash mismatch for deterministic HashToken")
	}
	if p1 == h1 {
		t.Error("plaintext and hash must differ (hash must not be the plaintext)")
	}
}
