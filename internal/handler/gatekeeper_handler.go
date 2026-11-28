package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"kampustix/internal/middleware"
	"kampustix/internal/service"
)

type GatekeeperHandler struct {
	GatekeeperSvc *service.GatekeeperService
	View          *Renderer
}

// scanPageData menyiapkan data untuk halaman scanner.
func (h *GatekeeperHandler) scanPageData(
	r *http.Request,
	extra map[string]any,
) map[string]any {

	user := middleware.CurrentUser(r)

	events, err := h.GatekeeperSvc.GetActiveEvents(r.Context())
	if err != nil {
		log.Printf("Gagal mengambil daftar event aktif: %v", err)
		events = nil
	}

	data := map[string]any{
		"Title":  "Gatekeeper Scanner - KampusTix",
		"User":   user,
		"Events": events,
	}

	for key, value := range extra {
		data[key] = value
	}

	return data
}

// ScanPage menampilkan halaman scanner tiket.
func (h *GatekeeperHandler) ScanPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)

	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data := h.scanPageData(r, nil)

	h.View.Tampil(w, "gatekeeper/scan.html", data)
}

// ValidateTicket memproses validasi dan check-in tiket.
// ValidateTicket memproses validasi dan check-in tiket.
func (h *GatekeeperHandler) ValidateTicket(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)

	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}

	code := strings.TrimSpace(r.PostFormValue("ticket_code"))

	if code == "" {
		h.View.Tampil(w, "gatekeeper/scan.html", h.scanPageData(r, map[string]any{
			"Error": "Kode tiket wajib diisi.",
		}))
		return
	}

	eventID, err := strconv.ParseInt(
		r.PostFormValue("event_id"),
		10,
		64,
	)

	if err != nil || eventID <= 0 {
		h.View.Tampil(w, "gatekeeper/scan.html", h.scanPageData(r, map[string]any{
			"Error": "Silakan pilih event terlebih dahulu.",
		}))
		return
	}

	ticket, err := h.GatekeeperSvc.ValidateAndCheckIn(
		r.Context(),
		code,
		eventID,
		user.ID,
	)

	if errors.Is(err, service.ErrTicketNotFound) {
		h.View.Tampil(w, "gatekeeper/scan.html", h.scanPageData(r, map[string]any{
			"Error": "Tiket tidak ditemukan.",
		}))
		return
	}
	if errors.Is(err, service.ErrTicketWrongEvent) {
		h.View.Tampil(w, "gatekeeper/scan.html", h.scanPageData(r, map[string]any{
			"Ticket": ticket,
			"Error":  "Tiket ini untuk event lain: " + ticket.EventName,
		}))
		return
	}

	errorMessages := map[error]string{
		service.ErrEventUpcoming:   "Event belum dimulai. Check-in hanya dibuka saat event berstatus ACTIVE.",
		service.ErrEventCancelled:  "Event sudah dibatalkan. Check-in tidak diperbolehkan.",
		service.ErrEventCompleted:  "Event sudah selesai. Check-in tidak diperbolehkan.",
		service.ErrTicketUsed:      "Tiket sudah pernah digunakan.",
		service.ErrTicketReserved:  "Tiket belum dibayar dan belum dapat digunakan.",
		service.ErrTicketRefunded:  "Tiket sudah direfund dan tidak berlaku.",
		service.ErrTicketCancelled: "Tiket sudah dibatalkan dan tidak berlaku.",
		service.ErrTicketNotActive: "Status tiket tidak valid.",
	}

	for serviceError, message := range errorMessages {
		if errors.Is(err, serviceError) {
			h.View.Tampil(w, "gatekeeper/scan.html", h.scanPageData(r, map[string]any{
				"Ticket": ticket,
				"Error":  message,
			}))
			return
		}
	}

	if err != nil {
		log.Printf("Gagal memvalidasi tiket %s: %v", code, err)
		http.Error(w, "Terjadi kesalahan saat memvalidasi tiket.", http.StatusInternalServerError)
		return
	}

	h.View.Tampil(w, "gatekeeper/scan.html", h.scanPageData(r, map[string]any{
		"Ticket":  ticket,
		"Success": "Check-in berhasil! Selamat datang.",
	}))
}

// PrintBuyerTicket menampilkan Boarding Pass milik Buyer yang sedang login.
func (h *GatekeeperHandler) PrintBuyerTicket(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)

	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	code := strings.TrimSpace(r.PathValue("code"))

	if code == "" {
		http.Error(w, "Kode tiket wajib diisi.", http.StatusBadRequest)
		return
	}

	boardingPass, err := h.GatekeeperSvc.GetBoardingPassForBuyer(
		r.Context(),
		code,
		user.ID,
	)

	if errors.Is(err, service.ErrTicketNotFound) {
		http.Error(w, "Tiket tidak ditemukan.", http.StatusNotFound)
		return
	}

	if err != nil {
		log.Printf("Gagal mengambil boarding pass %s: %v", code, err)
		http.Error(w, "Terjadi kesalahan saat mengambil data tiket.", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":        "Boarding Pass - KampusTix",
		"User":         user,
		"BoardingPass": boardingPass,
	}

	h.View.Tampil(w, "gatekeeper/print.html", data)
}
