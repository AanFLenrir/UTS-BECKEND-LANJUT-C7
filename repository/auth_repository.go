package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"latihan-fiber/UTS/model"
)

var ErrNotFound = errors.New("record tidak ditemukan")

type AuthRepository interface {
	FindUserByEmail(context.Context, string) (model.User, error)
	FindUserByID(context.Context, int64) (model.User, error)
	FindStudentByUserID(context.Context, int64) (model.Student, error)
}

type postgresAuthRepository struct {
	pool *pgxpool.Pool
}

func NewAuthRepository(pool *pgxpool.Pool) AuthRepository {
	return &postgresAuthRepository{pool: pool}
}

func (r *postgresAuthRepository) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	return r.findUser(ctx, `SELECT id,email,password,role,created_at FROM users WHERE LOWER(email)=LOWER($1)`, email)
}

func (r *postgresAuthRepository) FindUserByID(ctx context.Context, id int64) (model.User, error) {
	return r.findUser(ctx, `SELECT id,email,password,role,created_at FROM users WHERE id=$1`, id)
}

func (r *postgresAuthRepository) findUser(ctx context.Context, query string, arg any) (model.User, error) {
	var user model.User
	err := r.pool.QueryRow(ctx, query, arg).Scan(&user.ID, &user.Email, &user.Password, &user.Role, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("query user: %w", err)
	}
	return user, nil
}

func (r *postgresAuthRepository) FindStudentByUserID(ctx context.Context, userID int64) (model.Student, error) {
	var student model.Student
	err := r.pool.QueryRow(ctx, `
		SELECT id,user_id,nim,nama,prodi,angkatan,ipk_terakhir,deleted_at
		FROM students WHERE user_id=$1`, userID,
	).Scan(&student.ID, &student.UserID, &student.NIM, &student.Nama, &student.Prodi, &student.Angkatan, &student.IPKTerakhir, &student.DeletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Student{}, ErrNotFound
	}
	if err != nil {
		return model.Student{}, fmt.Errorf("query student: %w", err)
	}
	return student, nil
}
