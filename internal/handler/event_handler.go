
package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"kampustix/internal/middleware"
	"kampustix/internal/model"
	"kampustix/internal/service"
)

type EventHandler struct {
	EventSvc *service.EventService
	View     *Renderer
}

// GET /
// Menampilkan katalog event untuk pengunjung.
func (h *EventHandler) Katalog(w http.ResponseWriter, r *http.Request) {
	events, err := h.EventSvc.SemuaEvent(r.Context())
	if err != nil {
		http.Error(w, "Gagal mengambil data event", http.StatusInternalServerError)
		return
	}

	// Katalog publik hanya menampilkan event UPCOMING dan ACTIVE.
	var katalog []model.Event
	for _, event := range events {
		if event.Status == "UPCOMING" || event.Status == "ACTIVE" {
			katalog = append(katalog, event)
		}
	}

	data := map[string]any{
		"Title":  "Katalog Event - KampusTix",
		"Events": katalog,
		"User":   middleware.CurrentUser(r),
	}

	h.View.Tampil(w, "event/katalog.html", data)
}

// GET /events/{id}
// Menampilkan informasi dasar sebuah event.


func (h *EventHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "ID event tidak valid", http.StatusBadRequest)
		return
	}

	event, err := h.EventSvc.CariEventByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Event tidak ditemukan", http.StatusNotFound)
		return
	}

	kategori, err := h.EventSvc.SemuaKategoriByEventID(r.Context(), id)
	if err != nil {
		http.Error(w, "Gagal mengambil kategori tiket", http.StatusInternalServerError)
		return
	}

	var daftarKategori []map[string]any

	for _, k := range kategori {
		tiers, err := h.EventSvc.SemuaTierByCategoryID(
			r.Context(), k.ID,
		)
		if err != nil {
			http.Error(w, "Gagal mengambil tier tiket", http.StatusInternalServerError)
			return
		}

		tierAktif, err := h.EventSvc.CariTierAktif(
			r.Context(), k.ID,
		)
		if err != nil {
			http.Error(w, "Gagal menentukan tier aktif", http.StatusInternalServerError)
			return
		}

		var daftarTier []map[string]any

		for _, t := range tiers {
			daftarTier = append(daftarTier, map[string]any{
				"Tier":      t,
				"SisaKuota": service.SisaKuota(t.Quota, t.UsedQuota),
				"Aktif":     tierAktif != nil && tierAktif.ID == t.ID,
			})
		}

		daftarKategori = append(daftarKategori, map[string]any{
			"Category": k,
			"Tiers":    daftarTier,
		})
	}

	data := map[string]any{
		"Title":    "Detail Event - KampusTix",
		"Event":    event,
		"Kategori": daftarKategori,
		"User":     middleware.CurrentUser(r),
	}

	h.View.Tampil(w, "event/detail.html", data)
}


// GET /admin/events
// Menampilkan daftar event untuk admin.

func (h *EventHandler) AdminEvent(w http.ResponseWriter, r *http.Request) {
	events, err := h.EventSvc.SemuaEvent(r.Context())
	if err != nil {
		http.Error(w, "Gagal mengambil data event", http.StatusInternalServerError)
		return
	}

	// Menyiapkan data kategori dan tier untuk setiap event.
	var daftarEvent []map[string]any

	for _, event := range events {
		kategori, err := h.EventSvc.SemuaKategoriByEventID(
			r.Context(), event.ID,
		)
		if err != nil {
			http.Error(w, "Gagal mengambil kategori tiket", http.StatusInternalServerError)
			return
		}

		var daftarKategori []map[string]any

		for _, k := range kategori {
			tiers, err := h.EventSvc.SemuaTierByCategoryID(
				r.Context(), k.ID,
			)
			if err != nil {
				http.Error(w, "Gagal mengambil tier tiket", http.StatusInternalServerError)
				return
			}

			daftarKategori = append(daftarKategori, map[string]any{
				"Category": k,
				"Tiers":    tiers,
			})
		}

		daftarEvent = append(daftarEvent, map[string]any{
			"Event":      event,
			"Kategori":   daftarKategori,
		})
	}

	data := map[string]any{
		"Title":  "Kelola Event - KampusTix",
		"Events": daftarEvent,
		"User":   middleware.CurrentUser(r),
	}

	h.View.Tampil(w, "event/admin_event.html", data)
}


// POST /admin/events
// Menambahkan event baru.
func (h *EventHandler) TambahEvent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form tidak valid", http.StatusBadRequest)
		return
	}

	user := middleware.CurrentUser(r)
	if user == nil {
		http.Error(w, "Anda harus login terlebih dahulu", http.StatusUnauthorized)
		return
	}

	eventDate, err := parseTanggalEvent(r.PostFormValue("event_date"))
	if err != nil {
		http.Error(w, "Format tanggal event tidak valid", http.StatusBadRequest)
		return
	}

	capacity, err := strconv.Atoi(r.PostFormValue("total_capacity"))
	if err != nil {
		http.Error(w, "Kapasitas harus berupa angka", http.StatusBadRequest)
		return
	}

	event := &model.Event{
		Title:         r.PostFormValue("title"),
		EventType:     r.PostFormValue("event_type"),
		Description:   r.PostFormValue("description"),
		Venue:         r.PostFormValue("venue"),
		EventDate:     eventDate,
		TotalCapacity: capacity,
		Status:        "UPCOMING",
		CreatedBy:     user.ID,
	}

	if err := h.EventSvc.TambahEvent(r.Context(), event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/events", http.StatusSeeOther)
}

