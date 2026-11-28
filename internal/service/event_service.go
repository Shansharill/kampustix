
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

type EventService struct {
	Repo *repository.EventRepo
}

// Validasi data event sebelum disimpan.
func (s *EventService) ValidasiEvent(e *model.Event) error {
	if e == nil {
		return errors.New("data event tidak boleh kosong")
	}

	e.Title = strings.TrimSpace(e.Title)
	e.Venue = strings.TrimSpace(e.Venue)
	e.EventType = strings.ToUpper(strings.TrimSpace(e.EventType))
	e.Status = strings.ToUpper(strings.TrimSpace(e.Status))

	if e.Title == "" {
		return errors.New("judul event wajib diisi")
	}

	if e.Venue == "" {
		return errors.New("lokasi event wajib diisi")
	}

	if e.EventType != "CONCERT" && e.EventType != "SEMINAR" {
		return errors.New("jenis event harus CONCERT atau SEMINAR")
	}

	if e.Status == "" {
		e.Status = "UPCOMING"
	}

	if e.Status != "UPCOMING" &&
		e.Status != "ACTIVE" &&
		e.Status != "COMPLETED" &&
		e.Status != "CANCELLED" {
		return errors.New("status event tidak valid")
	}

	if e.TotalCapacity <= 0 {
		return errors.New("kapasitas event harus lebih dari 0")
	}

	if e.EventDate.IsZero() {
		return errors.New("tanggal event wajib diisi")
	}

	if e.CreatedBy <= 0 {
		return errors.New("ID pembuat event tidak valid")
	}

	return nil
}

// Mengambil seluruh event.
func (s *EventService) SemuaEvent(ctx context.Context) ([]model.Event, error) {
	return s.Repo.SemuaEvent(ctx)
}

// Mengambil event berdasarkan ID.
func (s *EventService) CariEventByID(ctx context.Context, id int64) (model.Event, error) {
	if id <= 0 {
		return model.Event{}, errors.New("ID event tidak valid")
	}

	return s.Repo.CariEventByID(ctx, id)
}

// Menambahkan event baru.
func (s *EventService) TambahEvent(ctx context.Context, e *model.Event) error {
	if err := s.ValidasiEvent(e); err != nil {
		return err
	}

	return s.Repo.TambahEvent(ctx, e)
}

// Memperbarui event.
func (s *EventService) UpdateEvent(ctx context.Context, e *model.Event) error {
	if e == nil || e.ID <= 0 {
		return errors.New("ID event tidak valid")
	}

	if err := s.ValidasiEvent(e); err != nil {
		return err
	}

	return s.Repo.UpdateEvent(ctx, e)
}

// Menghapus event.
func (s *EventService) HapusEvent(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("ID event tidak valid")
	}

	return s.Repo.HapusEvent(ctx, id)
}


// Validasi kategori tiket sebelum disimpan.
func (s *EventService) ValidasiKategori(k *model.TicketCategory) error {
	if k == nil {
		return errors.New("data kategori tidak boleh kosong")
	}

	k.CategoryName = strings.TrimSpace(k.CategoryName)

	if k.EventID <= 0 {
		return errors.New("ID event tidak valid")
	}

	if k.CategoryName == "" {
		return errors.New("nama kategori wajib diisi")
	}

	if k.TotalQuota <= 0 {
		return errors.New("kuota kategori harus lebih dari 0")
	}

	return nil
}

// Mengambil seluruh kategori milik suatu event.
func (s *EventService) SemuaKategoriByEventID(
	ctx context.Context,
	eventID int64,
) ([]model.TicketCategory, error) {
	if eventID <= 0 {
		return nil, errors.New("ID event tidak valid")
	}

	return s.Repo.SemuaKategoriByEventID(ctx, eventID)
}

// Menambahkan kategori tiket baru.
func (s *EventService) TambahKategori(
	ctx context.Context,
	k *model.TicketCategory,
) error {
	if err := s.ValidasiKategori(k); err != nil {
		return err
	}

	return s.Repo.TambahKategori(ctx, k)
}

// Validasi tier tiket sebelum disimpan.
func (s *EventService) ValidasiTier(t *model.TicketTier) error {
	if t == nil {
		return errors.New("data tier tidak boleh kosong")
	}

	t.TierName = strings.TrimSpace(t.TierName)

	if t.CategoryID <= 0 {
		return errors.New("ID kategori tidak valid")
	}

	if t.TierName == "" {
		return errors.New("nama tier wajib diisi")
	}

	if t.SequenceOrder <= 0 {
		return errors.New("urutan tier harus lebih dari 0")
	}

	if t.Price <= 0 {
		return errors.New("harga tier harus lebih dari 0")
	}

	if t.Quota <= 0 {
		return errors.New("kuota tier harus lebih dari 0")
	}

	if t.StartDate.IsZero() || t.EndDate.IsZero() {
		return errors.New("tanggal mulai dan akhir tier wajib diisi")
	}

	if t.StartDate.After(t.EndDate) {
		return errors.New("tanggal mulai tidak boleh melewati tanggal akhir")
	}

	if t.UsedQuota < 0 || t.UsedQuota > t.Quota {
		return errors.New("jumlah tiket terpakai tidak valid")
	}

	return nil
}

// Mengambil seluruh tier milik suatu kategori.
func (s *EventService) SemuaTierByCategoryID(
	ctx context.Context,
	categoryID int64,
) ([]model.TicketTier, error) {
	if categoryID <= 0 {
		return nil, errors.New("ID kategori tidak valid")
	}

	return s.Repo.SemuaTierByCategoryID(ctx, categoryID)
}

// Menambahkan tier tiket baru.
func (s *EventService) TambahTier(
	ctx context.Context,
	t *model.TicketTier,
) error {
	if err := s.ValidasiTier(t); err != nil {
		return err
	}

	return s.Repo.TambahTier(ctx, t)
}

// Mengambil tier aktif berdasarkan kategori.
func (s *EventService) CariTierAktif(
	ctx context.Context,
	categoryID int64,
) (*model.TicketTier, error) {
	if categoryID <= 0 {
		return nil, errors.New("ID kategori tidak valid")
	}

	return s.Repo.TierAktifByCategoryID(ctx, categoryID)
}


// Menghitung sisa kuota tiket.
func SisaKuota(totalQuota, usedQuota int) int {
	sisa := totalQuota - usedQuota

	if sisa < 0 {
		return 0
	}

	return sisa
}


// Menentukan tier tiket yang masih aktif berdasarkan aturan PRD.

func TierAktif(tiers []model.TicketTier, sekarang time.Time) *model.TicketTier {
	var tierTerpilih *model.TicketTier

	for i := range tiers {
		tier := &tiers[i]

		// Lewati tier jika kuotanya sudah habis.
		if tier.UsedQuota >= tier.Quota {
			continue
		}

		// Lewati tier jika tanggal penjualan belum dimulai.
		if sekarang.Before(tier.StartDate) {
			continue
		}

		// Lewati tier jika tanggal penjualan sudah berakhir.
		if sekarang.After(tier.EndDate) {
			continue
		}

		// Pilih tier dengan sequence_order terkecil.
		if tierTerpilih == nil ||
			tier.SequenceOrder < tierTerpilih.SequenceOrder {
			tierTerpilih = tier
		}
	}

	return tierTerpilih
}


func (s *EventService) CariKategoriByID(
	ctx context.Context,
	id int64,
) (model.TicketCategory, error) {
	if id <= 0 {
		return model.TicketCategory{}, errors.New("ID kategori tidak valid")
	}

	return s.Repo.CariKategoriByID(ctx, id)
}