package service

import (
	"context"
	"errors"
	"testing"

	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

type fakeEnrollmentRepository struct {
	student       model.Student
	studentErr    error
	list          []model.EnrollmentView
	listErr       error
	credits       int
	creditsErr    error
	courseCount   int64
	record        model.EnrollmentRecord
	recordErr     error
	tx            *fakeEnrollmentTx
	listStudentID int64
	listYear      string
}

func (f *fakeEnrollmentRepository) BeginTx(context.Context) (repository.EnrollmentTx, error) {
	return f.tx, nil
}
func (f *fakeEnrollmentRepository) GetStudentByID(_ context.Context, id int64) (model.Student, error) {
	if f.student.ID != id && f.studentErr == nil {
		return model.Student{}, repository.ErrNotFound
	}
	return f.student, f.studentErr
}
func (f *fakeEnrollmentRepository) GetStudentByUserID(context.Context, int64) (model.Student, error) {
	return f.student, f.studentErr
}
func (f *fakeEnrollmentRepository) ListEnrollmentsByStudent(_ context.Context, id int64, year string) ([]model.EnrollmentView, error) {
	f.listStudentID, f.listYear = id, year
	return f.list, f.listErr
}
func (f *fakeEnrollmentRepository) FindEnrollmentByID(context.Context, int64) (model.EnrollmentRecord, error) {
	return f.record, f.recordErr
}
func (f *fakeEnrollmentRepository) CountEnrollmentsByCourse(context.Context, int64) (int64, error) {
	return f.courseCount, nil
}
func (f *fakeEnrollmentRepository) GetTotalStudentCredits(context.Context, int64, string) (int, error) {
	return f.credits, f.creditsErr
}

type fakeEnrollmentTx struct {
	course       model.Course
	courseErr    error
	student      model.Student
	studentErr   error
	duplicate    bool
	duplicateErr error
	filled       int64
	filledErr    error
	credits      int
	creditsErr   error
	created      model.Enrollment
	createErr    error
	record       model.EnrollmentRecord
	recordErr    error
	deleteErr    error
	deletedID    int64
	committed    bool
	rolledBack   bool
}

func (f *fakeEnrollmentTx) GetCourseForUpdate(context.Context, int64) (model.Course, error) {
	return f.course, f.courseErr
}
func (f *fakeEnrollmentTx) GetStudentByUserIDForUpdate(context.Context, int64) (model.Student, error) {
	return f.student, f.studentErr
}
func (f *fakeEnrollmentTx) FindEnrollmentByStudentCourseYear(context.Context, int64, int64, string) (bool, error) {
	return f.duplicate, f.duplicateErr
}
func (f *fakeEnrollmentTx) CountEnrollmentsByCourse(context.Context, int64) (int64, error) {
	return f.filled, f.filledErr
}
func (f *fakeEnrollmentTx) GetTotalStudentCredits(context.Context, int64, string) (int, error) {
	return f.credits, f.creditsErr
}
func (f *fakeEnrollmentTx) CreateEnrollment(_ context.Context, enrollment model.Enrollment) (model.Enrollment, error) {
	enrollment.ID = 101
	f.created = enrollment
	return enrollment, f.createErr
}
func (f *fakeEnrollmentTx) FindEnrollmentByID(context.Context, int64) (model.EnrollmentRecord, error) {
	return f.record, f.recordErr
}
func (f *fakeEnrollmentTx) DeleteEnrollment(_ context.Context, id int64) error {
	f.deletedID = id
	return f.deleteErr
}
func (f *fakeEnrollmentTx) Commit(context.Context) error { f.committed = true; return nil }
func (f *fakeEnrollmentTx) Rollback(context.Context) error {
	if !f.committed {
		f.rolledBack = true
	}
	return nil
}

func enrollmentStudent() model.AuthIdentity {
	return model.AuthIdentity{UserID: 70, Email: "student@example.com", Role: "mahasiswa"}
}
func enrollmentAdmin() model.AuthIdentity {
	return model.AuthIdentity{UserID: 1, Email: "admin@example.com", Role: "admin"}
}
func enrollmentRequest() model.EnrollmentRequest {
	return model.EnrollmentRequest{CourseID: 8, TahunAkademik: "2025/2026-Ganjil"}
}

func readyEnrollmentTx() *fakeEnrollmentTx {
	return &fakeEnrollmentTx{
		course:  model.Course{ID: 8, KodeMK: "IF201", NamaMK: "Basis Data", SKS: 3, Semester: 3, Kuota: 10},
		student: model.Student{ID: 4, UserID: 70, IPKTerakhir: 3.25},
	}
}

func TestEnrollmentCreateSuccess(t *testing.T) {
	tx := readyEnrollmentTx()
	svc := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx})
	created, err := svc.Create(context.Background(), enrollmentStudent(), enrollmentRequest())
	if err != nil || created.ID != 101 || created.Course.ID != 8 || created.TahunAkademik != "2025/2026-Ganjil" || !tx.committed {
		t.Fatalf("created=%#v err=%v tx=%#v", created, err, tx)
	}
}

