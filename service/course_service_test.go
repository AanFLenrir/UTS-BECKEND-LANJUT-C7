package service

import (
	"context"
	"errors"
	"testing"

	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

type fakeCourseRepository struct {
	count     int64
	courses   []model.Course
	course    model.Course
	listErr   error
	countErr  error
	findErr   error
	createErr error
	createIn  model.CourseInput
	tx        *fakeCourseTx
	query     model.CourseListQuery
}

func (f *fakeCourseRepository) CountCourses(_ context.Context, q model.CourseListQuery) (int64, error) {
	f.query = q
	return f.count, f.countErr
}
func (f *fakeCourseRepository) ListCourses(_ context.Context, q model.CourseListQuery) ([]model.Course, error) {
	f.query = q
	return f.courses, f.listErr
}
func (f *fakeCourseRepository) FindCourseByID(context.Context, int64) (model.Course, error) {
	return f.course, f.findErr
}
func (f *fakeCourseRepository) CreateCourse(_ context.Context, input model.CourseInput) (model.Course, error) {
	f.createIn = input
	course := model.Course{ID: 22, KodeMK: input.KodeMK, NamaMK: input.NamaMK, SKS: input.SKS, Semester: input.Semester, Kuota: input.Kuota, SisaKuota: int64(input.Kuota)}
	return course, f.createErr
}
func (f *fakeCourseRepository) CountEnrollmentsByCourse(context.Context, int64) (int64, error) {
	return 0, nil
}
func (f *fakeCourseRepository) BeginTx(context.Context) (repository.CourseTx, error) {
	return f.tx, nil
}

type fakeCourseTx struct {
	course     model.Course
	filled     int64
	findErr    error
	countErr   error
	updateErr  error
	deleteErr  error
	updated    model.CourseInput
	deletedID  int64
	committed  bool
	rolledBack bool
}

func (f *fakeCourseTx) FindCourseForUpdate(context.Context, int64) (model.Course, error) {
	return f.course, f.findErr
}
func (f *fakeCourseTx) CountEnrollmentsByCourse(context.Context, int64) (int64, error) {
	return f.filled, f.countErr
}
func (f *fakeCourseTx) UpdateCourse(_ context.Context, id int64, input model.CourseInput) (model.Course, error) {
	f.updated = input
	return model.Course{ID: id, KodeMK: input.KodeMK, NamaMK: input.NamaMK, SKS: input.SKS, Semester: input.Semester, Kuota: input.Kuota}, f.updateErr
}
func (f *fakeCourseTx) DeleteCourse(_ context.Context, id int64) error {
	f.deletedID = id
	return f.deleteErr
}
func (f *fakeCourseTx) Commit(context.Context) error   { f.committed = true; return nil }
func (f *fakeCourseTx) Rollback(context.Context) error { f.rolledBack = true; return nil }

func courseAdmin() model.AuthIdentity   { return model.AuthIdentity{UserID: 1, Role: "admin"} }
func courseStudent() model.AuthIdentity { return model.AuthIdentity{UserID: 2, Role: "mahasiswa"} }
func validCourse() model.CourseInput {
	return model.CourseInput{KodeMK: "IF301", NamaMK: "Algoritma Lanjut", SKS: 3, Semester: 5, Kuota: 40}
}

func TestCreateCourseAdminOnlyAndDuplicate(t *testing.T) {
	repo := &fakeCourseRepository{}
	svc := NewCourseService(repo)
	if _, err := svc.Create(context.Background(), courseStudent(), validCourse()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student create error = %v", err)
	}
	created, err := svc.Create(context.Background(), courseAdmin(), validCourse())
	if err != nil || created.ID != 22 || created.SisaKuota != 40 {
		t.Fatalf("create result=%#v err=%v", created, err)
	}
	repo.createErr = repository.ErrDuplicateCourse
	if _, err := svc.Create(context.Background(), courseAdmin(), validCourse()); !errors.Is(err, ErrCourseConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
}

func TestCourseListPaginationFilterSearchSortAndQuota(t *testing.T) {
	semester := 3
	repo := &fakeCourseRepository{count: 11, courses: []model.Course{{ID: 1, Kuota: 10, Terisi: 12, SisaKuota: -2}}}
	svc := NewCourseService(repo)
	page, err := svc.List(context.Background(), courseStudent(), model.CourseListQuery{
		Page: 2, Limit: 5, Semester: &semester, Search: "algoritma", Sort: "nama_mk", SortOrder: "desc", AvailableOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalPages != 3 || repo.query.Offset != 5 || repo.query.Semester == nil || *repo.query.Semester != 3 || repo.query.Search != "algoritma" || repo.query.Sort != "nama_mk" || repo.query.SortOrder != "desc" || !repo.query.AvailableOnly {
		t.Fatalf("query/pagination not applied: %#v %#v", page, repo.query)
	}
	if page.Courses[0].Terisi != 12 || page.Courses[0].SisaKuota != 0 {
		t.Fatalf("quota values were not clamped: %#v", page.Courses[0])
	}
	if _, err := svc.List(context.Background(), courseAdmin(), model.CourseListQuery{Page: 1, Limit: 10, Sort: "kode_mk;DROP"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("unsafe sort error = %v", err)
	}
}

func TestCourseDetailAndMissing(t *testing.T) {
	repo := &fakeCourseRepository{course: model.Course{ID: 3, Kuota: 30, Terisi: 7, SisaKuota: 23}}
	svc := NewCourseService(repo)
	got, err := svc.Get(context.Background(), courseStudent(), 3)
	if err != nil || got.Terisi != 7 || got.SisaKuota != 23 {
		t.Fatalf("detail=%#v err=%v", got, err)
	}
	repo.findErr = repository.ErrNotFound
	if _, err := svc.Get(context.Background(), courseAdmin(), 44); !errors.Is(err, ErrCourseNotFound) {
		t.Fatalf("missing course error = %v", err)
	}
}

func TestUpdateAdminOnlyAndQuotaGuard(t *testing.T) {
	tx := &fakeCourseTx{course: model.Course{ID: 3}, filled: 7}
	repo := &fakeCourseRepository{tx: tx}
	svc := NewCourseService(repo)
	if _, err := svc.Update(context.Background(), courseStudent(), 3, validCourse()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student update error = %v", err)
	}
	tooSmall := validCourse()
	tooSmall.Kuota = 6
	if _, err := svc.Update(context.Background(), courseAdmin(), 3, tooSmall); !errors.Is(err, ErrQuotaConflict) {
		t.Fatalf("quota reduction error = %v", err)
	}
	if !tx.rolledBack || tx.committed {
		t.Fatalf("quota rejection did not rollback: %#v", tx)
	}
	tx.rolledBack = false
	valid := validCourse()
	valid.Kuota = 9
	updated, err := svc.Update(context.Background(), courseAdmin(), 3, valid)
	if err != nil || updated.Terisi != 7 || updated.SisaKuota != 2 || !tx.committed {
		t.Fatalf("valid update=%#v err=%v tx=%#v", updated, err, tx)
	}
}

func TestUpdateValidationAndNotFound(t *testing.T) {
	svc := NewCourseService(&fakeCourseRepository{})
	if _, err := svc.Update(context.Background(), courseAdmin(), 1, model.CourseInput{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid course error = %v", err)
	}
	tx := &fakeCourseTx{findErr: repository.ErrNotFound}
	svc = NewCourseService(&fakeCourseRepository{tx: tx})
	if _, err := svc.Update(context.Background(), courseAdmin(), 100, validCourse()); !errors.Is(err, ErrCourseNotFound) {
		t.Fatalf("missing course update error = %v", err)
	}
}

func TestDeleteCourseRequiresAdminAndNoEnrollments(t *testing.T) {
	tx := &fakeCourseTx{course: model.Course{ID: 3}}
	repo := &fakeCourseRepository{tx: tx}
	svc := NewCourseService(repo)
	if err := svc.Delete(context.Background(), courseStudent(), 3); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student delete error = %v", err)
	}
	tx.filled = 1
	if err := svc.Delete(context.Background(), courseAdmin(), 3); !errors.Is(err, ErrCourseConflict) {
		t.Fatalf("enrolled course delete error = %v", err)
	}
	if tx.deletedID != 0 || !tx.rolledBack {
		t.Fatalf("course with enrollment was deleted: %#v", tx)
	}
	tx.filled, tx.rolledBack = 0, false
	if err := svc.Delete(context.Background(), courseAdmin(), 3); err != nil || tx.deletedID != 3 || !tx.committed {
		t.Fatalf("delete course err=%v tx=%#v", err, tx)
	}
}

func TestCourseValidationRanges(t *testing.T) {
	input := validCourse()
	input.SKS, input.Semester, input.Kuota = 25, 15, -1
	_, err := NewCourseService(&fakeCourseRepository{}).Create(context.Background(), courseAdmin(), input)
	var validationErr *CourseValidationError
	if !errors.As(err, &validationErr) || len(validationErr.Fields) != 3 {
		t.Fatalf("validation error=%#v", err)
	}
}
