package helper

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"latihan-fiber/UTS/model"
)

func TestJWTParseValidAndRejectsTampering(t *testing.T) {
	manager, err := NewJWTManager("01234567890123456789012345678901", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	want := model.AuthIdentity{UserID: 42, Email: "person@example.com", Role: "admin"}
	token, err := manager.Generate(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := manager.Parse(token)
	if err != nil || got != want {
		t.Fatalf("Parse() = %#v, %v; want %#v", got, err, want)
	}

	other, _ := NewJWTManager("abcdefghijklmnopqrstuvwxyz123456", time.Minute)
	if _, err := other.Parse(token); err == nil {
		t.Fatal("Parse accepted a token signed with another secret")
	}
}

func TestJWTParseRejectsExpiredToken(t *testing.T) {
	manager, err := NewJWTManager("01234567890123456789012345678901", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims := AccessClaims{
		UserID: 1,
		Email:  "person@example.com",
		Role:   "mahasiswa",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "siakad-mini",
			Subject:   "1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(manager.secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Parse(token); err == nil {
		t.Fatal("Parse accepted an expired token")
	}
}
