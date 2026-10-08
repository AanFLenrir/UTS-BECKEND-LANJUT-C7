package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

var (
	ErrEnrollmentStudentNotFound = errors.New("student aktif tidak ditemukan")
	ErrEnrollmentDuplicate       = errors.New("enrollment duplikat")
	ErrCourseFull                = errors.New("kuota course penuh")
	ErrCreditLimit               = errors.New("batas SKS terlampaui")
)

type EnrollmentValidationError struct{ Fields map[string]string }

func (e *EnrollmentValidationError) Error() string        { return ErrValidation.Error() }
func (e *EnrollmentValidationError) Is(target error) bool { return target == ErrValidation }

type EnrollmentService struct {
	repo repository.EnrollmentRepository
}

func NewEnrollmentService(repo repository.EnrollmentRepository) *EnrollmentService {
	return &EnrollmentService{repo: repo}
}

func validAcademicYear(year string) bool {
	return regexp.MustCompile(`^[0-9]{4}/[0-9]{4}-(Ganjil|Genap)$`).MatchString(year)
}

func (s *EnrollmentService) Create(ctx context.Context, actor model.AuthIdentity, req model.EnrollmentRequest) (model.EnrollmentView, error) {
	if actor.Role != "mahasiswa" || actor.UserID <= 0 {
		return model.EnrollmentView{}, ErrForbidden
	}
	req.TahunAkademik = strings.TrimSpace(req.TahunAkademik)
	if req.CourseID <= 0 || !validAcademicYear(req.TahunAkademik) {
		fields := make(map[string]string)
		if req.CourseID <= 0 {
			fields["course_id"] = "course_id harus berupa bilangan positif"
		}
		if !validAcademicYear(req.TahunAkademik) {
			fields["tahun_akademik"] = "format tahun_akademik harus YYYY/YYYY-Ganjil atau YYYY/YYYY-Genap"
		}
		return model.EnrollmentView{}, &EnrollmentValidationError{Fields: fields}
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return model.EnrollmentView{}, err
	}
	defer tx.Rollback(context.Background())

	// Lock course first so quota checks and enrollment insertion serialize
	// with other requests for the same course.
	course, err := tx.GetCourseForUpdate(ctx, req.CourseID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.EnrollmentView{}, ErrCourseNotFound
	}
	if err != nil {
		return model.EnrollmentView{}, err
	}
	student, err := tx.GetStudentByUserIDForUpdate(ctx, actor.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.EnrollmentView{}, ErrEnrollmentStudentNotFound
	}
	if err != nil {
		return model.EnrollmentView{}, err
	}
	exists, err := tx.FindEnrollmentByStudentCourseYear(ctx, student.ID, course.ID, req.TahunAkademik)
	if err != nil {
		return model.EnrollmentView{}, err
	}
	if exists {
		return model.EnrollmentView{}, ErrEnrollmentDuplicate
	}
	filled, err := tx.CountEnrollmentsByCourse(ctx, course.ID)
	if err != nil {
		return model.EnrollmentView{}, err
	}
	if filled >= int64(course.Kuota) {
		return model.EnrollmentView{}, ErrCourseFull
	}
	total, err := tx.GetTotalStudentCredits(ctx, student.ID, req.TahunAkademik)
	if err != nil {
		return model.EnrollmentView{}, err
	}
	limit := SKSLimit(student.IPKTerakhir)
	if total+course.SKS > limit {
		return model.EnrollmentView{}, fmt.Errorf("%w: sisa SKS %d", ErrCreditLimit, limit-total)
	}
	created, err := tx.CreateEnrollment(ctx, model.Enrollment{
		StudentID: student.ID, CourseID: course.ID, TahunAkademik: req.TahunAkademik,
	})
	if errors.Is(err, repository.ErrDuplicateEnrollment) {
		return model.EnrollmentView{}, ErrEnrollmentDuplicate
	}
	if err != nil {
		return model.EnrollmentView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.EnrollmentView{}, err
	}
	return model.EnrollmentView{
		ID:            created.ID,
		Course:        model.CourseSummary{ID: course.ID, KodeMK: course.KodeMK, NamaMK: course.NamaMK, SKS: course.SKS, Semester: course.Semester},
		TahunAkademik: created.TahunAkademik,
		CreatedAt:     created.CreatedAt,
	}, nil
}

func (s *EnrollmentService) ListByStudent(ctx context.Context, actor model.AuthIdentity, studentID int64, year string) (model.StudentEnrollmentSummary, error) {
	if actor.Role != "admin" && actor.Role != "mahasiswa" {
		return model.StudentEnrollmentSummary{}, ErrForbidden
	}
	if studentID <= 0 {
		return model.StudentEnrollmentSummary{}, &EnrollmentValidationError{Fields: map[string]string{"id": "id harus berupa bilangan positif"}}
	}
	year = strings.TrimSpace(year)
	if year != "" && !validAcademicYear(year) {
		return model.StudentEnrollmentSummary{}, &EnrollmentValidationError{Fields: map[string]string{"tahun_akademik": "format tahun_akademik harus YYYY/YYYY-Ganjil atau YYYY/YYYY-Genap"}}
	}
	student, err := s.repo.GetStudentByID(ctx, studentID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.StudentEnrollmentSummary{}, ErrEnrollmentStudentNotFound
	}
	if err != nil {
		return model.StudentEnrollmentSummary{}, err
	}
	if actor.Role == "mahasiswa" && student.UserID != actor.UserID {
		return model.StudentEnrollmentSummary{}, ErrForbidden
	}
	enrollments, err := s.repo.ListEnrollmentsByStudent(ctx, student.ID, year)
	if err != nil {
		return model.StudentEnrollmentSummary{}, err
	}
	if enrollments == nil {
		enrollments = []model.EnrollmentView{}
	}
	relevantYear := year
	if relevantYear == "" {
		for _, enrollment := range enrollments {
			if enrollment.TahunAkademik > relevantYear {
				relevantYear = enrollment.TahunAkademik
			}
		}
	}
	total := 0
	if relevantYear != "" {
		for _, enrollment := range enrollments {
			if enrollment.TahunAkademik == relevantYear {
				total += enrollment.Course.SKS
			}
		}
	}
	result := model.StudentEnrollmentSummary{Enrollments: enrollments, TahunAkademik: relevantYear, TotalSKS: total, BatasSKS: SKSLimit(student.IPKTerakhir)}
	result.Student.ID, result.Student.NIM, result.Student.Nama = student.ID, student.NIM, student.Nama
	return result, nil
}

func (s *EnrollmentService) Delete(ctx context.Context, actor model.AuthIdentity, enrollmentID int64) error {
	if actor.Role != "mahasiswa" || actor.UserID <= 0 {
		return ErrForbidden
	}
	if enrollmentID <= 0 {
		return &EnrollmentValidationError{Fields: map[string]string{"id": "id harus berupa bilangan positif"}}
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	record, err := tx.FindEnrollmentByID(ctx, enrollmentID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if actor.Role == "mahasiswa" && record.UserID != actor.UserID {
		return ErrForbidden
	}
	if err := tx.DeleteEnrollment(ctx, enrollmentID); errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
