package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
)

const RequestIDLocal = "request_id"

// RequestTracing assigns a fresh request ID and logs only non-sensitive request metadata.
func RequestTracing(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError)
		}
		id := hex.EncodeToString(raw[:])
		c.Locals(RequestIDLocal, id)
		c.Set("X-Request-ID", id)
		started := time.Now()
		err := c.Next()
		if logger != nil {
			status := c.Response().StatusCode()
			if err != nil {
				var fiberErr *fiber.Error
				if errors.As(err, &fiberErr) {
					status = fiberErr.Code
				} else if status < fiber.StatusBadRequest {
					status = fiber.StatusInternalServerError
				}
			}
			logger.Info("http request", "request_id", id, "method", c.Method(), "path", c.Path(), "status", status, "duration_ms", time.Since(started).Milliseconds(), "client_ip", c.IP())
		}
		return err
	}
}
