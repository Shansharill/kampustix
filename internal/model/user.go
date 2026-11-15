package model

import "time"

// UserRole mendefinisikan tipe peran pengguna sesuai RBAC pada PRD
type UserRole string

const (
	RoleBuyer      UserRole = "BUYER"
	RoleAdmin      UserRole = "ADMIN"
	RoleGatekeeper UserRole = "GATEKEEPER"
)

// User merepresentasikan satu baris data pada tabel `users` di database MySQL
type User struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // jangan pernah diekspos ke respon JSON / view
	Role         UserRole  `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// RegisterInput menampung data yang dikirim dari form registrasi pengguna baru
type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

// LoginInput menampung data yang dikirim dari form login pengguna
type LoginInput struct {
	Email    string
	Password string
}
