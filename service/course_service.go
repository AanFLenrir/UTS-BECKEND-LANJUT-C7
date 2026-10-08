package service

import (
	"context"
	"errors"
	"strings"

	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

var (
	ErrCourseNotFound = errors.New("course tidak ditemukan")
	ErrCourseConflict = errors.New("course berkonflik dengan data terkait")
	ErrQuotaConflict  = errors.New("kuota tidak boleh lebih kecil dari jumlah enrollment")
)

type CourseValidationError struct{ Fields map[string]string }

func (e *CourseValidationError) Error() string        { return ErrValidation.Error() }
func (e *CourseValidationError) Is(target error) bool { return target == ErrValidation }

type CoursePage struct {
	Courses    []model.Course `json:"courses"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	Total      int64          `json:"total"`
	TotalPages int64          `json:"total_pages"`
}

type CourseService struct{ repo repository.CourseRepository }

func NewCourseService(repo repository.CourseRepository) *CourseService {
	return &CourseService{repo: repo}
}

func (s *CourseService) List(ctx context.Context, actor model.AuthIdentity, q model.CourseListQuery) (CoursePage, error) {
	if actor.Role != "admin" && actor.Role != "mahasiswa" {
		return CoursePage{}, ErrForbidden
	}
	if q.Page <= 0 || q.Limit <= 0 || q.Limit > 50 || q.Page-1 > int(^uint(0)>>1)/q.Limit {
		return CoursePage{}, courseValidation(map[string]string{"page": "page harus positif dan limit antara 1 sampai 50"})
	}
	if q.Semester != nil && (*q.Semester < 1 || *q.Semester > 14) {
		return CoursePage{}, courseValidation(map[string]string{"semester": "semester harus antara 1 dan 14"})
	}
	allowedSort := map[string]bool{"kode_mk": true, "nama_mk": true, "sks": true, "semester": true, "kuota": true}
	if q.Sort == "" {
		q.Sort = "kode_mk"
	} else if !allowedSort[q.Sort] {
		return CoursePage{}, courseValidation(map[string]string{"sort": "sort tidak didukung"})
	}
	if q.SortOrder == "" {
		q.SortOrder = "asc"
	} else {
		q.SortOrder = strings.ToLower(q.SortOrder)
		if q.SortOrder != "asc" && q.SortOrder != "desc" {
			return CoursePage{}, courseValidation(map[string]string{"order": "order harus asc atau desc"})
		}
	}
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 100 {
		return CoursePage{}, courseValidation(map[string]string{"search": "search maksimal 100 karakter"})
	}
	q.Offset = (q.Page - 1) * q.Limit
	total, err := s.repo.CountCourses(ctx, q)
	if err != nil {
		return CoursePage{}, err
	}
	courses, err := s.repo.ListCourses(ctx, q)
	if err != nil {
		return CoursePage{}, err
	}
	if courses == nil {
		courses = []model.Course{}
	}
	for i := range courses {
		courses[i].SisaKuota = remainingQuota(courses[i].Kuota, courses[i].Terisi)
	}
	pages := int64(0)
	if total > 0 {
		pages = (total + int64(q.Limit) - 1) / int64(q.Limit)
	}
	return CoursePage{Courses: courses, Page: q.Page, Limit: q.Limit, Total: total, TotalPages: pages}, nil
}

func (s *CourseService) Get(ctx context.Context, actor model.AuthIdentity, id int64) (model.Course, error) {
	if actor.Role != "admin" && actor.Role != "mahasiswa" {
		return model.Course{}, ErrForbidden
	}
	if id <= 0 {
		return model.Course{}, courseValidation(map[string]string{"id": "id harus berupa bilangan positif"})
	}
	course, err := s.repo.FindCourseByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.Course{}, ErrCourseNotFound
	}
	if err == nil {
		course.SisaKuota = remainingQuota(course.Kuota, course.Terisi)
	}
	return course, err
}

func (s *CourseService) Create(ctx context.Context, actor model.AuthIdentity, input model.CourseInput) (model.Course, error) {
	if actor.Role != "admin" {
		return model.Course{}, ErrForbidden
	}
	input = normalizeCourse(input)
	if fields := validateCourse(input); len(fields) > 0 {
		return model.Course{}, courseValidation(fields)
	}
	course, err := s.repo.CreateCourse(ctx, input)
	if errors.Is(err, repository.ErrDuplicateCourse) {
		return model.Course{}, ErrCourseConflict
	}
	if err == nil {
		course.SisaKuota = remainingQuota(course.Kuota, course.Terisi)
	}
	return course, err
}

func (s *CourseService) Update(ctx context.Context, actor model.AuthIdentity, id int64, input model.CourseInput) (model.Course, error) {
	if actor.Role != "admin" {
		return model.Course{}, ErrForbidden
	}
	if id <= 0 {
		return model.Course{}, courseValidation(map[string]string{"id": "id harus berupa bilangan positif"})
	}
	input = normalizeCourse(input)
	if fields := validateCourse(input); len(fields) > 0 {
		return model.Course{}, courseValidation(fields)
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return model.Course{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.FindCourseForUpdate(ctx, id); errors.Is(err, repository.ErrNotFound) {
		return model.Course{}, ErrCourseNotFound
	} else if err != nil {
		return model.Course{}, err
	}
	filled, err := tx.CountEnrollmentsByCourse(ctx, id)
	if err != nil {
		return model.Course{}, err
	}
	if int64(input.Kuota) < filled {
		return model.Course{}, ErrQuotaConflict
	}
	updated, err := tx.UpdateCourse(ctx, id, input)
	if errors.Is(err, repository.ErrDuplicateCourse) {
		return model.Course{}, ErrCourseConflict
	}
	if errors.Is(err, repository.ErrNotFound) {
		return model.Course{}, ErrCourseNotFound
	}
	if err != nil {
		return model.Course{}, err
	}
	updated.Terisi = filled
	updated.SisaKuota = remainingQuota(updated.Kuota, filled)
	if err := tx.Commit(ctx); err != nil {
		return model.Course{}, err
	}
	return updated, nil
}

func (s *CourseService) Delete(ctx context.Context, actor model.AuthIdentity, id int64) error {
	if actor.Role != "admin" {
		return ErrForbidden
	}
	if id <= 0 {
		return courseValidation(map[string]string{"id": "id harus berupa bilangan positif"})
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.FindCourseForUpdate(ctx, id); errors.Is(err, repository.ErrNotFound) {
		return ErrCourseNotFound
	} else if err != nil {
		return err
	}
	filled, err := tx.CountEnrollmentsByCourse(ctx, id)
	if err != nil {
		return err
	}
	if filled > 0 {
		return ErrCourseConflict
	}
	if err := tx.DeleteCourse(ctx, id); errors.Is(err, repository.ErrCourseHasEntries) {
		return ErrCourseConflict
	} else if errors.Is(err, repository.ErrNotFound) {
		return ErrCourseNotFound
	} else if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func normalizeCourse(input model.CourseInput) model.CourseInput {
	input.KodeMK = strings.TrimSpace(input.KodeMK)
	input.NamaMK = strings.TrimSpace(input.NamaMK)
	return input
}

func validateCourse(input model.CourseInput) map[string]string {
	fields := make(map[string]string)
	if input.KodeMK == "" || len(input.KodeMK) > 30 {
		fields["kode_mk"] = "kode_mk wajib diisi dan maksimal 30 karakter"
	}
	if input.NamaMK == "" || len(input.NamaMK) > 180 {
		fields["nama_mk"] = "nama_mk wajib diisi dan maksimal 180 karakter"
	}
	if input.SKS < 1 || input.SKS > 24 {
		fields["sks"] = "sks harus antara 1 dan 24"
	}
	if input.Semester < 1 || input.Semester > 14 {
		fields["semester"] = "semester harus antara 1 dan 14"
	}
	if input.Kuota < 0 {
		fields["kuota"] = "kuota tidak boleh negatif"
	}
	return fields
}

func courseValidation(fields map[string]string) error { return &CourseValidationError{Fields: fields} }

func remainingQuota(quota int, filled int64) int64 {
	remaining := int64(quota) - filled
	if remaining < 0 {
		return 0
	}
	return remaining
}
