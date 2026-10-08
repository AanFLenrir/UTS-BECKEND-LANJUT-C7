package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"latihan-fiber/UTS/helper"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/service"
)

const identityLocal = "auth_identity"

type IdentityAuthenticator interface {
	Authenticate(context.Context, model.AuthIdentity) (model.AuthIdentity, error)
}

func RequireAuth(jwtManager *helper.JWTManager, authenticator IdentityAuthenticator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		parts := strings.Fields(c.Get(fiber.HeaderAuthorization))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return authFailure(c)
		}
		claims, err := jwtManager.Parse(parts[1])
		if err != nil {
			return authFailure(c)
		}
		identity, err := authenticator.Authenticate(c.UserContext(), claims)
		if err != nil {
			if errors.Is(err, service.ErrUnauthorized) {
				return authFailure(c)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": "terjadi kesalahan pada server"})
		}
		c.Locals(identityLocal, identity)
		return c.Next()
	}
}

func RequireRole(roles ...string) fiber.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if helper.ValidRole(role) {
			allowed[role] = struct{}{}
		}
	}
	return func(c *fiber.Ctx) error {
		identity, ok := IdentityFromContext(c)
		if !ok {
			return authFailure(c)
		}
		if _, ok := allowed[identity.Role]; !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "akses ditolak"})
		}
		return c.Next()
	}
}

func IdentityFromContext(c *fiber.Ctx) (model.AuthIdentity, bool) {
	identity, ok := c.Locals(identityLocal).(model.AuthIdentity)
	return identity, ok
}

func authFailure(c *fiber.Ctx) error {
	c.Set(fiber.HeaderWWWAuthenticate, `Bearer realm="api"`)
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "message": "autentikasi diperlukan"})
}
