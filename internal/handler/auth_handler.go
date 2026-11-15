package handler

import (
	"net/http"

	"kampustix/internal/middleware"
	"kampustix/internal/model"
	"kampustix/internal/service"
)

// AuthHandler bertugas menangani request HTTP untuk login, registrasi, dan logout
type AuthHandler struct {
	AuthSvc *service.AuthService
	View    *Renderer
}

// HalamanLoginRegister menampilkan halaman formulir auth (bisa tab login atau register)
// Endpoint: GET /login atau GET /register
func (h *AuthHandler) HalamanLoginRegister(w http.ResponseWriter, r *http.Request) {
	// Menangkap query parameter ?pesan=... jika diarahkan dari redirect middleware
	pesanInfo := r.URL.Query().Get("pesan")

	// Tentukan tab mana yang aktif (default: login, atau register jika URL-nya /register)
	tabAktif := "login"
	if r.URL.Path == "/register" {
		tabAktif = "register"
	}

	data := map[string]any{
		"Title":    "Masuk & Daftar Akun - KampusTix",
		"TabAktif": tabAktif,
		"Pesan":    pesanInfo,
		"User":     middleware.CurrentUser(r), // Selalu kirimkan user (nil jika guest)
	}

	h.View.Tampil(w, "auth/login_register.html", data)
}

// ProsesLogin memvalidasi email & password yang dikirim dari form login
// Endpoint: POST /login
func (h *AuthHandler) ProsesLogin(w http.ResponseWriter, r *http.Request) {
	// 1. Parse data dari form HTML
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}

	input := model.LoginInput{
		Email:    r.PostFormValue("email"),
		Password: r.PostFormValue("password"),
	}

	// 2. Serahkan verifikasi ke AuthService (Layer Logika Bisnis)
	user, err := h.AuthSvc.Login(r.Context(), input)
	if err != nil {
		// Jika gagal, kembalikan ke halaman login sambil menampilkan pesan error ke UI
		data := map[string]any{
			"Title":    "Masuk - KampusTix",
			"TabAktif": "login",
			"Error":    err.Error(), // Pesan error dari service (misal: "email atau password salah")
			"Email":    input.Email, // Simpan input email agar user tidak perlu ketik ulang
			"User":     nil,
		}
		h.View.Tampil(w, "auth/login_register.html", data)
		return
	}

	// 3. Jika login sukses, pasang Cookie Session di browser
	middleware.PasangSessionCookie(w, user.ID)

	// 4. Arahkan pengguna ke halaman utama (Katalog Event)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ProsesRegister membuat akun baru dari data form pendaftaran
// Endpoint: POST /register
func (h *AuthHandler) ProsesRegister(w http.ResponseWriter, r *http.Request) {
	// 1. Parse form
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}

	input := model.RegisterInput{
		Name:     r.PostFormValue("name"),
		Email:    r.PostFormValue("email"),
		Password: r.PostFormValue("password"),
	}

	// 2. Kirim ke AuthService untuk validasi, enkripsi bcrypt, dan simpan ke database
	user, err := h.AuthSvc.Register(r.Context(), input)
	if err != nil {
		// Jika gagal (misal: email duplikat atau password < 6 karakter), tampilkan error di UI
		data := map[string]any{
			"Title":    "Daftar Akun - KampusTix",
			"TabAktif": "register",
			"Error":    err.Error(),
			"Name":     input.Name,
			"Email":    input.Email,
			"User":     nil,
		}
		h.View.Tampil(w, "auth/login_register.html", data)
		return
	}

	// 3. Begitu daftar sukses, langsung otomatis login-kan user dengan memasang cookie
	middleware.PasangSessionCookie(w, user.ID)

	// 4. Arahkan ke halaman utama
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ProsesLogout menghapus session cookie pengguna
// Endpoint: POST /logout
func (h *AuthHandler) ProsesLogout(w http.ResponseWriter, r *http.Request) {
	// 1. Hapus cookie dari browser (MaxAge = -1)
	middleware.HapusSessionCookie(w)

	// 2. Redirect ke halaman utama / katalog
	http.Redirect(w, r, "/login?pesan=Anda+telah+berhasil+keluar", http.StatusSeeOther)
}
