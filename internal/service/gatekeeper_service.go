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
)

func (s *GatekeeperService) ValidateAndCheckIn(
	ctx context.Context,
	code string,
	gatekeeperID int64,
) (*model.GatekeeperTicket, error) {

	// 1. Cari tiket berdasarkan kode.
	ticket, err := s.Repo.FindByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// 2. Pastikan tiket ditemukan.
	if ticket == nil {
		return nil, ErrTicketNotFound
	}

	// 3. Pastikan status tiket masih ACTIVE.
	if ticket.Status != "ACTIVE" {
		return ticket, ErrTicketNotActive
	}

	// 4. Ubah status tiket menjadi USED.
	success, err := s.Repo.CheckIn(ctx, code, gatekeeperID)
	if err != nil {
		return nil, err
	}

	// 5. Periksa apakah check-in berhasil.
	if !success {
		latestTicket, err := s.Repo.FindByCode(ctx, code)
		if err != nil {
			return nil, err
		}

		return latestTicket, ErrTicketNotActive
	}

	// 6. Ambil kembali tiket setelah berhasil check-in.
	return s.Repo.FindByCode(ctx, code)
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
