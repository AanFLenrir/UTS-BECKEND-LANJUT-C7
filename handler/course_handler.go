package handler

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"latihan-fiber/UTS/middleware"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/service"
)

type CourseUseCase interface {
	List(ctx context.Context, actor model.AuthIdentity, query model.CourseListQuery) (service.CoursePage, error)
	Get(ctx context.Context, actor model.AuthIdentity, id int64) (model.Course, error)
	Create(ctx context.Context, actor model.AuthIdentity, input model.CourseInput) (model.Course, error)
	Update(ctx context.Context, actor model.AuthIdentity, id int64, input model.CourseInput) (model.Course, error)
	Delete(ctx context.Context, actor model.AuthIdentity, id int64) error
}

type CourseHandler struct{ service CourseUseCase }

func NewCourseHandler(courseService CourseUseCase) *CourseHandler {
	return &CourseHandler{service: courseService}
}

func (h *CourseHandler) List(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return courseError(c, service.ErrForbidden)
	}
	page, err := queryInt(c, "page", 1)
	if err != nil {
		return courseValidationResponse(c, map[string]string{"page": "page harus berupa bilangan bulat positif"})
	}
	limit := 10
	limitParam, perPageParam := c.Query("limit"), c.Query("per_page")
	if limitParam != "" && perPageParam != "" && limitParam != perPageParam {
		return courseValidationResponse(c, map[string]string{"limit": "limit dan per_page tidak boleh berbeda"})
	}
	if perPageParam != "" {
		limit, err = strconv.Atoi(perPageParam)
	} else if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
	}
	if err != nil || page <= 0 || limit <= 0 || limit > 50 {
		return courseValidationResponse(c, map[string]string{"page": "page harus positif dan limit antara 1 sampai 50"})
	}
	var semester *int
	if raw := c.Query("semester"); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value <= 0 {
			return courseValidationResponse(c, map[string]string{"semester": "semester harus berupa bilangan positif"})
		}
		semester = &value
	}
	availableOnly := false
	if raw := c.Query("available"); raw != "" {
		availableOnly, err = strconv.ParseBool(raw)
		if err != nil {
			return courseValidationResponse(c, map[string]string{"available": "available harus true atau false"})
		}
	}
	sortField, order := c.Query("sort"), c.Query("order")
	if strings.HasPrefix(sortField, "-") {
		sortField = strings.TrimPrefix(sortField, "-")
		if order == "" {
			order = "desc"
		}
	}
	result, err := h.service.List(c.UserContext(), identity, model.CourseListQuery{
		Page: page, Limit: limit, Semester: semester, Search: c.Query("search"),
		Sort: sortField, SortOrder: order, AvailableOnly: availableOnly,
	})
	if err != nil {
		return courseError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true, "message": "Daftar mata kuliah", "data": result.Courses,
		"meta": fiber.Map{"page": result.Page, "limit": result.Limit, "total": result.Total, "total_pages": result.TotalPages},
	})
}

func (h *CourseHandler) Get(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return courseError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return courseValidationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	course, err := h.service.Get(c.UserContext(), identity, id)
	if err != nil {
		return courseError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "message": "Data mata kuliah", "data": course})
}

func (h *CourseHandler) Create(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return courseError(c, service.ErrForbidden)
	}
	var input model.CourseInput
	if err := decodeStrictJSON(c.Body(), &input); err != nil {
		return courseValidationResponse(c, map[string]string{"body": "request JSON tidak valid"})
	}
	course, err := h.service.Create(c.UserContext(), identity, input)
	if err != nil {
		return courseError(c, err)
	}
	c.Set(fiber.HeaderLocation, "/api/v1/courses/"+strconv.FormatInt(course.ID, 10))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"success": true, "message": "Mata kuliah berhasil dibuat", "data": course})
}

func (h *CourseHandler) Update(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return courseError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return courseValidationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	var input model.CourseInput
	if err := decodeStrictJSON(c.Body(), &input); err != nil {
		return courseValidationResponse(c, map[string]string{"body": "request JSON tidak valid"})
	}
	course, err := h.service.Update(c.UserContext(), identity, id, input)
	if err != nil {
		return courseError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "message": "Mata kuliah diperbarui", "data": course})
}

func (h *CourseHandler) Delete(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return courseError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return courseValidationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	if err := h.service.Delete(c.UserContext(), identity, id); err != nil {
		return courseError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func courseError(c *fiber.Ctx, err error) error {
	var validationErr *service.CourseValidationError
	if errors.As(err, &validationErr) {
		return courseValidationResponse(c, validationErr.Fields)
	}
	switch {
	case errors.Is(err, service.ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "akses ditolak"})
	case errors.Is(err, service.ErrCourseNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "mata kuliah tidak ditemukan"})
	case errors.Is(err, service.ErrCourseConflict), errors.Is(err, service.ErrQuotaConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "message": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": "terjadi kesalahan pada server"})
	}
}

func courseValidationResponse(c *fiber.Ctx, fields map[string]string) error {
	errors := make(map[string][]string, len(fields))
	for field, message := range fields {
		errors[field] = []string{message}
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"success": false, "message": "Validasi gagal", "errors": errors})
}
