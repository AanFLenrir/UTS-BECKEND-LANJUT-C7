package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"latihan-fiber/UTS/middleware"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/service"
)

type StudentUseCase interface {
	List(context.Context, model.AuthIdentity, model.StudentListQuery) (service.StudentPage, error)
	Create(context.Context, model.AuthIdentity, model.CreateStudentRequest) (model.Student, error)
	Get(context.Context, model.AuthIdentity, int64) (model.StudentDetail, error)
	Update(context.Context, model.AuthIdentity, int64, model.UpdateStudentRequest) (model.Student, error)
	Delete(context.Context, model.AuthIdentity, int64) error
}

type StudentHandler struct {
	service StudentUseCase
}

func NewStudentHandler(studentService StudentUseCase) *StudentHandler {
	return &StudentHandler{service: studentService}
}

func (h *StudentHandler) List(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return studentError(c, service.ErrForbidden)
	}
	page, err := queryInt(c, "page", 1)
	if err != nil {
		return validationResponse(c, map[string]string{"page": "page harus berupa bilangan bulat positif"})
	}
	limitParam := c.Query("limit")
	perPageParam := c.Query("per_page")
	if limitParam != "" && perPageParam != "" && limitParam != perPageParam {
		return validationResponse(c, map[string]string{"limit": "limit dan per_page tidak boleh berbeda"})
	}
	limit := 10
	if perPageParam != "" {
		limit, err = strconv.Atoi(perPageParam)
	} else if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
	}
	if err != nil || page <= 0 || limit <= 0 || limit > 50 {
		return validationResponse(c, map[string]string{"page": "page harus positif dan limit antara 1 sampai 50"})
	}
	var angkatan *int
	if raw := c.Query("angkatan"); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value <= 0 {
			return validationResponse(c, map[string]string{"angkatan": "angkatan harus berupa bilangan positif"})
		}
		angkatan = &value
	}
	sortField := c.Query("sort")
	order := c.Query("order")
	if (sortField != "" && sortField != "nama" && sortField != "-ipk_terakhir") || order != "" {
		return validationResponse(c, map[string]string{"sort": "sort yang didukung hanya nama dan -ipk_terakhir"})
	}
	if strings.HasPrefix(sortField, "-") {
		sortField = strings.TrimPrefix(sortField, "-")
		if order == "" {
			order = "desc"
		}
	}
	result, err := h.service.List(c.UserContext(), identity, model.StudentListQuery{
		Page: page, Limit: limit, Prodi: c.Query("prodi"), Angkatan: angkatan,
		Search: c.Query("search"), Sort: sortField, SortOrder: order,
	})
	if err != nil {
		return studentError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true, "message": "Daftar mahasiswa", "data": result.Students,
		"meta": fiber.Map{"current_page": result.Page, "per_page": result.Limit, "total": result.Total, "last_page": result.TotalPages},
	})
}

func (h *StudentHandler) Create(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return studentError(c, service.ErrForbidden)
	}
	var req model.CreateStudentRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return validationResponse(c, map[string]string{"body": "request JSON tidak valid"})
	}
	created, err := h.service.Create(c.UserContext(), identity, req)
	if err != nil {
		return studentError(c, err)
	}
	location := "/api/v1/students/" + strconv.FormatInt(created.ID, 10)
	c.Set(fiber.HeaderLocation, location)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true, "message": "Mahasiswa berhasil dibuat",
		"data": model.StudentListItem{
			ID: created.ID, UserID: created.UserID, Email: strings.TrimSpace(req.Email), NIM: created.NIM,
			Nama: created.Nama, Prodi: created.Prodi, Angkatan: created.Angkatan, IPKTerakhir: created.IPKTerakhir,
		},
	})
}

func (h *StudentHandler) Get(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return studentError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return validationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	result, err := h.service.Get(c.UserContext(), identity, id)
	if err != nil {
		return studentError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "message": "Data mahasiswa", "data": result})
}

func (h *StudentHandler) Update(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return studentError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return validationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &raw); err != nil {
		return validationResponse(c, map[string]string{"body": "request JSON tidak valid"})
	}
	var req model.UpdateStudentRequest
	if err := decodeStrictJSON(c.Body(), &req); err != nil {
		return validationResponse(c, map[string]string{"body": "request JSON tidak valid"})
	}
	for key := range raw {
		if strings.EqualFold(key, "nim") && req.NIM == nil {
			empty := ""
			req.NIM = &empty
			break
		}
	}
	updated, err := h.service.Update(c.UserContext(), identity, id, req)
	if err != nil {
		return studentError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "message": "Data mahasiswa diperbarui", "data": fiber.Map{
		"id": updated.ID, "user_id": updated.UserID, "nim": updated.NIM,
		"nama": updated.Nama, "prodi": updated.Prodi, "angkatan": updated.Angkatan, "ipk_terakhir": updated.IPKTerakhir,
	}})
}

func (h *StudentHandler) Delete(c *fiber.Ctx) error {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		return studentError(c, service.ErrForbidden)
	}
	id, err := pathID(c)
	if err != nil {
		return validationResponse(c, map[string]string{"id": "id harus berupa bilangan bulat positif"})
	}
	if err := h.service.Delete(c.UserContext(), identity, id); err != nil {
		return studentError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func queryInt(c *fiber.Ctx, key string, fallback int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func pathID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func studentError(c *fiber.Ctx, err error) error {
	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		return validationResponse(c, validationErr.Fields)
	}
	switch {
	case errors.Is(err, service.ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "akses ditolak"})
	case errors.Is(err, service.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "mahasiswa tidak ditemukan"})
	case errors.Is(err, service.ErrConflict):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"success": false, "message": "Validasi gagal", "errors": map[string][]string{
			"nim": {"NIM atau email sudah digunakan"}, "email": {"NIM atau email sudah digunakan"},
		}})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": "terjadi kesalahan pada server"})
	}
}

func validationResponse(c *fiber.Ctx, fields map[string]string) error {
	errors := make(map[string][]string, len(fields))
	for field, message := range fields {
		errors[field] = []string{message}
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"success": false, "message": "Validasi gagal", "errors": errors})
}
