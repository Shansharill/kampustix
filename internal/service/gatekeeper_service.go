package service

import (
	"context"
	"errors"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

// GatekeeperService mengelola aturan validasi tiket.
type GatekeeperService struct {
	Repo *repository.GatekeeperRepo
}

// Error khusus untuk kondisi tiket.
var (
	ErrTicketNotFound  = errors.New("tiket tidak ditemukan")
	ErrTicketNotActive = errors.New("tiket tidak aktif atau sudah digunakan")
	ErrTicketUsed      = errors.New("tiket sudah digunakan")
	ErrTicketReserved  = errors.New("tiket belum dibayar")
	ErrTicketRefunded  = errors.New("tiket sudah direfund")
	ErrTicketCancelled = errors.New("tiket sudah dibatalkan")
	ErrEventCancelled  = errors.New("event sudah dibatalkan")
	ErrEventCompleted  = errors.New("event sudah selesai")
)

func ticketStatusError(status string) error {
	switch status {
	case "USED":
		return ErrTicketUsed
	case "RESERVED":
		return ErrTicketReserved
	case "REFUNDED":
		return ErrTicketRefunded
	case "CANCELLED":
		return ErrTicketCancelled
	default:
		return ErrTicketNotActive
	}
}

func eventStatusError(status string) error {
	switch status {
	case "CANCELLED":
		return ErrEventCancelled
	case "COMPLETED":
		return ErrEventCompleted
	default:
		return nil
	}
}

func (s *GatekeeperService) ValidateAndCheckIn(
	ctx context.Context,
	code string,
	gatekeeperID int64,
) (*model.GatekeeperTicket, error) {

	// Langkah 1: Coba lakukan check-in langsung.
	success, err := s.Repo.CheckIn(ctx, code, gatekeeperID)
	if err != nil {
		return nil, err
	}

	// Langkah 2: Ambil data tiket terbaru.
	ticket, err := s.Repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	if ticket == nil {
		return nil, ErrTicketNotFound
	}

	// Jika check-in gagal, periksa penyebabnya.
	if !success {
		if err := eventStatusError(ticket.EventStatus); err != nil {
			return ticket, err
		}

		if ticket.Status != "ACTIVE" {
			return ticket, ticketStatusError(ticket.Status)
		}

		return ticket, ErrTicketNotActive
	}

	// Check-in berhasil.
	return ticket, nil
}

// GetBoardingPass mengambil informasi tiket untuk ditampilkan pada halaman cetak.
func (s *GatekeeperService) GetBoardingPass(
	ctx context.Context,
	code string,
) (*model.BoardingPass, error) {

	boardingPass, err := s.Repo.FindBoardingPass(ctx, code)
	if err != nil {
		return nil, err
	}

	if boardingPass == nil {
		return nil, ErrTicketNotFound
	}

	return boardingPass, nil
}

func (s *GatekeeperService) GetBoardingPassForBuyer(
	ctx context.Context,
	code string,
	buyerID int64,
) (*model.BoardingPass, error) {

	boardingPass, err := s.Repo.FindBoardingPassByBuyer(ctx, code, buyerID)
	if err != nil {
		return nil, err
	}

	if boardingPass == nil {
		return nil, ErrTicketNotFound
	}

	return boardingPass, nil
}
