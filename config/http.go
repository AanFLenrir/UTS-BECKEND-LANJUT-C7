package config

import (
	"github.com/gofiber/fiber/v2"
)

// ErrorHandler keeps unexpected framework errors and panics in the API error contract.
func ErrorHandler(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	if fiberErr, ok := err.(*fiber.Error); ok {
		status = fiberErr.Code
	}
	message := "terjadi kesalahan pada server"
	if status == fiber.StatusNotFound {
		message = "resource tidak ditemukan"
	} else if status >= fiber.StatusBadRequest && status < fiber.StatusInternalServerError {
		message = "request tidak dapat diproses"
	}
	return c.Status(status).JSON(fiber.Map{"success": false, "message": message})
}
