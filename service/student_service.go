package service

import (
	"context"
	"errors"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

var (
	ErrForbidden  = errors.New("akses ditolak")
	ErrNotFound   = errors.New("student tidak ditemukan")
	ErrConflict   = errors.New("email atau NIM sudah digunakan")
	ErrValidation = errors.New("validasi gagal")
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string        { return ErrValidation.Error() }
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

type StudentPage struct {
	Students   []model.StudentListItem `json:"students"`
	Page       int                     `json:"page"`
	Limit      int                     `json:"limit"`
	Total      int64                   `json:"total"`
	TotalPages int64                   `json:"total_pages"`
}

type StudentService struct {
	repo repository.StudentRepository
}

func NewStudentService(repo repository.StudentRepository) *StudentService {
	return &StudentService{repo: repo}
}

func (s *StudentService) List(ctx context.Context, actor model.AuthIdentity, q model.StudentListQuery) (StudentPage, error) {
	if actor.Role != "admin" {
		return StudentPage{}, ErrForbidden
	}
	if q.Page <= 0 || q.Limit <= 0 || q.Limit > 50 {
		return StudentPage{}, validation(map[string]string{"page": "page harus positif dan limit antara 1 sampai 50"})
	}
	if q.Page-1 > int(^uint(0)>>1)/q.Limit {
		return StudentPage{}, validation(map[string]string{"page": "page terlalu besar"})
	}
	desc := strings.HasPrefix(q.Sort, "-")
	if desc {
		q.Sort = strings.TrimPrefix(q.Sort, "-")
	}
	allowedSorts := map[string]bool{"nama": true, "ipk_terakhir": true}
	if q.Sort == "" {
		q.Sort = "nama"
	} else if !allowedSorts[q.Sort] {
		return StudentPage{}, validation(map[string]string{"sort": "sort tidak didukung"})
	}
	if desc {
		q.SortOrder = "desc"
	}
	if q.SortOrder == "" {
		q.SortOrder = "asc"
	} else {
		q.SortOrder = strings.ToLower(q.SortOrder)
		if q.SortOrder != "asc" && q.SortOrder != "desc" {
			return StudentPage{}, validation(map[string]string{"order": "order harus asc atau desc"})
		}
	}
	q.Prodi = strings.TrimSpace(q.Prodi)
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Prodi) > 120 || len(q.Search) > 100 {
		return StudentPage{}, validation(map[string]string{"filter": "panjang filter melebihi batas"})
	}
	q.Offset = (q.Page - 1) * q.Limit
	total, err := s.repo.CountStudents(ctx, q)
	if err != nil {
		return StudentPage{}, err
	}
	students, err := s.repo.ListStudents(ctx, q)
	if err != nil {
		return StudentPage{}, err
	}
	pages := int64(0)
	if total > 0 {
		pages = (total + int64(q.Limit) - 1) / int64(q.Limit)
	}
	return StudentPage{Students: students, Page: q.Page, Limit: q.Limit, Total: total, TotalPages: pages}, nil
}

func (s *StudentService) Create(ctx context.Context, actor model.AuthIdentity, req model.CreateStudentRequest) (model.Student, error) {
	if actor.Role != "admin" {
		return model.Student{}, ErrForbidden
	}
	req = normalizeCreate(req)
	req.Password = req.NIM
	if fields := validateCreate(req); len(fields) > 0 {
		return model.Student{}, validation(fields)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return model.Student{}, err
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return model.Student{}, err
	}
	defer tx.Rollback(ctx)
	userID, err := tx.CreateUser(ctx, req.Email, string(hash), "mahasiswa")
	if err != nil {
		return model.Student{}, translateRepositoryError(err)
	}
	student, err := tx.CreateStudent(ctx, model.Student{
		UserID: userID, NIM: req.NIM, Nama: req.Nama, Prodi: req.Prodi,
		Angkatan: req.Angkatan, IPKTerakhir: req.IPKTerakhir,
	})
	if err != nil {
		return model.Student{}, translateRepositoryError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Student{}, err
	}
	return student, nil
}

func (s *StudentService) Get(ctx context.Context, actor model.AuthIdentity, id int64) (model.StudentDetail, error) {
	if actor.Role != "admin" && actor.Role != "mahasiswa" {
		return model.StudentDetail{}, ErrForbidden
	}
	student, email, err := s.repo.FindStudentByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return model.StudentDetail{}, ErrNotFound
	}
	if err != nil {
		return model.StudentDetail{}, err
	}
	if actor.Role == "mahasiswa" && student.UserID != actor.UserID {
		return model.StudentDetail{}, ErrForbidden
	}
	courses, err := s.repo.FindStudentCourses(ctx, student.ID)
	if err != nil {
		return model.StudentDetail{}, err
	}
	if courses == nil {
		courses = []model.StudentCourse{}
	}
	year := latestAcademicYear(courses)
	totalSKS := 0
	for _, course := range courses {
		if year == "" || course.TahunAkademik == year {
			totalSKS += course.SKS
		}
	}
	return model.StudentDetail{
		ID: student.ID, UserID: student.UserID, Email: email, NIM: student.NIM,
		Nama: student.Nama, Prodi: student.Prodi, Angkatan: student.Angkatan,
		IPKTerakhir: student.IPKTerakhir, Courses: courses, TotalSKS: totalSKS,
		TahunAkademik: year, BatasSKS: SKSLimit(student.IPKTerakhir),
	}, nil
}

