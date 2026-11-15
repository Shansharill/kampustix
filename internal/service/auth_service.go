package service

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

var (
	ErrEmailSudahTerdaftar = errors.New("email sudah terdaftar, silakan gunakan email lain atau login")
	ErrKredensialSalah     = errors.New("email atau password salah")
	ErrInputTidakValid     = errors.New("nama, email, dan password wajib diisi")
	ErrPasswordTerlaluPendek = errors.New("password minimal harus terdiri dari 6 karakter")
)

// AuthService menangani logika bisnis autentikasi, registrasi, dan verifikasi password
type AuthService struct {
	UserRepo *repository.UserRepo
}

// Register mendaftarkan akun baru ke sistem dengan role default BUYER
func (s *AuthService) Register(ctx context.Context, input model.RegisterInput) (*model.User, error) {
	name := strings.TrimSpace(input.Name)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := input.Password

	// 1. Validasi input dasar
	if name == "" || email == "" || password == "" {
		return nil, ErrInputTidakValid
	}
	if len(password) < 6 {
		return nil, ErrPasswordTerlaluPendek
	}

	// 2. Cek apakah email sudah terdaftar
	existing, err := s.UserRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailSudahTerdaftar
	}

	// 3. Enkripsi (hash) password dengan bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 4. Siapkan objek user (default role: BUYER)
	userBaru := &model.User{
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
		Role:         model.RoleBuyer,
	}

	// 5. Simpan ke database melalui repository
	if err := s.UserRepo.Create(ctx, userBaru); err != nil {
		return nil, err
	}

	return userBaru, nil
}

// Login memverifikasi email dan password pengguna
func (s *AuthService) Login(ctx context.Context, input model.LoginInput) (*model.User, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := input.Password

	if email == "" || password == "" {
		return nil, ErrInputTidakValid
	}

	// 1. Ambil data user dari database berdasarkan email
	user, err := s.UserRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrKredensialSalah
	}

	// 2. Bandingkan password yang diketik user dengan password hash di database
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, ErrKredensialSalah
	}

	return user, nil
}
