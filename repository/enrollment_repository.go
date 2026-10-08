package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"latihan-fiber/UTS/model"
)

var ErrDuplicateEnrollment = errors.New("enrollment sudah ada")

type EnrollmentRepository interface {
	BeginTx(context.Context) (EnrollmentTx, error)
	GetStudentByID(context.Context, int64) (model.Student, error)
	GetStudentByUserID(context.Context, int64) (model.Student, error)
	ListEnrollmentsByStudent(context.Context, int64, string) ([]model.EnrollmentView, error)
	FindEnrollmentByID(context.Context, int64) (model.EnrollmentRecord, error)
	CountEnrollmentsByCourse(context.Context, int64) (int64, error)
	GetTotalStudentCredits(context.Context, int64, string) (int, error)
}

type EnrollmentTx interface {
	GetCourseForUpdate(context.Context, int64) (model.Course, error)
	GetStudentByUserIDForUpdate(context.Context, int64) (model.Student, error)
	FindEnrollmentByStudentCourseYear(context.Context, int64, int64, string) (bool, error)
	CountEnrollmentsByCourse(context.Context, int64) (int64, error)
	GetTotalStudentCredits(context.Context, int64, string) (int, error)
	CreateEnrollment(context.Context, model.Enrollment) (model.Enrollment, error)
	FindEnrollmentByID(context.Context, int64) (model.EnrollmentRecord, error)
	DeleteEnrollment(context.Context, int64) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type postgresEnrollmentRepository struct{ pool *pgxpool.Pool }

func NewEnrollmentRepository(pool *pgxpool.Pool) EnrollmentRepository {
	return &postgresEnrollmentRepository{pool: pool}
}

func (r *postgresEnrollmentRepository) BeginTx(ctx context.Context) (EnrollmentTx, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin enrollment transaction: %w", err)
	}
	return &postgresEnrollmentTx{tx: tx}, nil
}

func (r *postgresEnrollmentRepository) GetStudentByID(ctx context.Context, id int64) (model.Student, error) {
	return findActiveStudent(ctx, r.pool, `SELECT id,user_id,nim,nama,prodi,angkatan,ipk_terakhir,deleted_at FROM students WHERE id=$1 AND deleted_at IS NULL`, id)
}

func (r *postgresEnrollmentRepository) GetStudentByUserID(ctx context.Context, userID int64) (model.Student, error) {
	return findActiveStudent(ctx, r.pool, `SELECT id,user_id,nim,nama,prodi,angkatan,ipk_terakhir,deleted_at FROM students WHERE user_id=$1 AND deleted_at IS NULL`, userID)
}

func findActiveStudent(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sql string, id int64) (model.Student, error) {
	var student model.Student
	err := q.QueryRow(ctx, sql, id).Scan(&student.ID, &student.UserID, &student.NIM, &student.Nama, &student.Prodi, &student.Angkatan, &student.IPKTerakhir, &student.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Student{}, ErrNotFound
	}
	if err != nil {
		return model.Student{}, fmt.Errorf("find active student: %w", err)
	}
	return student, nil
}

func (r *postgresEnrollmentRepository) ListEnrollmentsByStudent(ctx context.Context, studentID int64, year string) ([]model.EnrollmentView, error) {
	query := `SELECT e.id,c.id,c.kode_mk,c.nama_mk,c.sks,c.semester,e.tahun_akademik,e.created_at
		FROM enrollments e JOIN courses c ON c.id=e.course_id WHERE e.student_id=$1`
	args := []any{studentID}
	if year != "" {
		query += ` AND e.tahun_akademik=$2`
		args = append(args, year)
	}
	query += ` ORDER BY e.tahun_akademik DESC,e.created_at ASC,e.id ASC`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list student enrollments: %w", err)
	}
	defer rows.Close()
	result := make([]model.EnrollmentView, 0)
	for rows.Next() {
		var item model.EnrollmentView
		if err := rows.Scan(&item.ID, &item.Course.ID, &item.Course.KodeMK, &item.Course.NamaMK, &item.Course.SKS, &item.Course.Semester, &item.TahunAkademik, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan student enrollment: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student enrollments: %w", err)
	}
	return result, nil
}

func (r *postgresEnrollmentRepository) FindEnrollmentByID(ctx context.Context, id int64) (model.EnrollmentRecord, error) {
	return findEnrollmentRecord(ctx, r.pool, id, false)
}

func (r *postgresEnrollmentRepository) GetTotalStudentCredits(ctx context.Context, studentID int64, year string) (int, error) {
	return totalCredits(ctx, r.pool, studentID, year)
}