func (s *StudentService) Update(ctx context.Context, actor model.AuthIdentity, id int64, req model.UpdateStudentRequest) (model.Student, error) {
	if actor.Role != "admin" {
		return model.Student{}, ErrForbidden
	}
	if req.NIM != nil {
		return model.Student{}, validation(map[string]string{"nim": "NIM tidak dapat diubah"})
	}
	req.Nama = strings.TrimSpace(req.Nama)
	req.Prodi = strings.TrimSpace(req.Prodi)
	if req.NIM != nil {
		return model.Student{}, validation(map[string]string{"nim": "NIM tidak dapat diubah"})
	}
	fields := validateStudentFields("", "", req.Nama, req.Prodi, req.Angkatan, req.IPKTerakhir)
	delete(fields, "email")
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			fields["email"] = "email tidak valid"
		}
	}
	if len(fields) > 0 {
		return model.Student{}, validation(fields)
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return model.Student{}, err
	}
	defer tx.Rollback(ctx)
	if req.Email != "" {
		current, _, findErr := s.repo.FindStudentByID(ctx, id)
		if errors.Is(findErr, repository.ErrNotFound) {
			return model.Student{}, ErrNotFound
		}
		if findErr != nil {
			return model.Student{}, findErr
		}
		if err := tx.UpdateUserEmail(ctx, current.UserID, req.Email); err != nil {
			return model.Student{}, translateRepositoryError(err)
		}
	}
	updated, err := tx.UpdateStudent(ctx, id, model.Student{
		Nama: req.Nama, Prodi: req.Prodi, Angkatan: req.Angkatan, IPKTerakhir: req.IPKTerakhir,
	})
	if err != nil {
		return model.Student{}, translateRepositoryError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Student{}, err
	}
	return updated, nil
}

func (s *StudentService) Delete(ctx context.Context, actor model.AuthIdentity, id int64) error {
	if actor.Role != "admin" {
		return ErrForbidden
	}
	err := s.repo.SoftDeleteStudent(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func SKSLimit(ipk float64) int {
	switch {
	case ipk >= 3.00:
		return 24
	case ipk >= 2.50:
		return 21
	default:
		return 18
	}
}

func latestAcademicYear(courses []model.StudentCourse) string {
	latest := ""
	for _, course := range courses {
		if course.TahunAkademik > latest {
			latest = course.TahunAkademik
		}
	}
	return latest
}

func normalizeCreate(req model.CreateStudentRequest) model.CreateStudentRequest {
	req.Email = strings.TrimSpace(req.Email)
	req.NIM = strings.TrimSpace(req.NIM)
	req.Nama = strings.TrimSpace(req.Nama)
	req.Prodi = strings.TrimSpace(req.Prodi)
	return req
}

func validateCreate(req model.CreateStudentRequest) map[string]string {
	fields := validateStudentFields(req.Email, req.Password, req.Nama, req.Prodi, req.Angkatan, req.IPKTerakhir, req.NIM)
	if req.Password == "" {
		fields["password"] = "password wajib diisi"
	}
	return fields
}

func validateStudentFields(email, password, nama, prodi string, angkatan int, ipk float64, nim ...string) map[string]string {
	errs := make(map[string]string)
	parsed, err := mail.ParseAddress(email)
	if email == "" || len(email) > 255 || err != nil || parsed.Address != email {
		errs["email"] = "email wajib diisi dengan format valid"
	}
	if password != "" && (len(password) < 8 || len(password) > 72) {
		errs["password"] = "password harus 8 sampai 72 karakter"
	}
	if len(nim) > 0 {
		if len(nim[0]) != 12 {
			errs["nim"] = "NIM wajib terdiri dari 12 digit"
		} else if _, err := strconv.Atoi(nim[0]); err != nil {
			errs["nim"] = "NIM wajib terdiri dari 12 digit"
		}
	}
	if nama == "" || len(nama) > 150 {
		errs["nama"] = "nama wajib diisi dan maksimal 150 karakter"
	}
	if prodi == "" || len(prodi) > 120 {
		errs["prodi"] = "prodi wajib diisi dan maksimal 120 karakter"
	}
	if angkatan < 1000 || angkatan > time.Now().Year() {
		errs["angkatan"] = "angkatan wajib 4 digit dan tidak boleh melebihi tahun berjalan"
	}
	if ipk < 0 || ipk > 4 {
		errs["ipk_terakhir"] = "IPK harus antara 0 dan 4"
	}
	return errs
}

func validation(fields map[string]string) error { return &ValidationError{Fields: fields} }

func translateRepositoryError(err error) error {
	switch {
	case errors.Is(err, repository.ErrDuplicateEmail), errors.Is(err, repository.ErrDuplicateNIM), errors.Is(err, repository.ErrConflict):
		return ErrConflict
	case errors.Is(err, repository.ErrNotFound):
		return ErrNotFound
	default:
		return err
	}
}