// POST /admin/events/{id}/update
// Memperbarui data event.
func (h *EventHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form tidak valid", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "ID event tidak valid", http.StatusBadRequest)
		return
	}

	eventLama, err := h.EventSvc.CariEventByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Event tidak ditemukan", http.StatusNotFound)
		return
	}

	eventDate, err := parseTanggalEvent(r.PostFormValue("event_date"))
	if err != nil {
		http.Error(w, "Format tanggal event tidak valid", http.StatusBadRequest)
		return
	}

	capacity, err := strconv.Atoi(r.PostFormValue("total_capacity"))
	if err != nil {
		http.Error(w, "Kapasitas harus berupa angka", http.StatusBadRequest)
		return
	}

	event := &model.Event{
		ID:            id,
		Title:         r.PostFormValue("title"),
		EventType:     r.PostFormValue("event_type"),
		Description:   r.PostFormValue("description"),
		Venue:         r.PostFormValue("venue"),
		EventDate:     eventDate,
		TotalCapacity: capacity,
		Status:        strings.ToUpper(r.PostFormValue("status")),
		CreatedBy:     eventLama.CreatedBy,
	}

	if err := h.EventSvc.UpdateEvent(r.Context(), event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/events", http.StatusSeeOther)
}

// POST /admin/events/{id}/delete
// Menghapus event.
func (h *EventHandler) HapusEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "ID event tidak valid", http.StatusBadRequest)
		return
	}

	if err := h.EventSvc.HapusEvent(r.Context(), id); err != nil {
		http.Error(w, "Gagal menghapus event. Pastikan event tidak sedang digunakan.", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/events", http.StatusSeeOther)
}


// POST /admin/events/{id}/categories
// Menambahkan kategori tiket ke suatu event.
func (h *EventHandler) TambahKategori(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form kategori tidak valid", http.StatusBadRequest)
		return
	}

	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || eventID <= 0 {
		http.Error(w, "ID event tidak valid", http.StatusBadRequest)
		return
	}

	// Pastikan event yang dituju memang tersedia.
	_, err = h.EventSvc.CariEventByID(r.Context(), eventID)
	if err != nil {
		http.Error(w, "Event tidak ditemukan", http.StatusNotFound)
		return
	}

	totalQuota, err := strconv.Atoi(r.PostFormValue("total_quota"))
	if err != nil {
		http.Error(w, "Kuota kategori harus berupa angka", http.StatusBadRequest)
		return
	}

	kategori := &model.TicketCategory{
		EventID:     eventID,
		CategoryName: r.PostFormValue("category_name"),
		TotalQuota:  totalQuota,
	}

	if err := h.EventSvc.TambahKategori(r.Context(), kategori); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/events", http.StatusSeeOther)
}

// POST /admin/categories/{id}/tiers
// Menambahkan tahapan harga ke suatu kategori.
func (h *EventHandler) TambahTier(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Form tier tidak valid", http.StatusBadRequest)
		return
	}

	categoryID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || categoryID <= 0 {
		http.Error(w, "ID kategori tidak valid", http.StatusBadRequest)
		return
	}

	// Pastikan kategori tersedia.
	// EventID pada model kategori diperiksa melalui daftar kategori event.
	// Untuk itu, kita perlu mencari kategori dari seluruh event yang tersedia.
		_, err = h.EventSvc.CariKategoriByID(r.Context(), categoryID)
	if err != nil {
		http.Error(w, "Kategori tiket tidak ditemukan", http.StatusNotFound)
		return
	}

	sequenceOrder, err := strconv.Atoi(r.PostFormValue("sequence_order"))
	if err != nil {
		http.Error(w, "Urutan tier harus berupa angka", http.StatusBadRequest)
		return
	}

	price, err := strconv.ParseFloat(r.PostFormValue("price"), 64)
	if err != nil {
		http.Error(w, "Harga tier harus berupa angka", http.StatusBadRequest)
		return
	}

	quota, err := strconv.Atoi(r.PostFormValue("quota"))
	if err != nil {
		http.Error(w, "Kuota tier harus berupa angka", http.StatusBadRequest)
		return
	}

	startDate, err := parseTanggalEvent(r.PostFormValue("start_date"))
	if err != nil {
		http.Error(w, "Format tanggal mulai tier tidak valid", http.StatusBadRequest)
		return
	}

	endDate, err := parseTanggalEvent(r.PostFormValue("end_date"))
	if err != nil {
		http.Error(w, "Format tanggal akhir tier tidak valid", http.StatusBadRequest)
		return
	}

	tier := &model.TicketTier{
		CategoryID:    categoryID,
		TierName:      r.PostFormValue("tier_name"),
		SequenceOrder: sequenceOrder,
		Price:         price,
		Quota:         quota,
		UsedQuota:     0,
		StartDate:     startDate,
		EndDate:       endDate,
	}

	if err := h.EventSvc.TambahTier(r.Context(), tier); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/events", http.StatusSeeOther)
}


// Membaca tanggal dari input HTML date atau datetime-local.
func parseTanggalEvent(value string) (time.Time, error) {
	value = strings.TrimSpace(value)

	layouts := []string{
		"2006-01-02T15:04",
		"2006-01-02",
	}

	for _, layout := range layouts {
		tanggal, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return tanggal, nil
		}
	}

	return time.Time{}, strconv.ErrSyntax
}
