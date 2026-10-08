package handler

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"

	"latihan-fiber/UTS/middleware"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/service"
)

type AuthUseCase interface {
	Login(context.Context, string, string) (model.LoginResponse, error)
	Me(context.Context, model.AuthIdentity) (model.MeResponse, error)
}

type AuthHandler struct {
	service AuthUseCase
}

func NewAuthHandler(authService AuthUseCase) *AuthHandler {
	return &AuthHandler{service: authService}
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var request model.LoginRequest
	if err := decodeStrictJSON(c.Body(), &request); err != nil {
		return authValidation(c, map[string]string{"body": "request JSON tidak valid"})
	}
	request.Email = strings.TrimSpace(request.Email)
	parsed, err := mail.ParseAddress(request.Email)
	fields := make(map[string]string)
	if err != nil || parsed.Address != request.Email {
		fields["email"] = "email wajib diisi dengan format valid"
	}
	if request.Password == "" || len(request.Password) < 8 || len(request.Password) > 72 {
		fields["password"] = "password wajib diisi dan minimal 8 karakter"
	}
	if len(fields) > 0 {
		return authValidation(c, fields)
	}

	result, err := h.service.Login(c.UserContext(), request.Email, request.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return fail(c, fiber.StatusUnauthorized, "email atau password salah")
		}
		return fail(c, fiber.StatusInternalServerError, "terjadi kesalahan pada server")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "Login successful",
		"data":    result,
	})
}

func authValidation(c *fiber.Ctx, fields map[string]string) error {
	errors := make(map[string][]string, len(fields))
	for field, message := range fields {
		errors[field] = []string{message}
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"success": false, "message": "Validasi gagal", "errors": errors})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return fail(c, fiber.StatusUnauthorized, "autentikasi diperlukan")
	}
	result, err := h.service.Me(c.UserContext(), identity)
	if err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			return fail(c, fiber.StatusUnauthorized, "token atau akun tidak valid")
		}
		return fail(c, fiber.StatusInternalServerError, "terjadi kesalahan pada server")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "Current user",
		"data":    result,
	})
}

func fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{
		"success": false,
		"message": message,
	})
}
