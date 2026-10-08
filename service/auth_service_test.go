package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"latihan-fiber/UTS/helper"
	"latihan-fiber/UTS/model"
)

type fakeAuthRepository struct {
	user    model.User
	student model.Student
	userErr error
	studErr error
}

func (f fakeAuthRepository) FindUserByEmail(context.Context, string) (model.User, error) {
	return f.user, f.userErr
}
func (f fakeAuthRepository) FindUserByID(context.Context, int64) (model.User, error) {
	return f.user, f.userErr
}
func (f fakeAuthRepository) FindStudentByUserID(context.Context, int64) (model.Student, error) {
	return f.student, f.studErr
}

func TestLoginIssuesTokenForValidAdmin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager, _ := helper.NewJWTManager("01234567890123456789012345678901", time.Minute)
	svc := NewAuthService(fakeAuthRepository{user: model.User{ID: 7, Email: "admin@example.com", Password: string(hash), Role: "admin"}}, manager)
	result, err := svc.Login(context.Background(), "admin@example.com", "admin-password")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken == "" || result.TokenType != "Bearer" || result.User.Role != "admin" {
		t.Fatalf("unexpected login result: %#v", result)
	}
	if _, err := manager.Parse(result.AccessToken); err != nil {
		t.Fatalf("generated token did not parse: %v", err)
	}
}

func TestLoginRejectsWrongPasswordAndDeletedStudent(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("student-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	manager, _ := helper.NewJWTManager("01234567890123456789012345678901", time.Minute)
	activeRepo := fakeAuthRepository{
		user:    model.User{ID: 8, Email: "student@example.com", Password: string(hash), Role: "mahasiswa"},
		student: model.Student{ID: 2, UserID: 8},
	}
	svc := NewAuthService(activeRepo, manager)
	if _, err := svc.Login(context.Background(), "student@example.com", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v", err)
	}

	deletedAt := time.Now()
	activeRepo.student.DeletedAt = &deletedAt
	svc = NewAuthService(activeRepo, manager)
	if _, err := svc.Login(context.Background(), "student@example.com", "student-pass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("deleted student error = %v", err)
	}
}

func TestAuthenticateRejectsSoftDeletedStudent(t *testing.T) {
	deletedAt := time.Now()
	repo := fakeAuthRepository{
		user:    model.User{ID: 8, Email: "student@example.com", Role: "mahasiswa"},
		student: model.Student{ID: 2, UserID: 8, DeletedAt: &deletedAt},
	}
	manager, _ := helper.NewJWTManager("01234567890123456789012345678901", time.Minute)
	svc := NewAuthService(repo, manager)
	_, err := svc.Authenticate(context.Background(), model.AuthIdentity{UserID: 8, Email: "student@example.com", Role: "mahasiswa"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Authenticate error = %v, want unauthorized", err)
	}
}

func TestLoginPropagatesRepositoryFailures(t *testing.T) {
	manager, _ := helper.NewJWTManager("01234567890123456789012345678901", time.Minute)
	want := errors.New("database unavailable")
	svc := NewAuthService(fakeAuthRepository{userErr: want}, manager)
	if _, err := svc.Login(context.Background(), "user@example.com", "password"); !errors.Is(err, want) {
		t.Fatalf("Login error = %v", err)
	}
}
