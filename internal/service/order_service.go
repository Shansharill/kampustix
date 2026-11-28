package service

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

// Sentinel error diekspor melalui package service agar handler cukup bergantung
// pada service, bukan perlu mengetahui detail implementasi repository.
var (
	ErrForbidden     = repository.ErrForbidden
	ErrOrderNotFound = repository.ErrOrderNotFound
)

// BusinessError adalah alias untuk error aturan bisnis dari repository.
type BusinessError = repository.BusinessError

type OrderService struct {
	repo *repository.OrderRepository
}

func NewOrderService(repo *repository.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// ProcessCheckout memvalidasi data buyer sebelum meneruskannya ke repository.
func (s *OrderService) ProcessCheckout(ctx context.Context, req model.CheckoutRequest) (int, error) {
	if req.UserID <= 0 {
		return 0, repository.NewBusinessError("Akun pembeli tidak valid. Silakan login kembali.")
	}

	if req.EventID <= 0 || req.TierID <= 0 {
		return 0, repository.NewBusinessError("Event dan tier tiket wajib dipilih.")
	}

	if req.Quantity < 1 || req.Quantity > 4 {
		return 0, repository.NewBusinessError("Jumlah pembelian tiket harus antara 1 hingga 4 tiket per pesanan.")
	}

	if len(req.Holders) != req.Quantity {
		return 0, repository.NewBusinessError("Jumlah data pemegang tiket tidak sesuai dengan jumlah tiket yang dipesan.")
	}

	// Validasi nama dan email setiap pemegang tiket.
	for i, holder := range req.Holders {
		name := strings.TrimSpace(holder.HolderName)
		email := strings.TrimSpace(holder.HolderEmail)

		if name == "" || utf8.RuneCountInString(name) > 100 {
			return 0, repository.NewBusinessError(
				fmt.Sprintf("Nama pemegang tiket ke-%d wajib diisi dan maksimal 100 karakter.", i+1),
			)
		}

		if email == "" || len(email) > 120 {
			return 0, repository.NewBusinessError(
				fmt.Sprintf("Email pemegang tiket ke-%d tidak valid atau melebihi 120 karakter.", i+1),
			)
		}

		parsedEmail, err := mail.ParseAddress(email)
		if err != nil || parsedEmail.Address != email {
			return 0, repository.NewBusinessError(
				fmt.Sprintf("Email pemegang tiket ke-%d tidak valid.", i+1),
			)
		}

		req.Holders[i].HolderName = name
		req.Holders[i].HolderEmail = email
	}

	// Normalisasi kode promo.
	req.PromoCode = strings.TrimSpace(strings.ToUpper(req.PromoCode))

	if len(req.PromoCode) > 30 {
		return 0, repository.NewBusinessError("Kode promo maksimal 30 karakter.")
	}

	return s.repo.CreateOrderWithHold(ctx, req)
}

// GetOrderForPayment mengambil order yang hanya boleh dilihat pemiliknya.
func (s *OrderService) GetOrderForPayment(
	ctx context.Context,
	orderID int,
	currentUserID int,
) (*model.PaymentPageData, error) {
	if orderID <= 0 || currentUserID <= 0 {
		return nil, repository.NewBusinessError("ID pesanan atau pengguna tidak valid.")
	}

	data, err := s.repo.GetPaymentPageData(ctx, orderID, currentUserID)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// SimulatePayment menjalankan simulasi pembayaran.
func (s *OrderService) SimulatePayment(
	ctx context.Context,
	orderID int,
	currentUserID int,
) error {
	if orderID <= 0 || currentUserID <= 0 {
		return repository.NewBusinessError("ID pesanan atau pengguna tidak valid.")
	}

	return s.repo.ConfirmPayment(ctx, orderID, currentUserID)
}

// CancelOrder hanya dapat membatalkan order PENDING milik buyer yang login.
func (s *OrderService) CancelOrder(
	ctx context.Context,
	orderID int,
	currentUserID int,
) error {
	if orderID <= 0 || currentUserID <= 0 {
		return repository.NewBusinessError("ID pesanan atau pengguna tidak valid.")
	}

	return s.repo.CancelOrder(ctx, orderID, currentUserID)
}
