package handler

import (
	"errors"
	"net/http"

	"kampustix/internal/middleware"
	"kampustix/internal/service"
)

type GatekeeperHandler struct {
	GatekeeperSvc *service.GatekeeperService
	View          *Renderer
}

// ScanPage menampilkan halaman scanner tiket.
func (h *GatekeeperHandler) ScanPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)

	data := map[string]any{
		"Title": "Gatekeeper Scanner - KampusTix",
		"User":  user,
	}

	h.View.Tampil(w, "gatekeeper/scan.html", data)
}

// ValidateTicket memproses validasi dan check-in tiket.
func (h *GatekeeperHandler) ValidateTicket(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}

	code := r.PostFormValue("ticket_code")

	if code == "" {
		h.View.Tampil(w, "gatekeeper/scan.html", map[string]any{
			"Title": "Gatekeeper Scanner - KampusTix",
			"User":  user,
			"Error": "Kode tiket wajib diisi.",
		})
		return
	}

	ticket, err := h.GatekeeperSvc.ValidateAndCheckIn(
		r.Context(),
		code,
		user.ID,
	)

	if errors.Is(err, service.ErrTicketNotFound) {
		h.View.Tampil(w, "gatekeeper/scan.html", map[string]any{
			"Title": "Gatekeeper Scanner - KampusTix",
			"User":  user,
			"Error": "Tiket tidak ditemukan.",
		})
		return
	}

	if errors.Is(err, service.ErrTicketNotActive) {
		h.View.Tampil(w, "gatekeeper/scan.html", map[string]any{
			"Title":  "Gatekeeper Scanner - KampusTix",
			"User":   user,
			"Ticket": ticket,
			"Error":  "Tiket tidak valid atau sudah pernah digunakan.",
		})
		return
	}

	if err != nil {
		http.Error(
			w,
			"Terjadi kesalahan saat memvalidasi tiket.",
			http.StatusInternalServerError,
		)
		return
	}

	h.View.Tampil(w, "gatekeeper/scan.html", map[string]any{
		"Title":   "Gatekeeper Scanner - KampusTix",
		"User":    user,
		"Ticket":  ticket,
		"Success": "Check-in berhasil! Selamat datang.",
	})
}

// PrintTicket menampilkan halaman boarding pass untuk dicetak.
func (h *GatekeeperHandler) PrintTicket(
	w http.ResponseWriter,
	r *http.Request,
) {
	user := middleware.CurrentUser(r)

	// Mengambil kode tiket dari URL.
	code := r.PathValue("code")

	if code == "" {
		http.Error(w, "Kode tiket wajib diisi.", http.StatusBadRequest)
		return
	}

	// Meminta Service mengambil informasi boarding pass.
	boardingPass, err := h.GatekeeperSvc.GetBoardingPass(
		r.Context(),
		code,
	)

	if errors.Is(err, service.ErrTicketNotFound) {
		http.Error(w, "Tiket tidak ditemukan.", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(
			w,
			"Terjadi kesalahan saat mengambil data tiket.",
			http.StatusInternalServerError,
		)
		return
	}

	// Mengirim data ke template print.html.
	data := map[string]any{
		"Title":        "Boarding Pass - KampusTix",
		"User":         user,
		"BoardingPass": boardingPass,
	}

	h.View.Tampil(w, "gatekeeper/print.html", data)
}
