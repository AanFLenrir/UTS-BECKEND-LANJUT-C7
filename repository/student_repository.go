package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"latihan-fiber/UTS/model"
)

var (
	ErrDuplicateEmail = errors.New("email sudah digunakan")
	ErrDuplicateNIM   = errors.New("NIM sudah digunakan")
	ErrConflict       = errors.New("data bertentangan dengan constraint")
)

type StudentRepository interface {
	CountStudents(context.Context, model.StudentListQuery) (int64, error)
	ListStudents(context.Context, model.StudentListQuery) ([]model.StudentListItem, error)
	FindStudentByID(context.Context, int64) (model.Student, string, error)
	FindStudentCourses(context.Context, int64) ([]model.StudentCourse, error)
	BeginTx(context.Context) (StudentTx, error)
	SoftDeleteStudent(context.Context, int64) error
}

type StudentTx interface {
	CreateUser(context.Context, string, string, string) (int64, error)
	CreateStudent(context.Context, model.Student) (model.Student, error)
	UpdateUserEmail(context.Context, int64, string) error
	UpdateStudent(context.Context, int64, model.Student) (model.Student, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

type postgresStudentRepository struct {
	pool *pgxpool.Pool
}

func NewStudentRepository(pool *pgxpool.Pool) StudentRepository {
	return &postgresStudentRepository{pool: pool}
}

func (r *postgresStudentRepository) BeginTx(ctx context.Context) (StudentTx, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin student transaction: %w", err)
	}
	return &postgresStudentTx{tx: tx}, nil
}

func (r *postgresStudentRepository) CountStudents(ctx context.Context, q model.StudentListQuery) (int64, error) {
	where, args := studentWhere(q)
	var total int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM students s "+where, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count students: %w", err)
	}
	return total, nil
}

func (r *postgresStudentRepository) ListStudents(ctx context.Context, q model.StudentListQuery) ([]model.StudentListItem, error) {
	where, args := studentWhere(q)
	sortColumns := map[string]string{
		"nim": "s.nim", "nama": "s.nama", "angkatan": "s.angkatan", "ipk_terakhir": "s.ipk_terakhir",
	}
	sortColumn := sortColumns[q.Sort]
	if sortColumn == "" {
		sortColumn = "s.nama"
	}
	order := "ASC"
	if strings.EqualFold(q.SortOrder, "desc") {
		order = "DESC"
	}
	args = append(args, q.Limit, q.Offset)
	query := fmt.Sprintf(`
		SELECT s.id,s.user_id,u.email,s.nim,s.nama,s.prodi,s.angkatan,s.ipk_terakhir
		FROM students s JOIN users u ON u.id=s.user_id
		%s ORDER BY %s %s,s.id ASC LIMIT $%d OFFSET $%d`,
		where, sortColumn, order, len(args)-1, len(args),
	)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list students: %w", err)
	}
	defer rows.Close()
	items := make([]model.StudentListItem, 0)
	for rows.Next() {
		var item model.StudentListItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.Email, &item.NIM, &item.Nama, &item.Prodi, &item.Angkatan, &item.IPKTerakhir); err != nil {
			return nil, fmt.Errorf("scan student: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate students: %w", err)
	}
	return items, nil
}

func (r *postgresStudentRepository) FindStudentByID(ctx context.Context, id int64) (model.Student, string, error) {
	var student model.Student
	var email string
	err := r.pool.QueryRow(ctx, `
		SELECT s.id,s.user_id,s.nim,s.nama,s.prodi,s.angkatan,s.ipk_terakhir,s.deleted_at,u.email
		FROM students s JOIN users u ON u.id=s.user_id
		WHERE s.id=$1 AND s.deleted_at IS NULL`, id,
	).Scan(&student.ID, &student.UserID, &student.NIM, &student.Nama, &student.Prodi, &student.Angkatan, &student.IPKTerakhir, &student.DeletedAt, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Student{}, "", ErrNotFound
	}
	if err != nil {
		return model.Student{}, "", fmt.Errorf("find student by id: %w", err)
	}
	return student, email, nil
}

