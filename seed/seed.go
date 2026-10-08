package seed

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type studentSeed struct {
	nim, nama, prodi string
	angkatan         int
	ipk              float64
}

var students = []studentSeed{
	{"231524001", "Andi Pratama", "Teknik Informatika", 2023, 3.45},
	{"231524002", "Budi Santoso", "Teknik Informatika", 2023, 3.10},
	{"231524003", "Citra Lestari", "Teknik Informatika", 2023, 3.72},
	{"231524004", "Dewi Anggraini", "Sistem Informasi", 2023, 3.20},
	{"231524005", "Eko Saputra", "Sistem Informasi", 2023, 2.85},
	{"231524006", "Fitri Handayani", "Sistem Informasi", 2023, 3.88},
	{"231524007", "Gilang Ramadhan", "Teknik Informatika", 2023, 2.40},
	{"231524008", "Hana Putri", "Manajemen Informatika", 2023, 3.05},
	{"231524009", "Indra Wijaya", "Manajemen Informatika", 2023, 3.31},
	{"231524010", "Jihan Amalia", "Teknik Informatika", 2023, 3.60},
	{"231524011", "Kevin Maulana", "Sistem Informasi", 2023, 2.75},
	{"231524012", "Laila Nuraini", "Manajemen Informatika", 2023, 3.95},
	{"231524013", "Miko Firmansyah", "Teknik Informatika", 2023, 2.30},
	{"231524014", "Nadia Kusuma", "Sistem Informasi", 2023, 3.40},
	{"231524015", "Oscar Nugraha", "Manajemen Informatika", 2023, 3.15},
	{"231524016", "Putri Maharani", "Teknik Informatika", 2023, 3.82},
	{"231524017", "Rafi Hidayat", "Sistem Informasi", 2023, 2.95},
	{"231524018", "Salsa Nabila", "Manajemen Informatika", 2023, 3.50},
	{"231524019", "Tegar Kurniawan", "Teknik Informatika", 2023, 2.20},
	{"231524020", "Ulya Safitri", "Sistem Informasi", 2023, 3.68},
}

var courses = []struct {
	kode, nama           string
	sks, semester, kuota int
}{
	{"IF101", "Algoritma dan Pemrograman", 3, 1, 40},
	{"IF102", "Struktur Data", 3, 2, 35},
	{"IF201", "Basis Data", 3, 3, 40},
	{"IF202", "Pemrograman Web", 3, 4, 35},
	{"IF301", "Rekayasa Perangkat Lunak", 3, 5, 30},
	{"IF302", "Jaringan Komputer", 3, 5, 30},
	{"SI201", "Analisis Proses Bisnis", 3, 3, 35},
	{"SI301", "Sistem Informasi Manajemen", 3, 5, 30},
	{"UM101", "Pendidikan Pancasila", 2, 1, 50},
	{"UM102", "Bahasa Indonesia", 2, 2, 50},
}

func Run(ctx context.Context, pool *pgxpool.Pool) error {
	email := strings.TrimSpace(os.Getenv("SEED_ADMIN_EMAIL"))
	password := os.Getenv("SEED_ADMIN_PASSWORD")
	if email == "" || len(password) < 8 {
		return fmt.Errorf("SEED_ADMIN_EMAIL wajib diisi dan SEED_ADMIN_PASSWORD minimal 8 karakter")
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("memulai transaksi seeder: %w", err)
	}
	defer tx.Rollback(ctx)

	adminHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password admin: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users(email,password,role) VALUES($1,$2,'admin') ON CONFLICT(email) DO NOTHING`, email, string(adminHash)); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	for _, st := range students {
		hash, err := bcrypt.GenerateFromPassword([]byte(st.nim), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password mahasiswa %s: %w", st.nim, err)
		}
		var userID int64
		err = tx.QueryRow(ctx, `INSERT INTO users(email,password,role) VALUES($1,$2,'mahasiswa') ON CONFLICT(email) DO UPDATE SET email=EXCLUDED.email RETURNING id`, st.nim+"@student.siakad.local", string(hash)).Scan(&userID)
		if err != nil {
			return fmt.Errorf("seed user %s: %w", st.nim, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO students(user_id,nim,nama,prodi,angkatan,ipk_terakhir) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(nim) DO NOTHING`, userID, st.nim, st.nama, st.prodi, st.angkatan, st.ipk); err != nil {
			return fmt.Errorf("seed student %s: %w", st.nim, err)
		}
	}

	for _, course := range courses {
		if _, err := tx.Exec(ctx, `INSERT INTO courses(kode_mk,nama_mk,sks,semester,kuota) VALUES($1,$2,$3,$4,$5) ON CONFLICT(kode_mk) DO NOTHING`, course.kode, course.nama, course.sks, course.semester, course.kuota); err != nil {
			return fmt.Errorf("seed course %s: %w", course.kode, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seeder: %w", err)
	}
	return nil
}
