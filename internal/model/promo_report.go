package model

import (
	"errors"
	"time"
)

// ErrPromoNotFound dikembalikan repository jika kode promo tidak ada.
var ErrPromoNotFound = errors.New("kode promo tidak ditemukan")

// PromoCode merepresentasikan satu baris tabel `promo_codes`
// (ditambah nama event & kategori untuk keperluan tampilan).
type PromoCode struct {
	ID              int64
	EventID         int64
	EventTitle      string
	CategoryID      *int64 // nil = berlaku untuk semua kategori di event tersebut
	CategoryName    string // "" jika CategoryID == nil
	Code            string
	DiscountPercent float64
	MaxUses         int
	UsedCount       int
	ValidUntil      time.Time
	IsActive        bool
	CreatedAt       time.Time
}

// SisaKuota menghitung sisa pemakaian promo.
func (p PromoCode) SisaKuota() int { return p.MaxUses - p.UsedCount }

// Kedaluwarsa true jika batas tanggal promo sudah lewat.
func (p PromoCode) Kedaluwarsa() bool { return time.Now().After(p.ValidUntil) }

// PromoInput menampung isian form tambah promo.
type PromoInput struct {
	Code            string
	EventID         int64
	CategoryID      int64 // 0 = semua kategori
	DiscountPercent float64
	MaxUses         int
	ValidUntil      time.Time
	ValidUntilRaw   string // isian mentah form, untuk diisi ulang saat error
}

// PilihanEvent & PilihanKategori dipakai untuk dropdown di form promo.
type PilihanEvent struct {
	ID    int64
	Title string
}

type PilihanKategori struct {
	ID      int64
	EventID int64
	Name    string
}

// HasilPromo adalah hasil validasi + perhitungan diskon yang dipakai modul Checkout.
type HasilPromo struct {
	PromoID int64
	Kode    string
	Persen  float64
	Diskon  int64 // rupiah
}

// RevenueReport berisi 5 metrik utama laporan keuangan (PRD 3.8). Satuan: rupiah.
type RevenueReport struct {
	Gross            int64 // jumlah subtotal order PAID
	TotalDiskon      int64 // jumlah discount_amount order PAID
	NetSebelumRefund int64 // Gross - TotalDiskon
	TotalRefund      int64 // jumlah refund_amount (mandiri 75% + event batal 100%)
	NetAkhir         int64 // NetSebelumRefund - TotalRefund
}

// BarisPenjualan = rincian omzet per kategori & tier.
type BarisPenjualan struct {
	EventTitle   string
	Kategori     string
	Tier         string
	TiketTerjual int
	Gross        int64
	Diskon       int64
	Refund       int64
	Net          int64 // Gross - Diskon - Refund
}
