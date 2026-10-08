package middleware

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"latihan-fiber/UTS/helper"
	"latihan-fiber/UTS/model"
)

type acceptIdentity struct{}

func (acceptIdentity) Authenticate(_ context.Context, identity model.AuthIdentity) (model.AuthIdentity, error) {
	return identity, nil
}

func TestRequireAuthAndRole(t *testing.T) {
	manager, err := helper.NewJWTManager("01234567890123456789012345678901", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	auth := RequireAuth(manager, acceptIdentity{})
	app.Get("/admin", auth, RequireRole("admin"), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/student", auth, RequireRole("mahasiswa"), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	request := func(path, token string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set(fiber.HeaderAuthorization, token)
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	if got := request("/admin", ""); got != fiber.StatusUnauthorized {
		t.Fatalf("missing token status = %d", got)
	}
	if got := request("/admin", "Bearer invalid"); got != fiber.StatusUnauthorized {
		t.Fatalf("invalid token status = %d", got)
	}

	admin, err := manager.Generate(model.AuthIdentity{UserID: 1, Email: "admin@example.com", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := manager.Generate(model.AuthIdentity{UserID: 2, Email: "student@example.com", Role: "mahasiswa"})
	if err != nil {
		t.Fatal(err)
	}
	if got := request("/admin", "Bearer "+admin); got != fiber.StatusNoContent {
		t.Fatalf("admin role status = %d", got)
	}
	if got := request("/admin", "Bearer "+student); got != fiber.StatusForbidden {
		t.Fatalf("student on admin route status = %d", got)
	}
	if got := request("/student", "Bearer "+student); got != fiber.StatusNoContent {
		t.Fatalf("student role status = %d", got)
	}
	if got := request("/student", "Bearer "+admin); got != fiber.StatusForbidden {
		t.Fatalf("admin on student route status = %d", got)
	}
}

func TestRequireAuthRejectsMalformedBearerHeader(t *testing.T) {
	manager, _ := helper.NewJWTManager("01234567890123456789012345678901", time.Minute)
	app := fiber.New()
	app.Get("/private", RequireAuth(manager, acceptIdentity{}), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	for _, header := range []string{"token", "Bearer", "Basic abc", "Bearer token extra"} {
		req, _ := http.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set(fiber.HeaderAuthorization, header)
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != fiber.StatusUnauthorized || !strings.HasPrefix(response.Header.Get(fiber.HeaderWWWAuthenticate), "Bearer") {
			t.Fatalf("header %q: status=%d challenge=%q", header, response.StatusCode, response.Header.Get(fiber.HeaderWWWAuthenticate))
		}
	}
}

func TestRequireAuthRejectsExpiredToken(t *testing.T) {
	secret := "01234567890123456789012345678901"
	manager, _ := helper.NewJWTManager(secret, time.Minute)
	claims := helper.AccessClaims{
		UserID: 1,
		Email:  "admin@example.com",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "siakad-mini",
			Subject:   "1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/private", RequireAuth(manager, acceptIdentity{}), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	req, _ := http.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+expired)
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expired token status = %d, want 401", response.StatusCode)
	}
}