func (r *postgresEnrollmentRepository) CountEnrollmentsByCourse(ctx context.Context, courseID int64) (int64, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM enrollments WHERE course_id=$1`, courseID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count enrollments by course: %w", err)
	}
	return count, nil
}

type postgresEnrollmentTx struct{ tx pgx.Tx }

func (t *postgresEnrollmentTx) GetCourseForUpdate(ctx context.Context, id int64) (model.Course, error) {
	var course model.Course
	err := t.tx.QueryRow(ctx, `SELECT id,kode_mk,nama_mk,sks,semester,kuota FROM courses WHERE id=$1 FOR UPDATE`, id).
		Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.Kuota)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Course{}, ErrNotFound
	}
	if err != nil {
		return model.Course{}, fmt.Errorf("lock course for enrollment: %w", err)
	}
	return course, nil
}

func (t *postgresEnrollmentTx) GetStudentByUserIDForUpdate(ctx context.Context, userID int64) (model.Student, error) {
	var student model.Student
	err := t.tx.QueryRow(ctx, `SELECT id,user_id,nim,nama,prodi,angkatan,ipk_terakhir,deleted_at
		FROM students WHERE user_id=$1 AND deleted_at IS NULL FOR UPDATE`, userID).
		Scan(&student.ID, &student.UserID, &student.NIM, &student.Nama, &student.Prodi, &student.Angkatan, &student.IPKTerakhir, &student.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Student{}, ErrNotFound
	}
	if err != nil {
		return model.Student{}, fmt.Errorf("lock student for enrollment: %w", err)
	}
	return student, nil
}

func (t *postgresEnrollmentTx) FindEnrollmentByStudentCourseYear(ctx context.Context, studentID, courseID int64, year string) (bool, error) {
	var exists bool
	err := t.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM enrollments WHERE student_id=$1 AND course_id=$2 AND tahun_akademik=$3)`, studentID, courseID, year).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check duplicate enrollment: %w", err)
	}
	return exists, nil
}

func (t *postgresEnrollmentTx) CountEnrollmentsByCourse(ctx context.Context, courseID int64) (int64, error) {
	var count int64
	if err := t.tx.QueryRow(ctx, `SELECT COUNT(*) FROM enrollments WHERE course_id=$1`, courseID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count enrollments in transaction: %w", err)
	}
	return count, nil
}

func (t *postgresEnrollmentTx) GetTotalStudentCredits(ctx context.Context, studentID int64, year string) (int, error) {
	return totalCredits(ctx, t.tx, studentID, year)
}

func totalCredits(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, studentID int64, year string) (int, error) {
	var total int
	err := q.QueryRow(ctx, `SELECT COALESCE(SUM(c.sks),0)::INTEGER FROM enrollments e JOIN courses c ON c.id=e.course_id WHERE e.student_id=$1 AND e.tahun_akademik=$2`, studentID, year).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum student credits: %w", err)
	}
	return total, nil
}

func (t *postgresEnrollmentTx) CreateEnrollment(ctx context.Context, enrollment model.Enrollment) (model.Enrollment, error) {
	err := t.tx.QueryRow(ctx, `INSERT INTO enrollments(student_id,course_id,tahun_akademik) VALUES($1,$2,$3) RETURNING id,student_id,course_id,tahun_akademik,created_at`, enrollment.StudentID, enrollment.CourseID, enrollment.TahunAkademik).
		Scan(&enrollment.ID, &enrollment.StudentID, &enrollment.CourseID, &enrollment.TahunAkademik, &enrollment.CreatedAt)
	if isEnrollmentUnique(err) {
		return model.Enrollment{}, ErrDuplicateEnrollment
	}
	if err != nil {
		return model.Enrollment{}, fmt.Errorf("insert enrollment: %w", err)
	}
	return enrollment, nil
}

func (t *postgresEnrollmentTx) FindEnrollmentByID(ctx context.Context, id int64) (model.EnrollmentRecord, error) {
	return findEnrollmentRecord(ctx, t.tx, id, true)
}

func findEnrollmentRecord(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id int64, lock bool) (model.EnrollmentRecord, error) {
	sql := `SELECT e.id,e.student_id,e.course_id,e.tahun_akademik,e.created_at,s.user_id,c.id,c.kode_mk,c.nama_mk,c.sks,c.semester
		FROM enrollments e JOIN students s ON s.id=e.student_id JOIN courses c ON c.id=e.course_id WHERE e.id=$1`
	if lock {
		sql += ` FOR UPDATE OF e`
	}
	var record model.EnrollmentRecord
	err := q.QueryRow(ctx, sql, id).Scan(&record.Enrollment.ID, &record.Enrollment.StudentID, &record.Enrollment.CourseID, &record.Enrollment.TahunAkademik, &record.Enrollment.CreatedAt, &record.UserID, &record.Course.ID, &record.Course.KodeMK, &record.Course.NamaMK, &record.Course.SKS, &record.Course.Semester)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EnrollmentRecord{}, ErrNotFound
	}
	if err != nil {
		return model.EnrollmentRecord{}, fmt.Errorf("find enrollment: %w", err)
	}
	return record, nil
}

func (t *postgresEnrollmentTx) DeleteEnrollment(ctx context.Context, id int64) error {
	command, err := t.tx.Exec(ctx, `DELETE FROM enrollments WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete enrollment: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *postgresEnrollmentTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *postgresEnrollmentTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

func isEnrollmentUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
