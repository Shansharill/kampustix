package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"kampustix/internal/model"
	"kampustix/internal/repository"
)

type OrderService struct {
	repo *repository.OrderRepository
}

func NewOrderService(repo *repository.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// ProcessCheckout memvalidasi data buyer sebelum meneruskannya ke transaksi repository.
func (s *OrderService) ProcessCheckout(ctx context.Context, req model.CheckoutRequest) (int, error) {
	if req.UserID <= 0 {
		return 0, errors.New("akun pembeli tidak valid; silakan login kembali")
	}
	if req.EventID <= 0 || req.TierID <= 0 {
		return 0, errors.New("event dan tier tiket wajib dipilih")
	}
	if req.Quantity < 1 || req.Quantity > 4 {
		return 0, errors.New("jumlah pembelian tiket harus antara 1 hingga 4 tiket per pesanan")
	}
	if len(req.Holders) != req.Quantity {
		return 0, errors.New("jumlah data pemegang tiket tidak sesuai dengan jumlah tiket yang dipesan")
	}

	// Validasi nama dan email setiap pemegang tiket sesuai batas kolom ERD.
	for i, holder := range req.Holders {
		name := strings.TrimSpace(holder.HolderName)
		email := strings.TrimSpace(holder.HolderEmail)

		if name == "" || utf8.RuneCountInString(name) > 100 {
			return 0, fmt.Errorf("nama pemegang tiket ke-%d wajib diisi dan maksimal 100 karakter", i+1)
		}
		if email == "" || len(email) > 120 || strings.ContainsAny(email, " \t\r\n") ||
			!strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
			return 0, fmt.Errorf("email pemegang tiket ke-%d tidak valid atau melebihi 120 karakter", i+1)
		}

		req.Holders[i].HolderName = name
		req.Holders[i].HolderEmail = email
	}

	req.PromoCode = strings.TrimSpace(strings.ToUpper(req.PromoCode))
	if len(req.PromoCode) > 30 {
		return 0, errors.New("kode promo maksimal 30 karakter")
	}

	return s.repo.CreateOrderWithHold(ctx, req)
}

func (s *OrderService) GetOrderForPayment(ctx context.Context, orderID int, currentUserID int) (*model.PaymentPageData, error) {
	if orderID <= 0 || currentUserID <= 0 {
		return nil, errors.New("ID pesanan atau pengguna tidak valid")
	}

	data, err := s.repo.GetPaymentPageData(ctx, orderID)
	if err != nil {
		return nil, err
	}

	// Owner Check: buyer hanya boleh melihat order miliknya sendiri.
	if data.Order.UserID != currentUserID {
		return nil, errors.New("FORBIDDEN")
	}

	return data, nil
}

func (s *OrderService) SimulatePayment(ctx context.Context, orderID int, currentUserID int) error {
	if orderID <= 0 || currentUserID <= 0 {
		return errors.New("ID pesanan atau pengguna tidak valid")
	}
	return s.repo.ConfirmPayment(ctx, orderID, currentUserID)
}

// CancelOrder hanya dapat membatalkan order PENDING milik buyer yang sedang login.
func (s *OrderService) CancelOrder(ctx context.Context, orderID int, currentUserID int) error {
	if orderID <= 0 || currentUserID <= 0 {
		return errors.New("ID pesanan atau pengguna tidak valid")
	}
	return s.repo.CancelOrder(ctx, orderID, currentUserID)
}
