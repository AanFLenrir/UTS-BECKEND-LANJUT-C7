package model

import "time"

type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type AuthIdentity struct {
	UserID int64  `json:"id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int64        `json:"expires_in"`
	User        AuthIdentity `json:"user"`
}

type MeResponse struct {
	ID      int64    `json:"id"`
	Email   string   `json:"email"`
	Role    string   `json:"role"`
	Student *Student `json:"student,omitempty"`
}

type Student struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	NIM         string     `json:"nim"`
	Nama        string     `json:"nama"`
	Prodi       string     `json:"prodi"`
	Angkatan    int        `json:"angkatan"`
	IPKTerakhir float64    `json:"ipk_terakhir"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type StudentListItem struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"user_id"`
	Email       string  `json:"email"`
	NIM         string  `json:"nim"`
	Nama        string  `json:"nama"`
	Prodi       string  `json:"prodi"`
	Angkatan    int     `json:"angkatan"`
	IPKTerakhir float64 `json:"ipk_terakhir"`
}

type StudentCourse struct {
	ID            int64  `json:"id"`
	KodeMK        string `json:"kode_mk"`
	NamaMK        string `json:"nama_mk"`
	SKS           int    `json:"sks"`
	Semester      int    `json:"semester"`
	TahunAkademik string `json:"tahun_akademik"`
}

type StudentDetail struct {
	ID            int64           `json:"id"`
	UserID        int64           `json:"user_id"`
	Email         string          `json:"email"`
	NIM           string          `json:"nim"`
	Nama          string          `json:"nama"`
	Prodi         string          `json:"prodi"`
	Angkatan      int             `json:"angkatan"`
	IPKTerakhir   float64         `json:"ipk_terakhir"`
	Courses       []StudentCourse `json:"mata_kuliah"`
	TahunAkademik string          `json:"tahun_akademik,omitempty"`
	TotalSKS      int             `json:"total_sks"`
	BatasSKS      int             `json:"batas_sks"`
}

type StudentListQuery struct {
	Page      int
	Limit     int
	Offset    int
	Prodi     string
	Angkatan  *int
	Search    string
	Sort      string
	SortOrder string
}

type CreateStudentRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"-"` // public creation always uses NIM as the initial password
	NIM         string  `json:"nim"`
	Nama        string  `json:"nama"`
	Prodi       string  `json:"prodi"`
	Angkatan    int     `json:"angkatan"`
	IPKTerakhir float64 `json:"ipk_terakhir"`
}

type UpdateStudentRequest struct {
	Email       string  `json:"-"` // legacy internal field; excluded from the public request contract
	NIM         *string `json:"nim"`
	Nama        string  `json:"nama"`
	Prodi       string  `json:"prodi"`
	Angkatan    int     `json:"angkatan"`
	IPKTerakhir float64 `json:"ipk_terakhir"`
}

type Course struct {
	ID        int64  `json:"id"`
	KodeMK    string `json:"kode_mk"`
	NamaMK    string `json:"nama_mk"`
	SKS       int    `json:"sks"`
	Semester  int    `json:"semester"`
	Kuota     int    `json:"kuota"`
	Terisi    int64  `json:"terisi"`
	SisaKuota int64  `json:"sisa_kuota"`
}

type CourseInput struct {
	KodeMK   string `json:"kode_mk"`
	NamaMK   string `json:"nama_mk"`
	SKS      int    `json:"sks"`
	Semester int    `json:"semester"`
	Kuota    int    `json:"kuota"`
}

type CourseListQuery struct {
	Page          int
	Limit         int
	Offset        int
	Semester      *int
	Search        string
	Sort          string
	SortOrder     string
	AvailableOnly bool
}

type Enrollment struct {
	ID            int64     `json:"id"`
	StudentID     int64     `json:"student_id"`
	CourseID      int64     `json:"course_id"`
	TahunAkademik string    `json:"tahun_akademik"`
	CreatedAt     time.Time `json:"created_at"`
}

type EnrollmentRequest struct {
	CourseID      int64  `json:"course_id"`
	TahunAkademik string `json:"tahun_akademik"`
}

type CourseSummary struct {
	ID       int64  `json:"id"`
	KodeMK   string `json:"kode_mk"`
	NamaMK   string `json:"nama_mk"`
	SKS      int    `json:"sks"`
	Semester int    `json:"semester"`
}

type EnrollmentView struct {
	ID            int64         `json:"id"`
	Course        CourseSummary `json:"course"`
	TahunAkademik string        `json:"tahun_akademik"`
	CreatedAt     time.Time     `json:"created_at"`
}

type StudentEnrollmentSummary struct {
	Student struct {
		ID   int64  `json:"id"`
		NIM  string `json:"nim"`
		Nama string `json:"nama"`
	} `json:"student"`
	Enrollments   []EnrollmentView `json:"enrollments"`
	TahunAkademik string           `json:"tahun_akademik,omitempty"`
	TotalSKS      int              `json:"total_sks"`
	BatasSKS      int              `json:"batas_sks"`
}

type EnrollmentRecord struct {
	Enrollment Enrollment
	Course     CourseSummary
	UserID     int64
}