func TestEnrollmentCreateMissingStudentOrCourse(t *testing.T) {
	tx := readyEnrollmentTx()
	tx.courseErr = repository.ErrNotFound
	if _, err := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx}).Create(context.Background(), enrollmentStudent(), enrollmentRequest()); !errors.Is(err, ErrCourseNotFound) {
		t.Fatalf("course missing error=%v", err)
	}
	tx = readyEnrollmentTx()
	tx.studentErr = repository.ErrNotFound
	if _, err := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx}).Create(context.Background(), enrollmentStudent(), enrollmentRequest()); !errors.Is(err, ErrEnrollmentStudentNotFound) {
		t.Fatalf("student missing error=%v", err)
	}
}

func TestEnrollmentCreateDuplicateFullAndCreditLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		prep func(*fakeEnrollmentTx)
		want error
	}{
		{"duplicate", func(tx *fakeEnrollmentTx) { tx.duplicate = true }, ErrEnrollmentDuplicate},
		{"full", func(tx *fakeEnrollmentTx) { tx.filled = 10 }, ErrCourseFull},
		{"credit limit", func(tx *fakeEnrollmentTx) { tx.student.IPKTerakhir = 2.49; tx.credits = 16 }, ErrCreditLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := readyEnrollmentTx()
			tc.prep(tx)
			_, err := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx}).Create(context.Background(), enrollmentStudent(), enrollmentRequest())
			if !errors.Is(err, tc.want) || !tx.rolledBack || tx.committed {
				t.Fatalf("error=%v tx=%#v", err, tx)
			}
		})
	}
}

func TestEnrollmentCreateRequiresStudentRoleAndValidInput(t *testing.T) {
	svc := NewEnrollmentService(&fakeEnrollmentRepository{tx: readyEnrollmentTx()})
	if _, err := svc.Create(context.Background(), enrollmentAdmin(), enrollmentRequest()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin create error=%v", err)
	}
	bad := enrollmentRequest()
	bad.CourseID, bad.TahunAkademik = 0, ""
	_, err := svc.Create(context.Background(), enrollmentStudent(), bad)
	var validationErr *EnrollmentValidationError
	if !errors.As(err, &validationErr) || len(validationErr.Fields) != 2 {
		t.Fatalf("validation error=%#v", err)
	}
}

func TestListEnrollmentsOwnershipAndAdminAccess(t *testing.T) {
	repo := &fakeEnrollmentRepository{
		student: model.Student{ID: 4, UserID: 70, NIM: "231524001", Nama: "Mahasiswa", IPKTerakhir: 3.1},
		list: []model.EnrollmentView{
			{ID: 1, TahunAkademik: "2024/2025-Ganjil", Course: model.CourseSummary{SKS: 2}},
			{ID: 2, TahunAkademik: "2025/2026-Ganjil", Course: model.CourseSummary{SKS: 3}},
		},
		credits: 3,
	}
	svc := NewEnrollmentService(repo)
	result, err := svc.ListByStudent(context.Background(), enrollmentStudent(), 4, "")
	if err != nil || result.Student.ID != 4 || len(result.Enrollments) != 2 || result.TotalSKS != 3 || result.TahunAkademik != "2025/2026-Ganjil" {
		t.Fatalf("own list=%#v err=%v", result, err)
	}
	if _, err := svc.ListByStudent(context.Background(), model.AuthIdentity{UserID: 99, Role: "mahasiswa"}, 4, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other student list error=%v", err)
	}
	if _, err := svc.ListByStudent(context.Background(), enrollmentAdmin(), 4, "2025/2026-Ganjil"); err != nil || repo.listYear != "2025/2026-Ganjil" {
		t.Fatalf("admin list err=%v year=%q", err, repo.listYear)
	}
}

func TestDeleteEnrollmentOwnershipAndAdmin(t *testing.T) {
	record := model.EnrollmentRecord{Enrollment: model.Enrollment{ID: 12}, UserID: 70}
	tx := &fakeEnrollmentTx{record: record}
	svc := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx})
	if err := svc.Delete(context.Background(), model.AuthIdentity{UserID: 71, Role: "mahasiswa"}, 12); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign delete error=%v", err)
	}
	if !tx.rolledBack || tx.deletedID != 0 {
		t.Fatalf("foreign delete changed enrollment: %#v", tx)
	}
	tx = &fakeEnrollmentTx{record: record}
	svc = NewEnrollmentService(&fakeEnrollmentRepository{tx: tx})
	if err := svc.Delete(context.Background(), enrollmentStudent(), 12); err != nil || tx.deletedID != 12 || !tx.committed {
		t.Fatalf("owner delete err=%v tx=%#v", err, tx)
	}
	tx = &fakeEnrollmentTx{record: record}
	if err := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx}).Delete(context.Background(), enrollmentAdmin(), 12); !errors.Is(err, ErrForbidden) || tx.committed {
		t.Fatalf("admin delete err=%v tx=%#v", err, tx)
	}
}

func TestDeleteEnrollmentNotFound(t *testing.T) {
	tx := &fakeEnrollmentTx{recordErr: repository.ErrNotFound}
	err := NewEnrollmentService(&fakeEnrollmentRepository{tx: tx}).Delete(context.Background(), enrollmentStudent(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing error=%v", err)
	}
}
