package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"latihan-fiber/UTS/helper"
	"latihan-fiber/UTS/model"
	"latihan-fiber/UTS/repository"
)

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrUnauthorized       = errors.New("akun tidak dapat diautentikasi")
)

type AuthService struct {
	repo repository.AuthRepository
	jwt  *helper.JWTManager
}

func NewAuthService(repo repository.AuthRepository, jwtManager *helper.JWTManager) *AuthService {
	return &AuthService{repo: repo, jwt: jwtManager}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (model.LoginResponse, error) {
	user, err := s.repo.FindUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.LoginResponse{}, ErrInvalidCredentials
		}
		return model.LoginResponse{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil || !helper.ValidRole(user.Role) {
		return model.LoginResponse{}, ErrInvalidCredentials
	}
	identity := model.AuthIdentity{UserID: user.ID, Email: user.Email, Role: user.Role}
	if user.Role == "mahasiswa" {
		student, err := s.repo.FindStudentByUserID(ctx, user.ID)
		if errors.Is(err, repository.ErrNotFound) || student.DeletedAt != nil {
			return model.LoginResponse{}, ErrInvalidCredentials
		}
		if err != nil {
			return model.LoginResponse{}, err
		}
	}
	token, err := s.jwt.Generate(identity)
	if err != nil {
		return model.LoginResponse{}, err
	}
	return model.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.jwt.TTL().Seconds()),
		User:        identity,
	}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, claims model.AuthIdentity) (model.AuthIdentity, error) {
	user, err := s.repo.FindUserByID(ctx, claims.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		return model.AuthIdentity{}, ErrUnauthorized
	}
	if err != nil {
		return model.AuthIdentity{}, err
	}
	if user.Email != claims.Email || user.Role != claims.Role || !helper.ValidRole(user.Role) {
		return model.AuthIdentity{}, ErrUnauthorized
	}
	if user.Role == "mahasiswa" {
		student, err := s.repo.FindStudentByUserID(ctx, user.ID)
		if errors.Is(err, repository.ErrNotFound) || student.DeletedAt != nil {
			return model.AuthIdentity{}, ErrUnauthorized
		}
		if err != nil {
			return model.AuthIdentity{}, err
		}
	}
	return model.AuthIdentity{UserID: user.ID, Email: user.Email, Role: user.Role}, nil
}

func (s *AuthService) Me(ctx context.Context, identity model.AuthIdentity) (model.MeResponse, error) {
	result := model.MeResponse{ID: identity.UserID, Email: identity.Email, Role: identity.Role}
	if identity.Role == "mahasiswa" {
		student, err := s.repo.FindStudentByUserID(ctx, identity.UserID)
		if errors.Is(err, repository.ErrNotFound) || student.DeletedAt != nil {
			return model.MeResponse{}, ErrUnauthorized
		}
		if err != nil {
			return model.MeResponse{}, err
		}
		result.Student = &student
	}
	return result, nil
}

func (s *AuthService) TokenIdentity(token string) (model.AuthIdentity, error) {
	return s.jwt.Parse(token)
}

func (s *AuthService) TokenTTL() time.Duration { return s.jwt.TTL() }
