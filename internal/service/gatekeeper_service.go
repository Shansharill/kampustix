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

// Error khusus untuk kondisi tiket dan event.
var (
	ErrTicketNotFound   = errors.New("tiket tidak ditemukan")
	ErrTicketNotActive  = errors.New("tiket tidak aktif atau sudah digunakan")
	ErrTicketUsed       = errors.New("tiket sudah digunakan")
	ErrTicketReserved   = errors.New("tiket belum dibayar")
	ErrTicketRefunded   = errors.New("tiket sudah direfund")
	ErrTicketCancelled  = errors.New("tiket sudah dibatalkan")
	ErrTicketWrongEvent = errors.New("tiket berasal dari event lain")

	ErrEventUpcoming  = errors.New("event belum dimulai")
	ErrEventCancelled = errors.New("event sudah dibatalkan")
	ErrEventCompleted = errors.New("event sudah selesai")
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
	case "UPCOMING":
		return ErrEventUpcoming
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
	selectedEventID int64,
	gatekeeperID int64,
) (*model.GatekeeperTicket, error) {

	// Langkah 1: Cari tiket berdasarkan kode.
	ticket, err := s.Repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	if ticket == nil {
		return nil, ErrTicketNotFound
	}

	// Langkah 2: Cocokkan event yang dipilih dengan event tiket.
	if ticket.EventID != selectedEventID {
		return ticket, ErrTicketWrongEvent
	}

	// Langkah 3: Periksa status event.
	if err := eventStatusError(ticket.EventStatus); err != nil {
		return ticket, err
	}

	// Langkah 4: Periksa status tiket.
	if ticket.Status != "ACTIVE" {
		return ticket, ticketStatusError(ticket.Status)
	}

	// Langkah 5: Lakukan check-in.
	success, err := s.Repo.CheckIn(ctx, code, selectedEventID, gatekeeperID)
	if err != nil {
		return nil, err
	}

	if !success {
		// Ambil kembali status terbaru jika check-in gagal.
		ticket, err = s.Repo.FindByCode(ctx, code)
		if err != nil {
			return nil, err
		}

		if ticket == nil {
			return nil, ErrTicketNotFound
		}

		if ticket.EventID != selectedEventID {
			return ticket, ErrTicketWrongEvent
		}

		if err := eventStatusError(ticket.EventStatus); err != nil {
			return ticket, err
		}

		if ticket.Status != "ACTIVE" {
			return ticket, ticketStatusError(ticket.Status)
		}

		return ticket, ErrTicketNotActive
	}

	// Langkah 6: Ambil data tiket setelah check-in berhasil.
	ticket, err = s.Repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	return ticket, nil
}

// GetBoardingPassForBuyer menampilkan boarding pass milik buyer.
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

// GetActiveEvents mengambil daftar event aktif untuk Gatekeeper.
func (s *GatekeeperService) GetActiveEvents(
	ctx context.Context,
) ([]model.GatekeeperEvent, error) {

	return s.Repo.FindActiveEvents(ctx)
}
