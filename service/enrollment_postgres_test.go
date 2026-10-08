package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"latihan-fiber/UTS/database"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

func testEnrollmentPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_ = godotenv.Load("../.env")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.NewPool(ctx)
	if err != nil {
		t.Skipf("PostgreSQL integration test skipped: %v", err)
	}
	return pool
}

type enrollmentFixture struct {
	userIDs    []int64
	studentIDs []int64
	courseID   int64
}

func makeEnrollmentFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, capacity int) enrollmentFixture {
	t.Helper()
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(stamp) > 18 {
		stamp = stamp[len(stamp)-18:]
	}
	fixture := enrollmentFixture{}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("integration-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		email := fmt.Sprintf("phase5-%s-%d@example.test", stamp, i)
		var userID, studentID int64
		if err := tx.QueryRow(ctx, `INSERT INTO users(email,password,role) VALUES($1,$2,'mahasiswa') RETURNING id`, email, string(passwordHash)).Scan(&userID); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create integration user: %v", err)
		}
		nim := fmt.Sprintf("T%s%d", stamp, i)
		if err := tx.QueryRow(ctx, `INSERT INTO students(user_id,nim,nama,prodi,angkatan,ipk_terakhir) VALUES($1,$2,$3,'Test Prodi',2024,3.50) RETURNING id`, userID, nim, "Phase Five Test Student").Scan(&studentID); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create integration student: %v", err)
		}
		fixture.userIDs = append(fixture.userIDs, userID)
		fixture.studentIDs = append(fixture.studentIDs, studentID)
	}
	code := "P5" + stamp
	if err := tx.QueryRow(ctx, `INSERT INTO courses(kode_mk,nama_mk,sks,semester,kuota) VALUES($1,'Phase Five Concurrency Course',3,1,$2) RETURNING id`, code, capacity).Scan(&fixture.courseID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create integration course: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM enrollments WHERE course_id=$1`, fixture.courseID); err != nil {
			t.Errorf("cleanup test enrollments: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM students WHERE id=ANY($1::BIGINT[])`, fixture.studentIDs); err != nil {
			t.Errorf("cleanup test students: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM users WHERE id=ANY($1::BIGINT[])`, fixture.userIDs); err != nil {
			t.Errorf("cleanup test users: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM courses WHERE id=$1`, fixture.courseID); err != nil {
			t.Errorf("cleanup test course: %v", err)
		}
	})
	return fixture
}

func TestPostgresEnrollmentInsertFailureRollsBack(t *testing.T) {
	pool := testEnrollmentPool(t)
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fixture := makeEnrollmentFixture(t, ctx, pool, 1)
	repo := repository.NewEnrollmentRepository(pool)
	tx, err := repo.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.GetCourseForUpdate(ctx, fixture.courseID); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	_, insertErr := tx.CreateEnrollment(ctx, model.Enrollment{StudentID: 9223372036854775807, CourseID: fixture.courseID, TahunAkademik: "2025/2026-Ganjil"})
	if insertErr == nil {
		_ = tx.Rollback(context.Background())
		t.Fatal("expected enrollment insert to fail its student foreign key")
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback failed insert: %v", err)
	}
	count, err := repo.CountEnrollmentsByCourse(ctx, fixture.courseID)
	if err != nil || count != 0 {
		t.Fatalf("enrollments after rollback = %d, err=%v", count, err)
	}
}

func TestPostgresConcurrentEnrollmentRespectsCourseQuota(t *testing.T) {
	pool := testEnrollmentPool(t)
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixture := makeEnrollmentFixture(t, ctx, pool, 1)
	svc := NewEnrollmentService(repository.NewEnrollmentRepository(pool))
	start := make(chan struct{})
	type outcome struct{ err error }
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for i, userID := range fixture.userIDs {
		wg.Add(1)
		go func(i int, userID int64) {
			defer wg.Done()
			<-start
			_, err := svc.Create(ctx, model.AuthIdentity{UserID: userID, Role: "mahasiswa", Email: fmt.Sprintf("phase5-%d@example.test", i)}, model.EnrollmentRequest{CourseID: fixture.courseID, TahunAkademik: "2025/2026-Ganjil"})
			results <- outcome{err: err}
		}(i, userID)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, full := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			successes++
		case errors.Is(result.err, ErrCourseFull):
			full++
		default:
			t.Errorf("unexpected concurrent enrollment error: %v", result.err)
		}
	}
	count, err := repository.NewEnrollmentRepository(pool).CountEnrollmentsByCourse(ctx, fixture.courseID)
	if err != nil {
		t.Fatal(err)
	}
	if successes != 1 || full != 1 || count != 1 {
		t.Fatalf("success=%d full=%d stored=%d; want one of each", successes, full, count)
	}
}
