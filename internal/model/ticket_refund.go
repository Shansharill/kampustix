package model

import (
	"database/sql"
	"errors"
	"math"
	"time"
)

// Error aturan tiket. Didefinisikan di model supaya aturan yang SAMA dipakai
// oleh service (saat aksi) dan template (saat menampilkan tombol).
var (
	ErrTiketTidakAktif     = errors.New("hanya tiket berstatus aktif yang bisa diproses")
	ErrSudahPernahTransfer = errors.New("tiket ini sudah pernah dipindahkan, transfer hanya boleh 1 kali")
	ErrEventSudahBatal     = errors.New("event sudah dibatalkan, tiket sudah otomatis di-refund 100%")
	ErrEventSudahLewat     = errors.New("event sudah berlangsung atau selesai, tiket tidak bisa dipindahkan")
	ErrLewatH7             = errors.New("refund hanya bisa diajukan paling lambat 7 hari sebelum event")
)

// Status tiket sesuai ENUM tabel tickets
const (
	TiketReserved  = "RESERVED"
	TiketActive    = "ACTIVE"
	TiketUsed      = "USED"
	TiketRefunded  = "REFUNDED"
	TiketCancelled = "CANCELLED"
)

// StatusTiket berisi data yang menentukan boleh/tidaknya transfer & refund.
// Dipakai bersama oleh TiketSaya (tampilan) dan TiketUntukAksi (saat aksi dijalankan).
type StatusTiket struct {
	Status          string
	TransferCount   int
	EventStatus     string
	MasihH7         bool // NOW() <= event_date - 7 hari, dihitung MySQL
	EventBelumMulai bool // NOW() < event_date, dihitung MySQL
}

// AlasanTolakTransfer mengembalikan nil jika transfer boleh (PRD 3.6), atau alasan penolakannya
func (s StatusTiket) AlasanTolakTransfer() error {
	switch {
	case s.EventStatus == "CANCELLED":
		return ErrEventSudahBatal
	case s.Status != TiketActive:
		return ErrTiketTidakAktif
	case s.TransferCount >= 1:
		return ErrSudahPernahTransfer
	case s.EventStatus == "COMPLETED" || !s.EventBelumMulai:
		return ErrEventSudahLewat
	}
	return nil
}

// AlasanTolakRefund mengembalikan nil jika refund mandiri boleh (PRD 3.5), atau alasan penolakannya
func (s StatusTiket) AlasanTolakRefund() error {
	switch {
	case s.EventStatus == "CANCELLED":
		return ErrEventSudahBatal
	case s.Status != TiketActive:
		return ErrTiketTidakAktif
	case s.EventStatus == "COMPLETED" || !s.MasihH7:
		return ErrLewatH7
	}
	return nil
}

// TiketSaya adalah satu kartu tiket di halaman /my-tickets (tickets + order + event + kategori + tier + refund)
type TiketSaya struct {
	StatusTiket
	ID            int64
	TicketCode    string
	HolderName    string
	HolderEmail   string
	UnitNetPrice  float64
	OrderCode     string
	EventTitle    string
	Venue         string
	EventDate     time.Time
	Kategori      string
	Tier          string
	RefundAmount  sql.NullFloat64 // terisi jika tiket sudah di-refund
	RefundPercent sql.NullFloat64
}

// BolehTransfer dipakai template untuk menampilkan tombol transfer
func (t TiketSaya) BolehTransfer() bool { return t.AlasanTolakTransfer() == nil }

// BolehRefund dipakai template untuk menampilkan tombol refund
func (t TiketSaya) BolehRefund() bool { return t.AlasanTolakRefund() == nil }

// EstimasiRefund = nominal yang diterima jika pembeli refund mandiri sekarang (75%)
func (t TiketSaya) EstimasiRefund() float64 { return HitungRefund(t.UnitNetPrice, 75) }

// HitungRefund = unit_net_price × persen/100, dibulatkan 2 desimal sesuai DECIMAL(12,2)
func HitungRefund(unitNetPrice, persen float64) float64 {
	return math.Round(unitNetPrice*persen) / 100
}

// TiketUntukAksi adalah data minimal tiket yang dikunci (FOR UPDATE) saat transfer/refund
type TiketUntukAksi struct {
	StatusTiket
	ID           int64
	OrderID      int64
	TierID       int64
	HolderName   string
	HolderEmail  string
	UnitNetPrice float64
}

// Refund merepresentasikan satu baris tabel refunds (permohonan / catatan pengembalian dana)
type Refund struct {
	TicketID      int64
	OrderID       int64
	RequestedBy   int64
	RefundPercent float64 // 75 = refund mandiri, 100 = event dibatalkan
	RefundAmount  float64
	RefundType    string // BUYER_INITIATED atau EVENT_CANCELLED
	Reason        string
}

// TransferInput menampung data pemegang baru dari form transfer
type TransferInput struct {
	HolderName  string
	HolderEmail string
}
