package service

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

// PromoError = kesalahan input/aturan promo yang aman ditampilkan ke pengguna.
// (Sengaja tidak memakai ValidasiError milik contoh d_produk supaya modul ini mandiri.)
type PromoError struct{ Pesan string }

func (e *PromoError) Error() string { return e.Pesan }

func promoErr(p string) error { return &PromoError{Pesan: p} }

var polaKode = regexp.MustCompile(`^[A-Z0-9_-]{3,30}$`)

// PromoReportService: logika bisnis promo & laporan keuangan (Anggota 4).
type PromoReportService struct {
	Repo *repository.PromoReportRepo
}

// DataFormPromo = semua data yang dibutuhkan halaman /admin/promos.
type DataFormPromo struct {
	Promos     []model.PromoCode
	Events     []model.PilihanEvent
	Categories []model.PilihanKategori
}

func (s *PromoReportService) DataHalamanPromo(ctx context.Context) (DataFormPromo, error) {
	var d DataFormPromo
	var err error
	if d.Promos, err = s.Repo.SemuaPromo(ctx); err != nil {
		return d, err
	}
	if d.Events, err = s.Repo.DaftarEvent(ctx); err != nil {
		return d, err
	}
	d.Categories, err = s.Repo.DaftarKategori(ctx)
	return d, err
}

// BuatPromo memvalidasi lalu menyimpan promo baru.
func (s *PromoReportService) BuatPromo(ctx context.Context, in model.PromoInput) error {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if !polaKode.MatchString(in.Code) {
		return promoErr("Kode promo harus 3-30 karakter: huruf, angka, strip (-) atau garis bawah (_), tanpa spasi")
	}
	if in.EventID <= 0 {
		return promoErr("Pilih event terlebih dahulu")
	}
	// NaN / Inf lolos perbandingan biasa, jadi dicek eksplisit.
	if math.IsNaN(in.DiscountPercent) || math.IsInf(in.DiscountPercent, 0) ||
		in.DiscountPercent <= 0 || in.DiscountPercent > 100 {
		return promoErr("Persentase diskon harus lebih dari 0 dan maksimal 100")
	}
	in.DiscountPercent = math.Round(in.DiscountPercent*100) / 100 // kolom DECIMAL(5,2)
	if in.DiscountPercent <= 0 {
		return promoErr("Persentase diskon terlalu kecil, minimal 0,01%")
	}
	if in.MaxUses < 1 {
		return promoErr("Kuota pemakaian minimal 1")
	}
	if in.MaxUses > math.MaxInt32 {
		return promoErr("Kuota pemakaian terlalu besar (maksimal 2.147.483.647)")
	}
	if in.ValidUntil.IsZero() {
		return promoErr("Batas tanggal berlaku wajib diisi")
	}
	if !in.ValidUntil.After(time.Now()) {
		return promoErr("Batas tanggal berlaku harus di masa depan")
	}

	ev, err := s.Repo.InfoEvent(ctx, in.EventID, in.ValidUntil)
	if err != nil {
		return err
	}
	if ev == nil {
		return promoErr("Event yang dipilih tidak ditemukan")
	}
	if ev.Status == "CANCELLED" || ev.Status == "COMPLETED" {
		return promoErr("Event ini sudah dibatalkan atau selesai, tidak bisa dibuatkan promo")
	}
	if ev.PromoMelewatiEvent {
		return promoErr("Batas berlaku promo tidak boleh melewati tanggal event")
	}

	if in.CategoryID > 0 {
		cocok, err := s.Repo.KategoriMilikEvent(ctx, in.CategoryID, in.EventID)
		if err != nil {
			return err
		}
		if !cocok {
			return promoErr("Kategori yang dipilih bukan bagian dari event tersebut")
		}
	}

	dup, err := s.Repo.KodeSudahAda(ctx, in.Code)
	if err != nil {
		return err
	}
	if dup {
		return promoErr("Kode promo \"" + in.Code + "\" sudah dipakai, gunakan kode lain")
	}
	if _, err := s.Repo.TambahPromo(ctx, in); err != nil {
		// Dua permintaan dengan kode sama bisa lolos cek di atas bersamaan;
		// UNIQUE di database yang jadi penentu akhir.
		if errors.Is(err, model.ErrKodePromoDuplikat) {
			return promoErr("Kode promo \"" + in.Code + "\" sudah dipakai, gunakan kode lain")
		}
		return err
	}
	return nil
}

