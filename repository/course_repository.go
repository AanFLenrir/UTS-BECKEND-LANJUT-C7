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
	ErrDuplicateCourse  = errors.New("kode mata kuliah sudah digunakan")
	ErrCourseHasEntries = errors.New("course masih memiliki enrollment")
)

type CourseRepository interface {
	CountCourses(context.Context, model.CourseListQuery) (int64, error)
	ListCourses(context.Context, model.CourseListQuery) ([]model.Course, error)
	FindCourseByID(context.Context, int64) (model.Course, error)
	CreateCourse(context.Context, model.CourseInput) (model.Course, error)
	CountEnrollmentsByCourse(context.Context, int64) (int64, error)
	BeginTx(context.Context) (CourseTx, error)
}

type CourseTx interface {
	FindCourseForUpdate(context.Context, int64) (model.Course, error)
	CountEnrollmentsByCourse(context.Context, int64) (int64, error)
	UpdateCourse(context.Context, int64, model.CourseInput) (model.Course, error)
	DeleteCourse(context.Context, int64) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type postgresCourseRepository struct{ pool *pgxpool.Pool }

func NewCourseRepository(pool *pgxpool.Pool) CourseRepository {
	return &postgresCourseRepository{pool: pool}
}

func (r *postgresCourseRepository) CountCourses(ctx context.Context, q model.CourseListQuery) (int64, error) {
	where, args := courseWhere(q)
	query := `WITH ec AS (SELECT course_id,COUNT(*)::BIGINT AS filled FROM enrollments GROUP BY course_id)
		SELECT COUNT(*) FROM courses c LEFT JOIN ec ON ec.course_id=c.id ` + where
	var total int64
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count courses: %w", err)
	}
	return total, nil
}

func (r *postgresCourseRepository) ListCourses(ctx context.Context, q model.CourseListQuery) ([]model.Course, error) {
	where, args := courseWhere(q)
	sortColumns := map[string]string{
		"kode_mk": "c.kode_mk", "nama_mk": "c.nama_mk", "sks": "c.sks", "semester": "c.semester", "kuota": "c.kuota",
	}
	sortColumn := sortColumns[q.Sort]
	if sortColumn == "" {
		sortColumn = "c.kode_mk"
	}
	order := "ASC"
	if strings.EqualFold(q.SortOrder, "desc") {
		order = "DESC"
	}
	args = append(args, q.Limit, q.Offset)
	query := fmt.Sprintf(`
		WITH ec AS (SELECT course_id,COUNT(*)::BIGINT AS filled FROM enrollments GROUP BY course_id)
		SELECT c.id,c.kode_mk,c.nama_mk,c.sks,c.semester,c.kuota,
			COALESCE(ec.filled,0),GREATEST(c.kuota-COALESCE(ec.filled,0),0)
		FROM courses c LEFT JOIN ec ON ec.course_id=c.id
		%s ORDER BY %s %s,c.id ASC LIMIT $%d OFFSET $%d`,
		where, sortColumn, order, len(args)-1, len(args),
	)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list courses: %w", err)
	}
	defer rows.Close()
	courses := make([]model.Course, 0)
	for rows.Next() {
		course, err := scanCourse(rows)
		if err != nil {
			return nil, err
		}
		courses = append(courses, course)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate courses: %w", err)
	}
	return courses, nil
}

func (r *postgresCourseRepository) FindCourseByID(ctx context.Context, id int64) (model.Course, error) {
	var course model.Course
	err := r.pool.QueryRow(ctx, `
		SELECT c.id,c.kode_mk,c.nama_mk,c.sks,c.semester,c.kuota,
			COUNT(e.id)::BIGINT,GREATEST(c.kuota-COUNT(e.id)::BIGINT,0)
		FROM courses c LEFT JOIN enrollments e ON e.course_id=c.id
		WHERE c.id=$1 GROUP BY c.id`, id,
	).Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.Kuota, &course.Terisi, &course.SisaKuota)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Course{}, ErrNotFound
	}
	if err != nil {
		return model.Course{}, fmt.Errorf("find course: %w", err)
	}
	return course, nil
}

