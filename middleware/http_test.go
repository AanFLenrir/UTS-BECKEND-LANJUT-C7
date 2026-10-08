package middleware

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"log/slog"

	"github.com/gofiber/fiber/v2"
)

func TestRequestTracingCreatesContextIDAndOmitsCredentialsFromLogs(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	app := fiber.New()
	app.Use(RequestTracing(logger))
	app.Post("/private", func(c *fiber.Ctx) error {
		id, ok := c.Locals(RequestIDLocal).(string)
		if !ok || len(id) != 32 {
			t.Errorf("request ID local = %q, want 32-character ID", id)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	req, _ := http.NewRequest(http.MethodPost, "/private", strings.NewReader(`{"password":"private-password"}`))
	req.Header.Set(fiber.HeaderAuthorization, "Bearer private-token")
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.StatusCode)
	}
	id := response.Header.Get("X-Request-ID")
	if len(id) != 32 {
		t.Fatalf("X-Request-ID = %q, want 32-character ID", id)
	}
	if strings.Contains(output.String(), "private-password") || strings.Contains(output.String(), "private-token") {
		t.Fatal("request log contains sensitive request data")
	}
}
