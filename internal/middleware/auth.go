package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

// Definisikan key khusus untuk context agar tidak bentrok dengan package lain
type contextKey string

const userContextKey contextKey = "authenticated_user"

const CookieSessionName = "kampustix_session_uid"

// AuthMiddleware mengelola verifikasi session cookie dan proteksi hak akses (RBAC)
type AuthMiddleware struct {
	UserRepo *repository.UserRepo
}

// NewAuthMiddleware membuat instance baru dari AuthMiddleware
func NewAuthMiddleware(userRepo *repository.UserRepo) *AuthMiddleware {
	return &AuthMiddleware{UserRepo: userRepo}
}

// Authenticate adalah middleware global yang memeriksa apakah ada session cookie yang valid.
// Jika ada, data *model.User dimasukkan ke dalam request context.
// Jika tidak ada, pengguna dibiarkan sebagai Guest (user == nil di context).
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieSessionName)
		if err != nil || cookie.Value == "" {
			// Tidak ada cookie, lanjutkan sebagai Guest
			next.ServeHTTP(w, r)
			return
		}

		userID, err := strconv.ParseInt(cookie.Value, 10, 64)
		if err != nil {
			// Cookie tidak valid, hapus cookie dan lanjutkan sebagai Guest
			HapusSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}

		// Ambil data user dari database
		user, err := m.UserRepo.FindByID(r.Context(), userID)
		if err != nil || user == nil {
			// User di database sudah tidak ada / terhapus
			HapusSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}

		// Masukkan pointer *model.User ke dalam context request
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth adalah guard middleware: menolak pengguna yang belum login (Guest)
// dan langsung mengarahkannya ke halaman /login
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole adalah guard middleware: memeriksa apakah user memiliki salah satu peran yang diizinkan
// Jika role tidak cocok, sistem merespons dengan HTTP 403 Forbidden
func (m *AuthMiddleware) RequireRole(roles ...model.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := CurrentUser(r)
			if user == nil {
				http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
				return
			}

			// Periksa apakah role user termasuk dalam daftar role yang diperbolehkan
			diizinkan := false
			for _, rRole := range roles {
				if user.Role == rRole {
					diizinkan = true
					break
				}
			}

			if !diizinkan {
				http.Error(w, "403 Forbidden: Anda tidak memiliki hak akses ke halaman ini", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireGuest memastikan rute hanya bisa diakses oleh pengunjung yang belum login.
// Jika user sudah login dan mencoba buka /login atau /register, otomatis dilempar ke / (katalog).
func (m *AuthMiddleware) RequireGuest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user := CurrentUser(r); user != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CurrentUser adalah fungsi helper publik yang bisa dipanggil oleh anggota tim lain
// di dalam Handler mana pun untuk mendapatkan data pengguna yang sedang aktif.
// Mengembalikan nil jika pengguna adalah Guest.
func CurrentUser(r *http.Request) *model.User {
	val := r.Context().Value(userContextKey)
	if val == nil {
		return nil
	}
	if user, ok := val.(*model.User); ok {
		return user
	}
	return nil
}

// PasangSessionCookie membuat cookie session di browser pengguna saat berhasil login
func PasangSessionCookie(w http.ResponseWriter, userID int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieSessionName,
		Value:    strconv.FormatInt(userID, 10),
		Path:     "/",
		HttpOnly: true,                         // Mencegah cookie dibaca oleh JavaScript jahat (XSS protection)
		SameSite: http.SameSiteLaxMode,         // Mencegah CSRF attack
		Expires:  time.Now().Add(24 * time.Hour), // Berlaku 24 jam
	})
}

// HapusSessionCookie menghapus cookie session saat pengguna logout
func HapusSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieSessionName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1, // Langsung kadaluwarsa seketika
		Expires:  time.Unix(0, 0),
	})
}