func (r *postgresCourseRepository) CreateCourse(ctx context.Context, input model.CourseInput) (model.Course, error) {
	var course model.Course
	err := r.pool.QueryRow(ctx, `
		INSERT INTO courses(kode_mk,nama_mk,sks,semester,kuota)
		VALUES($1,$2,$3,$4,$5)
		RETURNING id,kode_mk,nama_mk,sks,semester,kuota`,
		input.KodeMK, input.NamaMK, input.SKS, input.Semester, input.Kuota,
	).Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.Kuota)
	if isCourseUnique(err) {
		return model.Course{}, ErrDuplicateCourse
	}
	if err != nil {
		return model.Course{}, fmt.Errorf("create course: %w", err)
	}
	course.SisaKuota = int64(course.Kuota)
	return course, nil
}

func (r *postgresCourseRepository) CountEnrollmentsByCourse(ctx context.Context, courseID int64) (int64, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM enrollments WHERE course_id=$1`, courseID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count course enrollments: %w", err)
	}
	return count, nil
}

func (r *postgresCourseRepository) BeginTx(ctx context.Context) (CourseTx, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin course transaction: %w", err)
	}
	return &postgresCourseTx{tx: tx}, nil
}

func courseWhere(q model.CourseListQuery) (string, []any) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if q.Semester != nil {
		args = append(args, *q.Semester)
		conditions = append(conditions, fmt.Sprintf("c.semester=$%d", len(args)))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		conditions = append(conditions, fmt.Sprintf("(c.kode_mk ILIKE $%d OR c.nama_mk ILIKE $%d)", len(args), len(args)))
	}
	if q.AvailableOnly {
		conditions = append(conditions, "c.kuota>COALESCE(ec.filled,0)")
	}
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func scanCourse(row pgx.Row) (model.Course, error) {
	var course model.Course
	err := row.Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.Kuota, &course.Terisi, &course.SisaKuota)
	if err != nil {
		return model.Course{}, fmt.Errorf("scan course: %w", err)
	}
	return course, nil
}

func isCourseUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type postgresCourseTx struct{ tx pgx.Tx }

func (t *postgresCourseTx) FindCourseForUpdate(ctx context.Context, id int64) (model.Course, error) {
	var course model.Course
	err := t.tx.QueryRow(ctx, `SELECT id,kode_mk,nama_mk,sks,semester,kuota FROM courses WHERE id=$1 FOR UPDATE`, id).
		Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.Kuota)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Course{}, ErrNotFound
	}
	if err != nil {
		return model.Course{}, fmt.Errorf("lock course: %w", err)
	}
	return course, nil
}

func (t *postgresCourseTx) CountEnrollmentsByCourse(ctx context.Context, id int64) (int64, error) {
	var count int64
	if err := t.tx.QueryRow(ctx, `SELECT COUNT(*) FROM enrollments WHERE course_id=$1`, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("count course enrollments in transaction: %w", err)
	}
	return count, nil
}

func (t *postgresCourseTx) UpdateCourse(ctx context.Context, id int64, input model.CourseInput) (model.Course, error) {
	var course model.Course
	err := t.tx.QueryRow(ctx, `
		UPDATE courses SET kode_mk=$1,nama_mk=$2,sks=$3,semester=$4,kuota=$5
		WHERE id=$6 RETURNING id,kode_mk,nama_mk,sks,semester,kuota`,
		input.KodeMK, input.NamaMK, input.SKS, input.Semester, input.Kuota, id,
	).Scan(&course.ID, &course.KodeMK, &course.NamaMK, &course.SKS, &course.Semester, &course.Kuota)
	if isCourseUnique(err) {
		return model.Course{}, ErrDuplicateCourse
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Course{}, ErrNotFound
	}
	if err != nil {
		return model.Course{}, fmt.Errorf("update course: %w", err)
	}
	return course, nil
}

func (t *postgresCourseTx) DeleteCourse(ctx context.Context, id int64) error {
	command, err := t.tx.Exec(ctx, `DELETE FROM courses WHERE id=$1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrCourseHasEntries
		}
		return fmt.Errorf("delete course: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (t *postgresCourseTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *postgresCourseTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