// ToggleAktif mengaktifkan/menonaktifkan promo.
func (s *PromoReportService) ToggleAktif(ctx context.Context, id int64) error {
	return s.Repo.ToggleAktif(ctx, id)
}

// ------------------------------------------------ DIPANGGIL MODUL CHECKOUT

// HitungDiskon: Discount = Gross x persen/100 (dibulatkan ke rupiah terdekat).
// Diskon tidak pernah melebihi subtotal.
func HitungDiskon(subtotal int64, persen float64) int64 {
	d := int64(math.Round(float64(subtotal) * persen / 100))
	if d > subtotal {
		d = subtotal
	}
	if d < 0 {
		d = 0
	}
	return d
}

// HargaBersihPerTiket: unit_net_price = unit_base_price - (totalDiskon / qty)  (PRD 3.4).
// Nilai ini wajib disimpan di tickets.unit_net_price karena dipakai untuk hitung refund.
func HargaBersihPerTiket(unitBase float64, totalDiskon int64, qty int) float64 {
	if qty <= 0 {
		return unitBase
	}
	return unitBase - float64(totalDiskon)/float64(qty)
}

// ValidasiDanTerapkan dipanggil Checkout (Anggota 3) DI DALAM transaksi checkout.
// Ia memotong 1 kuota promo secara atomik dan menghitung diskonnya.
//
//   - kode kosong        -> (nil, nil): checkout jalan tanpa promo
//   - kode tidak valid   -> (nil, *PromoError): tampilkan pesan ke pembeli
//   - kode valid         -> (*HasilPromo, nil): simpan PromoID ke orders.promo_code_id
//
// Jika transaksi checkout di-rollback, kuota promo ikut kembali otomatis.
func (s *PromoReportService) ValidasiDanTerapkan(ctx context.Context, tx *sql.Tx,
	kode string, eventID, categoryID, subtotal int64) (*model.HasilPromo, error) {

	kode = strings.ToUpper(strings.TrimSpace(kode))
	if kode == "" {
		return nil, nil
	}
	p, err := s.Repo.PakaiKuota(ctx, tx, kode, eventID, categoryID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, promoErr("Kode promo tidak valid, sudah kedaluwarsa, kuota habis, atau tidak berlaku untuk tiket ini")
	}
	return &model.HasilPromo{
		PromoID: p.ID,
		Kode:    p.Code,
		Persen:  p.DiscountPercent,
		Diskon:  HitungDiskon(subtotal, p.DiscountPercent),
	}, nil
}

// LepasKuota dipanggil saat order EXPIRED / CANCELLED (Lazy Expiration Anggota 3).
func (s *PromoReportService) LepasKuota(ctx context.Context, tx *sql.Tx, promoID int64) error {
	return s.Repo.LepasKuota(ctx, tx, promoID)
}

// ------------------------------------------------------------------ LAPORAN

// DataLaporan = isi halaman /admin/reports/revenue.
type DataLaporan struct {
	Ringkasan model.RevenueReport
	Rincian   []model.BarisPenjualan
}

// HitungRingkasan menjumlahkan baris per tier menjadi 5 metrik utama (PRD 3.8).
// Karena KPI diturunkan dari baris yang sama dengan tabel, keduanya tidak pernah selisih.
func HitungRingkasan(rincian []model.BarisPenjualan) model.RevenueReport {
	var r model.RevenueReport
	for _, b := range rincian {
		r.Gross += b.Gross
		r.TotalDiskon += b.Diskon
		r.TotalRefund += b.Refund
	}
	r.NetSebelumRefund = r.Gross - r.TotalDiskon
	r.NetAkhir = r.NetSebelumRefund - r.TotalRefund
	return r
}

func (s *PromoReportService) Laporan(ctx context.Context) (DataLaporan, error) {
	rincian, err := s.Repo.PenjualanPerTier(ctx)
	if err != nil {
		return DataLaporan{}, err
	}
	return DataLaporan{Ringkasan: HitungRingkasan(rincian), Rincian: rincian}, nil
}

// IsPromoError membantu handler membedakan kesalahan input vs kesalahan server.
func IsPromoError(err error) (*PromoError, bool) {
	var pe *PromoError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}
