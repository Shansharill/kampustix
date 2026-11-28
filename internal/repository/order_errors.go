package repository

import "errors"

// Sentinel error untuk kebutuhan validasi akses dan data order.
var (
	ErrForbidden     = errors.New("akses order ditolak")
	ErrOrderNotFound = errors.New("pesanan tidak ditemukan")
)

// BusinessError digunakan untuk kesalahan aturan bisnis
// yang aman ditampilkan kepada pengguna.
type BusinessError struct {
	Message string
}

// Error mengembalikan pesan kesalahan bisnis.
func (e *BusinessError) Error() string {
	if e == nil {
		return "kesalahan aturan bisnis"
	}

	return e.Message
}

// NewBusinessError membuat error baru untuk validasi bisnis.
func NewBusinessError(message string) error {
	return &BusinessError{
		Message: message,
	}
}
