package handler

import (
	"context"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"latihan-fiber/UTS/middleware"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/service"
)

type EnrollmentUseCase interface {
	Create(context.Context, model.AuthIdentity, model.EnrollmentRequest) (model.EnrollmentView, error)
	ListByStudent(context.Context, model.AuthIdentity, int64, string) (model.StudentEnrollmentSummary, error)
	Delete(context.Context, model.AuthIdentity, int64) error
}

type EnrollmentHandler struct{ service EnrollmentUseCase }

func NewEnrollmentHandler(enrollmentService EnrollmentUseCase) *EnrollmentHandler {
	return &EnrollmentHandler{service: enrollmentService}
}

func (h *EnrollmentHandler) Create(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return enrollmentError(c, service.ErrForbidden)
	}
	var request model.EnrollmentRequest
	if err := decodeStrictJSON(c.Body(), &request); err != nil {
		return enrollmentValidation(c, map[string]string{"body": "request JSON tidak valid atau berisi field yang tidak dikenal"})
	}
	created, err := h.service.Create(c.UserContext(), identity, request)
	if err != nil {
		return enrollmentError(c, err)
	}
	c.Set(fiber.HeaderLocation, "/api/v1/enrollments/"+strconv.FormatInt(created.ID, 10))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "Enrollment berhasil dibuat", "data": created})
}

func (h *EnrollmentHandler) ListByStudent(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return enrollmentError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return courseValidationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	result, err := h.service.ListByStudent(c.UserContext(), identity, id, c.Query("tahun_akademik"))
	if err != nil {
		return enrollmentError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "message": "Daftar enrollment mahasiswa", "data": result})
}

func (h *EnrollmentHandler) Delete(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return enrollmentError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return courseValidationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	if err := h.service.Delete(c.UserContext(), identity, id); err != nil {
		return enrollmentError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func enrollmentError(c *fiber.Ctx, err error) error {
	var validationErr *service.EnrollmentValidationError
	if errors.As(err, &validationErr) {
		return enrollmentValidation(c, validationErr.Fields)
	}
	switch {
	case errors.Is(err, service.ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "akses ditolak"})
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrCourseNotFound), errors.Is(err, service.ErrEnrollmentStudentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "student, course, atau enrollment tidak ditemukan"})
	case errors.Is(err, service.ErrEnrollmentDuplicate), errors.Is(err, service.ErrCourseFull), errors.Is(err, service.ErrCreditLimit):
		if errors.Is(err, service.ErrEnrollmentDuplicate) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "message": "course sudah diambil pada tahun akademik ini"})
		}
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"success": false, "message": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": "terjadi kesalahan pada server"})
	}
}

func enrollmentValidation(c *fiber.Ctx, fields map[string]string) error {
	errors := make(map[string][]string, len(fields))
	for field, message := range fields {
		errors[field] = []string{message}
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"success": false, "message": "Validasi gagal", "errors": errors})
}
