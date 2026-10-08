package helper

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"latihan-fiber/UTS/model"
)

var ErrInvalidToken = errors.New("token tidak valid")

type AccessClaims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttl time.Duration) (*JWTManager, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET harus memiliki minimal 32 byte")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("JWT access TTL harus lebih besar dari nol")
	}
	return &JWTManager{secret: []byte(secret), ttl: ttl}, nil
}

func (m *JWTManager) TTL() time.Duration { return m.ttl }

func (m *JWTManager) Generate(user model.AuthIdentity) (string, error) {
	now := time.Now()
	claims := AccessClaims{
		UserID: user.UserID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "siakad-mini",
			Subject:   fmt.Sprint(user.UserID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *JWTManager) Parse(tokenString string) (model.AuthIdentity, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("siakad-mini"), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid || claims.UserID <= 0 || claims.Email == "" || !ValidRole(claims.Role) {
		return model.AuthIdentity{}, ErrInvalidToken
	}
	return model.AuthIdentity{UserID: claims.UserID, Email: claims.Email, Role: claims.Role}, nil
}

func ValidRole(role string) bool { return role == "admin" || role == "mahasiswa" }
