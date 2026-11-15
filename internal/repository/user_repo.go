package repository

import (
	"context"
	"database/sql"
	"errors"

	"kampustix/internal/model"
)

// UserRepo mengelola operasi database untuk entitas User
type UserRepo struct {
	DB *sql.DB
}

// Create menyimpan user baru ke database saat registrasi
func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	query := `INSERT INTO users (name, email, password_hash, role) VALUES (?, ?, ?, ?)`
	res, err := r.DB.ExecContext(ctx, query, u.Name, u.Email, u.PasswordHash, u.Role)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	return nil
}

// FindByEmail mencari user berdasarkan alamat email (digunakan saat login)
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, name, email, password_hash, role, created_at FROM users WHERE email = ? LIMIT 1`
	row := r.DB.QueryRowContext(ctx, query, email)

	var u model.User
	var roleStr string
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // user tidak ditemukan (bukan error fatal)
		}
		return nil, err
	}
	u.Role = model.UserRole(roleStr)
	return &u, nil
}

// FindByID mencari user berdasarkan ID (digunakan saat validasi session cookie di middleware)
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	query := `SELECT id, name, email, password_hash, role, created_at FROM users WHERE id = ? LIMIT 1`
	row := r.DB.QueryRowContext(ctx, query, id)

	var u model.User
	var roleStr string
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &roleStr, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	u.Role = model.UserRole(roleStr)
	return &u, nil
}
