package service

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

type fakeStudentRepository struct {
	count      int64
	items      []model.StudentListItem
	profile    model.Student
	email      string
	courses    []model.StudentCourse
	findErr    error
	countErr   error
	listErr    error
	coursesErr error
	deleteErr  error
	deletedID  int64
	tx         *fakeStudentTx
	query      model.StudentListQuery
}

func (f *fakeStudentRepository) CountStudents(_ context.Context, q model.StudentListQuery) (int64, error) {
	f.query = q
	return f.count, f.countErr
}
func (f *fakeStudentRepository) ListStudents(_ context.Context, q model.StudentListQuery) ([]model.StudentListItem, error) {
	f.query = q
	return f.items, f.listErr
}
func (f *fakeStudentRepository) FindStudentByID(context.Context, int64) (model.Student, string, error) {
	return f.profile, f.email, f.findErr
}
func (f *fakeStudentRepository) FindStudentCourses(context.Context, int64) ([]model.StudentCourse, error) {
	return f.courses, f.coursesErr
}
func (f *fakeStudentRepository) BeginTx(context.Context) (repository.StudentTx, error) {
	return f.tx, nil
}
func (f *fakeStudentRepository) SoftDeleteStudent(_ context.Context, id int64) error {
	f.deletedID = id
	return f.deleteErr
}

type fakeStudentTx struct {
	userID         int64
	passwordHash   string
	createUserErr  error
	createStudErr  error
	updateEmailErr error
	updateStudErr  error
	student        model.Student
	userEmail      string
	role           string
	committed      bool
	rolledBack     bool
}

func (f *fakeStudentTx) CreateUser(_ context.Context, email, hash, role string) (int64, error) {
	f.userEmail, f.role = email, role
	f.passwordHash = hash
	return f.userID, f.createUserErr
}
func (f *fakeStudentTx) CreateStudent(_ context.Context, student model.Student) (model.Student, error) {
	f.student = student
	student.ID = 55
	return student, f.createStudErr
}
func (f *fakeStudentTx) UpdateUserEmail(_ context.Context, _ int64, email string) error {
	f.userEmail = email
	return f.updateEmailErr
}
func (f *fakeStudentTx) UpdateStudent(_ context.Context, id int64, student model.Student) (model.Student, error) {
	student.ID = id
	return student, f.updateStudErr
}
func (f *fakeStudentTx) Commit(context.Context) error   { f.committed = true; return nil }
func (f *fakeStudentTx) Rollback(context.Context) error { f.rolledBack = true; return nil }

func adminIdentity() model.AuthIdentity {
	return model.AuthIdentity{UserID: 1, Email: "admin@example.com", Role: "admin"}
}
func studentIdentity() model.AuthIdentity {
	return model.AuthIdentity{UserID: 7, Email: "student@example.com", Role: "mahasiswa"}
}

func validCreateStudent() model.CreateStudentRequest {
	return model.CreateStudentRequest{Email: "new@example.com", Password: "strong-pass-1", NIM: "231524000099", Nama: "Nama Baru", Prodi: "Teknik Informatika", Angkatan: 2024, IPKTerakhir: 3.5}
}

