package model

import (
	"database/sql"
	"math"
	"time"
)

// Status tiket sesuai ENUM tabel tickets
const (
	TiketReserved  = "RESERVED"
	TiketActive    = "ACTIVE"
	TiketUsed      = "USED"
	TiketRefunded  = "REFUNDED"
	TiketCancelled = "CANCELLED"
)

// TiketSaya adalah satu kartu tiket di halaman /my-tickets (tickets + order + event + kategori + tier + refund)
type TiketSaya struct {
	ID            int64
	TicketCode    string
	HolderName    string
	HolderEmail   string
	Status        string
	TransferCount int
	UnitNetPrice  float64
	OrderCode     string
	EventTitle    string
	Venue         string
	EventDate     time.Time
	EventStatus   string
	Kategori      string
	Tier          string
	MasihH7       bool            // NOW() <= event_date - 7 hari, dihitung MySQL
	RefundAmount  sql.NullFloat64 // terisi jika tiket sudah di-refund
	RefundPercent sql.NullFloat64
}

// BolehTransfer dipakai template untuk mengunci tombol transfer (PRD 3.6)
func (t TiketSaya) BolehTransfer() bool {
	return t.Status == TiketActive && t.TransferCount < 1 && t.EventStatus != "CANCELLED"
}

// BolehRefund dipakai template untuk menampilkan tombol refund (PRD 3.5)
func (t TiketSaya) BolehRefund() bool {
	return t.Status == TiketActive && t.MasihH7 && t.EventStatus != "CANCELLED"
}

// EstimasiRefund = nominal yang diterima jika pembeli refund mandiri sekarang (75%)
func (t TiketSaya) EstimasiRefund() float64 { return HitungRefund(t.UnitNetPrice, 75) }

// HitungRefund = unit_net_price × persen/100, dibulatkan 2 desimal sesuai DECIMAL(10,2)
func HitungRefund(unitNetPrice, persen float64) float64 {
	return math.Round(unitNetPrice*persen) / 100
}

// TiketUntukAksi adalah data minimal tiket yang dikunci (FOR UPDATE) saat transfer/refund
type TiketUntukAksi struct {
	ID            int64
	OrderID       int64
	TierID        int64
	Status        string
	TransferCount int
	UnitNetPrice  float64
	EventStatus   string
	MasihH7       bool
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