func (r *postgresStudentRepository) FindStudentCourses(ctx context.Context, studentID int64) ([]model.StudentCourse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id,c.kode_mk,c.nama_mk,c.sks,c.semester,e.tahun_akademik
		FROM enrollments e JOIN courses c ON c.id=e.course_id
		WHERE e.student_id=$1
		ORDER BY e.tahun_akademik DESC,c.nama_mk ASC`, studentID)
	if err != nil {
		return nil, fmt.Errorf("find student enrollments: %w", err)
	}
	defer rows.Close()
	courses := make([]model.StudentCourse, 0)
	for rows.Next() {
		var course model.StudentCourse
		if err := rows.Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.TahunAkademik); err != nil {
			return nil, fmt.Errorf("scan enrolled course: %w", err)
		}
		courses = append(courses, course)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student enrollments: %w", err)
	}
	return courses, nil
}

func (r *postgresStudentRepository) SoftDeleteStudent(ctx context.Context, id int64) error {
	result, err := r.pool.Exec(ctx, `UPDATE students SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("soft delete student: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func studentWhere(q model.StudentListQuery) (string, []any) {
	conditions := []string{"s.deleted_at IS NULL"}
	args := make([]any, 0, 3)
	if q.Prodi != "" {
		args = append(args, q.Prodi)
		conditions = append(conditions, fmt.Sprintf("s.prodi=$%d", len(args)))
	}
	if q.Angkatan != nil {
		args = append(args, *q.Angkatan)
		conditions = append(conditions, fmt.Sprintf("s.angkatan=$%d", len(args)))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		conditions = append(conditions, fmt.Sprintf("(s.nim ILIKE $%d OR s.nama ILIKE $%d)", len(args), len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

type postgresStudentTx struct{ tx pgx.Tx }

func (t *postgresStudentTx) CreateUser(ctx context.Context, email, passwordHash, role string) (int64, error) {
	var id int64
	err := t.tx.QueryRow(ctx, `INSERT INTO users(email,password,role) VALUES($1,$2,$3) RETURNING id`, email, passwordHash, role).Scan(&id)
	if isUniqueViolation(err) {
		return 0, ErrDuplicateEmail
	}
	if err != nil {
		return 0, fmt.Errorf("insert student user: %w", err)
	}
	return id, nil
}

func (t *postgresStudentTx) CreateStudent(ctx context.Context, student model.Student) (model.Student, error) {
	err := t.tx.QueryRow(ctx, `
		INSERT INTO students(user_id,nim,nama,prodi,angkatan,ipk_terakhir)
		VALUES($1,$2,$3,$4,$5,$6)
		RETURNING id,user_id,nim,nama,prodi,angkatan,ipk_terakhir,deleted_at`,
		student.UserID, student.NIM, student.Nama, student.Prodi, student.Angkatan, student.IPKTerakhir,
	).Scan(&student.ID, &student.UserID, &student.NIM, &student.Nama, &student.Prodi, &student.Angkatan, &student.IPKTerakhir, &student.DeletedAt)
	if isUniqueViolation(err) {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "students_nim_key" {
			return model.Student{}, ErrDuplicateNIM
		}
		return model.Student{}, ErrConflict
	}
	if err != nil {
		return model.Student{}, fmt.Errorf("insert student: %w", err)
	}
	return student, nil
}

func (t *postgresStudentTx) UpdateUserEmail(ctx context.Context, userID int64, email string) error {
	_, err := t.tx.Exec(ctx, `UPDATE users SET email=$1 WHERE id=$2`, email, userID)
	if isUniqueViolation(err) {
		return ErrDuplicateEmail
	}
	if err != nil {
		return fmt.Errorf("update student account email: %w", err)
	}
	return nil
}

func (t *postgresStudentTx) UpdateStudent(ctx context.Context, id int64, student model.Student) (model.Student, error) {
	err := t.tx.QueryRow(ctx, `
		UPDATE students SET nama=$1,prodi=$2,angkatan=$3,ipk_terakhir=$4
		WHERE id=$5 AND deleted_at IS NULL
		RETURNING id,user_id,nim,nama,prodi,angkatan,ipk_terakhir,deleted_at`,
		student.Nama, student.Prodi, student.Angkatan, student.IPKTerakhir, id,
	).Scan(&student.ID, &student.UserID, &student.NIM, &student.Nama, &student.Prodi, &student.Angkatan, &student.IPKTerakhir, &student.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Student{}, ErrNotFound
	}
	if err != nil {
		return model.Student{}, fmt.Errorf("update student: %w", err)
	}
	return student, nil
}

func (t *postgresStudentTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *postgresStudentTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