func TestStudentListRequiresAdminAndUsesPaginationFiltersAndSafeSort(t *testing.T) {
	repo := &fakeStudentRepository{count: 21, items: []model.StudentListItem{{ID: 1}}}
	svc := NewStudentService(repo)
	if _, err := svc.List(context.Background(), studentIdentity(), model.StudentListQuery{Page: 1, Limit: 10}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student list error = %v", err)
	}
	angkatan := 2024
	got, err := svc.List(context.Background(), adminIdentity(), model.StudentListQuery{
		Page: 3, Limit: 10, Prodi: "Teknik Informatika", Angkatan: &angkatan,
		Search: "231", Sort: "ipk_terakhir", SortOrder: "desc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalPages != 3 || got.Page != 3 || repo.query.Offset != 20 || repo.query.Prodi != "Teknik Informatika" || repo.query.Angkatan == nil || repo.query.Search != "231" || repo.query.Sort != "ipk_terakhir" || repo.query.SortOrder != "desc" {
		t.Fatalf("query/page not applied: result=%#v query=%#v", got, repo.query)
	}
	if _, err := svc.List(context.Background(), adminIdentity(), model.StudentListQuery{Page: 1, Limit: 10, Sort: "id; DROP TABLE students"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("unsafe sort error = %v", err)
	}
}

func TestCreateStudentCommitsHashAndStudentTogether(t *testing.T) {
	tx := &fakeStudentTx{userID: 99}
	svc := NewStudentService(&fakeStudentRepository{tx: tx})
	created, err := svc.Create(context.Background(), adminIdentity(), validCreateStudent())
	if err != nil {
		t.Fatal(err)
	}
	if !tx.committed || tx.role != "mahasiswa" || created.UserID != 99 || created.ID != 55 {
		t.Fatalf("unexpected transaction result: tx=%#v student=%#v", tx, created)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(tx.passwordHash), []byte("231524000099")); err != nil {
		t.Fatal("password was not stored as bcrypt hash")
	}
}

func TestCreateDuplicateEmailOrNIMReturnsConflict(t *testing.T) {
	for _, duplicate := range []error{repository.ErrDuplicateEmail, repository.ErrDuplicateNIM} {
		tx := &fakeStudentTx{userID: 5}
		if errors.Is(duplicate, repository.ErrDuplicateEmail) {
			tx.createUserErr = duplicate
		} else {
			tx.createStudErr = duplicate
		}
		svc := NewStudentService(&fakeStudentRepository{tx: tx})
		if _, err := svc.Create(context.Background(), adminIdentity(), validCreateStudent()); !errors.Is(err, ErrConflict) {
			t.Fatalf("duplicate %v error = %v", duplicate, err)
		}
		if !tx.rolledBack || tx.committed {
			t.Fatalf("duplicate did not rollback: %#v", tx)
		}
	}
}

func TestCreateStudentInsertFailureRollsBackUser(t *testing.T) {
	tx := &fakeStudentTx{userID: 88, createStudErr: errors.New("student insert failed")}
	svc := NewStudentService(&fakeStudentRepository{tx: tx})
	if _, err := svc.Create(context.Background(), adminIdentity(), validCreateStudent()); err == nil {
		t.Fatal("expected insert error")
	}
	if !tx.rolledBack || tx.committed {
		t.Fatalf("failed student insert did not rollback: %#v", tx)
	}
}

func TestStudentDetailOwnershipAndSKS(t *testing.T) {
	repo := &fakeStudentRepository{
		profile: model.Student{ID: 4, UserID: 7, NIM: "231524007", IPKTerakhir: 2.5},
		email:   "student@example.com",
		courses: []model.StudentCourse{{ID: 1, SKS: 3}, {ID: 2, SKS: 2}},
	}
	svc := NewStudentService(repo)
	if _, err := svc.Get(context.Background(), adminIdentity(), 4); err != nil {
		t.Fatalf("admin detail: %v", err)
	}
	got, err := svc.Get(context.Background(), studentIdentity(), 4)
	if err != nil || got.TotalSKS != 5 || got.BatasSKS != 21 || got.Email != "student@example.com" {
		t.Fatalf("own detail = %#v, %v", got, err)
	}
	if _, err := svc.Get(context.Background(), model.AuthIdentity{UserID: 8, Role: "mahasiswa"}, 4); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other student's detail error = %v", err)
	}
}

func TestUpdateRejectsNIMAndDuplicateEmail(t *testing.T) {
	svc := NewStudentService(&fakeStudentRepository{})
	nim := "999"
	if _, err := svc.Update(context.Background(), adminIdentity(), 4, model.UpdateStudentRequest{NIM: &nim}); !errors.Is(err, ErrValidation) {
		t.Fatalf("NIM update error = %v", err)
	}
	tx := &fakeStudentTx{updateEmailErr: repository.ErrDuplicateEmail}
	repo := &fakeStudentRepository{profile: model.Student{ID: 4, UserID: 7}, tx: tx}
	svc = NewStudentService(repo)
	req := model.UpdateStudentRequest{Email: "other@example.com", Nama: "Nama", Prodi: "Prodi", Angkatan: 2024, IPKTerakhir: 3}
	if _, err := svc.Update(context.Background(), adminIdentity(), 4, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email error = %v", err)
	}
	if !tx.rolledBack {
		t.Fatal("email conflict did not rollback transaction")
	}
}

func TestSoftDeleteCallsRepository(t *testing.T) {
	repo := &fakeStudentRepository{}
	if err := NewStudentService(repo).Delete(context.Background(), adminIdentity(), 43); err != nil {
		t.Fatal(err)
	}
	if repo.deletedID != 43 {
		t.Fatalf("soft delete ID = %d", repo.deletedID)
	}
	if err := NewStudentService(repo).Delete(context.Background(), studentIdentity(), 43); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student delete error = %v", err)
	}
}

func TestSKSLimitBoundariesAndCreateValidation(t *testing.T) {
	for _, tc := range []struct {
		ipk  float64
		want int
	}{{3, 24}, {2.99, 21}, {2.5, 21}, {2.49, 18}} {
		if got := SKSLimit(tc.ipk); got != tc.want {
			t.Errorf("SKSLimit(%v)=%d, want %d", tc.ipk, got, tc.want)
		}
	}
	invalid := validCreateStudent()
	invalid.Email = "not-email"
	invalid.Password = "short"
	invalid.NIM = ""
	invalid.Angkatan = 0
	invalid.IPKTerakhir = 4.1
	_, err := NewStudentService(&fakeStudentRepository{}).Create(context.Background(), adminIdentity(), invalid)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || len(validationErr.Fields) < 5 {
		t.Fatalf("validation error = %#v", err)
	}
}
