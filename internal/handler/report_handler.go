package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kampustix/internal/middleware"
	"kampustix/internal/model"
	"kampustix/internal/service"
)

// ReportHandler: halaman admin untuk kode promo & laporan keuangan (Anggota 4).
type ReportHandler struct {
	Svc  *service.PromoReportService
	View *Renderer
}

// DaftarPromo  GET /admin/promos
func (h *ReportHandler) DaftarPromo(w http.ResponseWriter, r *http.Request) {
	h.tampilPromo(w, r, model.PromoInput{}, "")
}

// SimpanPromo  POST /admin/promos
func (h *ReportHandler) SimpanPromo(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal memproses formulir", http.StatusBadRequest)
		return
	}
	in := promoDariForm(r)

	if err := h.Svc.BuatPromo(r.Context(), in); err != nil {
		if pe, ok := service.IsPromoError(err); ok {
			// Input salah: tampilkan lagi form dengan isian sebelumnya + pesan error.
			h.tampilPromo(w, r, in, pe.Pesan)
			return
		}
		log.Printf("error simpan promo: %v", err)
		http.Error(w, "Terjadi kesalahan pada server", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/promos?success=Kode+promo+berhasil+ditambahkan", http.StatusSeeOther)
}

// TogglePromo  POST /admin/promos/{id}/toggle
func (h *ReportHandler) TogglePromo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "ID tidak valid", http.StatusBadRequest)
		return
	}
	if err := h.Svc.ToggleAktif(r.Context(), id); err != nil {
		if errors.Is(err, model.ErrPromoNotFound) {
			http.Error(w, "Kode promo tidak ditemukan", http.StatusNotFound)
			return
		}
		log.Printf("error toggle promo: %v", err)
		http.Error(w, "Terjadi kesalahan pada server", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/promos?success=Status+promo+diperbarui", http.StatusSeeOther)
}

// LaporanRevenue  GET /admin/reports/revenue
func (h *ReportHandler) LaporanRevenue(w http.ResponseWriter, r *http.Request) {
	d, err := h.Svc.Laporan(r.Context())
	if err != nil {
		log.Printf("error laporan: %v", err)
		http.Error(w, "Terjadi kesalahan pada server", http.StatusInternalServerError)
		return
	}
	h.View.Tampil(w, "admin/laporan.html", map[string]any{
		"Title":     "Laporan Keuangan - KampusTix",
		"User":      middleware.CurrentUser(r),
		"Ringkasan": d.Ringkasan,
		"Rincian":   d.Rincian,
	})
}

// ---------------------------------------------------------------- pembantu

func (h *ReportHandler) tampilPromo(w http.ResponseWriter, r *http.Request, in model.PromoInput, errMsg string) {
	d, err := h.Svc.DataHalamanPromo(r.Context())
	if err != nil {
		log.Printf("error halaman promo: %v", err)
		http.Error(w, "Terjadi kesalahan pada server", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"Title":      "Kelola Kode Promo - KampusTix",
		"User":       middleware.CurrentUser(r),
		"Promos":     d.Promos,
		"Events":     d.Events,
		"Categories": d.Categories,
		"Input":      in,
		"Success":    r.URL.Query().Get("success"),
	}
	if errMsg != "" {
		data["Error"] = errMsg
	} else if e := r.URL.Query().Get("error"); e != "" {
		data["Error"] = e
	}
	h.View.Tampil(w, "admin/promo.html", data)
}

func promoDariForm(r *http.Request) model.PromoInput {
	eventID, _ := strconv.ParseInt(r.PostFormValue("event_id"), 10, 64)
	catID, _ := strconv.ParseInt(r.PostFormValue("category_id"), 10, 64)
	persen, _ := strconv.ParseFloat(strings.Replace(strings.TrimSpace(r.PostFormValue("discount_percent")), ",", ".", 1), 64)
	maxUses, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue("max_uses")))

	raw := strings.TrimSpace(r.PostFormValue("valid_until"))
	var until time.Time
	if raw != "" {
		// <input type="datetime-local"> mengirim "2006-01-02T15:04"
		if t, err := time.ParseInLocation("2006-01-02T15:04", raw, time.Local); err == nil {
			until = t
		}
	}
	return model.PromoInput{
		Code:            r.PostFormValue("code"),
		EventID:         eventID,
		CategoryID:      catID,
		DiscountPercent: persen,
		MaxUses:         maxUses,
		ValidUntil:      until,
		ValidUntilRaw:   raw,
	}
}
