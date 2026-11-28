package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"kampustix/internal/middleware"
	"kampustix/internal/model"
	"kampustix/internal/service"
)

// TicketHandler menangani Tiket Saya, transfer, refund mandiri, dan pembatalan event (Anggota 5)
type TicketHandler struct {
	TicketSvc *service.TicketService
	View      *Renderer
}

// errorUntukUser: pesan error bisnis yang aman ditampilkan ke pengguna (selain ini = error server)
var errorUntukUser = []error{
	service.ErrTiketTidakDitemukan, service.ErrTiketTidakAktif, service.ErrSudahPernahTransfer,
	service.ErrEventSudahBatal, service.ErrEventSudahLewat, service.ErrLewatH7, service.ErrPemegangSama,
	service.ErrNamaPemegang, service.ErrEmailPemegang, service.ErrAlasanTerlaluPanjang,
	service.ErrEventTidakDitemukan, service.ErrEventTidakBisaBatal,
}

// kembali melakukan redirect (pola Post/Redirect/Get): ke tujuanSukses dengan pesan sukses,
// atau ke tujuanGagal dengan pesan error. Kedua tujuan diakhiri "?" atau "&".
func kembali(w http.ResponseWriter, r *http.Request, tujuanSukses, tujuanGagal string, err error, sukses string) {
	if err == nil {
		http.Redirect(w, r, tujuanSukses+"pesan="+url.QueryEscape(sukses), http.StatusSeeOther)
		return
	}
	for _, e := range errorUntukUser {
		if errors.Is(err, e) {
			http.Redirect(w, r, tujuanGagal+"error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}
	log.Printf("error %s %s: %v", r.Method, r.URL.Path, err)
	http.Error(w, "Terjadi kesalahan pada server, silakan coba lagi", http.StatusInternalServerError)
}

// idTiketDariPath membaca {id} dari URL; hanya bilangan bulat positif yang diterima
func idTiketDariPath(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

// TiketSaya menampilkan tiket milik pembeli yang sedang login
// Endpoint: GET /my-tickets?tab=aktif|riwayat|dibatalkan
func (h *TicketHandler) TiketSaya(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)
	tab := r.URL.Query().Get("tab")
	if tab != "riwayat" && tab != "dibatalkan" {
		tab = "aktif"
	}
	tiket, err := h.TicketSvc.TiketSaya(r.Context(), user.ID, tab)
	if err != nil {
		log.Printf("gagal memuat tiket user %d: %v", user.ID, err)
		http.Error(w, "Gagal memuat tiket", http.StatusInternalServerError)
		return
	}
	h.View.Tampil(w, "tiket/tiket_saya.html", map[string]any{
		"Title":   "Tiket Saya - KampusTix",
		"User":    user,
		"Tab":     tab,
		"Tiket":   tiket,
		"Success": r.URL.Query().Get("pesan"),
		"Error":   r.URL.Query().Get("error"),
	})
}

// TransferTiket memindahkan pemegang tiket (maksimal 1 kali)
// Endpoint: POST /tickets/{id}/transfer
func (h *TicketHandler) TransferTiket(w http.ResponseWriter, r *http.Request) {
	id, ok := idTiketDariPath(r)
	if !ok {
		http.Error(w, "ID tiket tidak valid", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}
	in := model.TransferInput{
		HolderName:  r.PostFormValue("holder_name"),
		HolderEmail: r.PostFormValue("holder_email"),
	}
	err := h.TicketSvc.Transfer(r.Context(), middleware.CurrentUser(r).ID, id, in)
	kembali(w, r, "/my-tickets?", "/my-tickets?", err, "Tiket berhasil dipindahkan. Transfer untuk tiket ini sekarang terkunci.")
}

// RefundTiket mengajukan refund mandiri 75% (maksimal H-7)
// Endpoint: POST /tickets/{id}/refund
func (h *TicketHandler) RefundTiket(w http.ResponseWriter, r *http.Request) {
	id, ok := idTiketDariPath(r)
	if !ok {
		http.Error(w, "ID tiket tidak valid", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}
	amount, err := h.TicketSvc.Refund(r.Context(), middleware.CurrentUser(r).ID, id, r.PostFormValue("reason"))
	// Berhasil -> tiket pindah ke tab Dibatalkan; gagal -> tetap di tab Aktif agar tiketnya terlihat
	kembali(w, r, "/my-tickets?tab=dibatalkan&", "/my-tickets?tab=aktif&", err,
		fmt.Sprintf("Refund berhasil. Dana %s (75%%) akan dikembalikan.", rupiah(amount)))
}

// BatalkanEvent membatalkan event dan me-refund 100% semua tiket aktif (khusus ADMIN)
// Endpoint: POST /admin/events/{id}/cancel
func (h *TicketHandler) BatalkanEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := idTiketDariPath(r)
	if !ok {
		http.Error(w, "ID event tidak valid", http.StatusBadRequest)
		return
	}
	jumlah, err := h.TicketSvc.BatalkanEvent(r.Context(), middleware.CurrentUser(r).ID, id)
	kembali(w, r, "/admin/events?", "/admin/events?", err,
		fmt.Sprintf("Event dibatalkan. %d tiket aktif di-refund 100%%.", jumlah))
}
