package service

import (
	"context"
	"errors"
	"strings"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

type OrderService struct {
	repo *repository.OrderRepository
}

func NewOrderService(repo *repository.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) ProcessCheckout(ctx context.Context, req model.CheckoutRequest) (int, error) {
	// Validasi batas 1 - 4 tiket per transaksi sesuai PRD
	if req.Quantity < 1 || req.Quantity > 4 {
		return 0, errors.New("jumlah pembelian tiket harus antara 1 hingga 4 tiket per pesanan")
	}

	if len(req.Holders) != req.Quantity {
		return 0, errors.New("data pemegang tiket tidak sesuai dengan jumlah tiket yang dipesan")
	}

	// Validasi kelengkapan nama & email masing-masing pemegang tiket
	for i, h := range req.Holders {
		name := strings.TrimSpace(h.HolderName)
		email := strings.TrimSpace(h.HolderEmail)
		if name == "" || email == "" || !strings.Contains(email, "@") {
			return 0, errors.New("nama lengkap dan email yang valid wajib diisi untuk seluruh pemegang tiket")
		}
		req.Holders[i].HolderName = name
		req.Holders[i].HolderEmail = email
	}

	req.PromoCode = strings.TrimSpace(strings.ToUpper(req.PromoCode))
	return s.repo.CreateOrderWithHold(ctx, req)
}

func (s *OrderService) GetOrderForPayment(ctx context.Context, orderID int, currentUserID int) (*model.PaymentPageData, error) {
	data, err := s.repo.GetPaymentPageData(ctx, orderID)
	if err != nil {
		return nil, err
	}

	// Owner Check (Cegah pembeli melihat halaman pembayaran milik orang lain)
	if data.Order.UserID != currentUserID {
		return nil, errors.New("FORBIDDEN")
	}

	return data, nil
}

func (s *OrderService) SimulatePayment(ctx context.Context, orderID int, currentUserID int) error {
	return s.repo.ConfirmPayment(ctx, orderID, currentUserID)
}
