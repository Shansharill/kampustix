package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

var (
	// Aturan tiket (sumbernya di model agar sama dengan yang dipakai template)
	ErrTiketTidakAktif     = model.ErrTiketTidakAktif
	ErrSudahPernahTransfer = model.ErrSudahPernahTransfer
	ErrEventSudahBatal     = model.ErrEventSudahBatal
	ErrEventSudahLewat     = model.ErrEventSudahLewat
	ErrLewatH7             = model.ErrLewatH7

	ErrTiketTidakDitemukan  = errors.New("tiket tidak ditemukan")
	ErrPemegangSama         = errors.New("nama dan email pemegang baru sama dengan pemegang sekarang, transfer dibatalkan agar jatah 1x tidak terpakai")
	ErrNamaPemegang         = errors.New("nama pemegang baru wajib diisi (maksimal 100 karakter)")
	ErrEmailPemegang        = errors.New("email pemegang baru tidak valid")
	ErrAlasanTerlaluPanjang = errors.New("alasan refund maksimal 255 karakter")

	ErrEventTidakDitemukan = errors.New("event tidak ditemukan")
	ErrEventTidakBisaBatal = errors.New("event yang sudah selesai atau sudah dibatalkan tidak bisa dibatalkan lagi")
)

// TicketService menangani logika bisnis Tiket Saya, transfer, dan refund (Anggota 5)
type TicketService struct {
	RefundRepo *repository.RefundRepo
}

// TiketSaya mengambil tiket milik user lalu menyaring sesuai tab (aktif / riwayat / dibatalkan)
func (s *TicketService) TiketSaya(ctx context.Context, userID int64, tab string) ([]model.TiketSaya, error) {
	semua, err := s.RefundRepo.TiketMilikUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return FilterTab(semua, tab), nil
}

// Transfer memindahkan pemegang tiket (PRD 3.6): wajib milik user, ACTIVE, dan belum pernah ditransfer
func (s *TicketService) Transfer(ctx context.Context, userID, ticketID int64, in model.TransferInput) error {
	in.HolderName = strings.TrimSpace(in.HolderName)
	in.HolderEmail = strings.ToLower(strings.TrimSpace(in.HolderEmail))
	if err := ValidasiTransfer(in); err != nil {
		return err
	}

	tx, err := s.RefundRepo.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // tidak berefek jika Commit sudah berhasil

	t, err := s.RefundRepo.KunciTiketMilik(ctx, tx, ticketID, userID)
	if err != nil {
		return err
	}
	if t == nil {
		return ErrTiketTidakDitemukan
	}
	if err := t.AlasanTolakTransfer(); err != nil {
		return err
	}
	if strings.EqualFold(t.HolderName, in.HolderName) && strings.EqualFold(t.HolderEmail, in.HolderEmail) {
		return ErrPemegangSama
	}
	berubah, err := s.RefundRepo.GantiPemegang(ctx, tx, t.ID, in)
	if err != nil {
		return err
	}
	if !berubah {
		return ErrSudahPernahTransfer
	}
	return tx.Commit()
}

// Refund mengajukan refund mandiri 75% (PRD 3.5 poin 1). Mengembalikan nominal refund.
func (s *TicketService) Refund(ctx context.Context, userID, ticketID int64, alasan string) (float64, error) {
	alasan = strings.TrimSpace(alasan)
	if utf8.RuneCountInString(alasan) > 255 {
		return 0, ErrAlasanTerlaluPanjang
	}

	tx, err := s.RefundRepo.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	t, err := s.RefundRepo.KunciTiketMilik(ctx, tx, ticketID, userID)
	if err != nil {
		return 0, err
	}
	if t == nil {
		return 0, ErrTiketTidakDitemukan
	}
	if err := t.AlasanTolakRefund(); err != nil {
		return 0, err
	}
	amount := model.HitungRefund(t.UnitNetPrice, 75)
	// Tiga perubahan (tiket, refunds, kuota tier) dalam satu transaksi: gagal satu, batal semua
	rf := model.Refund{
		TicketID:      t.ID,
		OrderID:       t.OrderID,
		RequestedBy:   userID,
		RefundPercent: 75,
		RefundAmount:  amount,
		RefundType:    "BUYER_INITIATED",
		Reason:        alasan,
	}
	if err := s.RefundRepo.CatatRefundMandiri(ctx, tx, rf, t.TierID); err != nil {
		return 0, err
	}
	return amount, tx.Commit()
}

// BatalkanEvent membatalkan event oleh admin dan me-refund 100% semua tiket aktif (PRD 3.5 poin 2)
func (s *TicketService) BatalkanEvent(ctx context.Context, adminID, eventID int64) (int64, error) {
	tx, err := s.RefundRepo.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	status, err := s.RefundRepo.KunciStatusEvent(ctx, tx, eventID)
	if err != nil {
		return 0, err
	}
	switch status {
	case "":
		return 0, ErrEventTidakDitemukan
	case "CANCELLED", "COMPLETED":
		return 0, ErrEventTidakBisaBatal
	}
	jumlah, err := s.RefundRepo.BatalkanEvent(ctx, tx, eventID, adminID)
	if err != nil {
		return 0, err
	}
	return jumlah, tx.Commit()
}

// ===== Aturan murni (tanpa database) supaya mudah dites =====

// FilterTab: aktif = ACTIVE/RESERVED, riwayat = USED, dibatalkan = REFUNDED/CANCELLED
func FilterTab(semua []model.TiketSaya, tab string) []model.TiketSaya {
	cocok := map[string][]string{
		"riwayat":    {model.TiketUsed},
		"dibatalkan": {model.TiketRefunded, model.TiketCancelled},
	}[tab]
	if cocok == nil {
		cocok = []string{model.TiketActive, model.TiketReserved}
	}
	var hasil []model.TiketSaya
	for _, t := range semua {
		for _, st := range cocok {
			if t.Status == st {
				hasil = append(hasil, t)
			}
		}
	}
	return hasil
}

func ValidasiTransfer(in model.TransferInput) error {
	if in.HolderName == "" || utf8.RuneCountInString(in.HolderName) > 100 {
		return ErrNamaPemegang
	}
	addr, err := mail.ParseAddress(in.HolderEmail)
	if err != nil || addr.Address != in.HolderEmail || len(in.HolderEmail) > 120 {
		return ErrEmailPemegang
	}
	return nil
}
