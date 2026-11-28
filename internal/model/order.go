package model

import "time"

// Order merepresentasikan tabel orders di database
type Order struct {
	ID             int        `db:"id"`
	OrderCode      string     `db:"order_code"`
	UserID         int        `db:"user_id"`
	EventID        int        `db:"event_id"`
	TierID         int        `db:"tier_id"`
	PromoCodeID    *int       `db:"promo_code_id"` // Pointer karena bisa NULL jika tanpa promo
	Quantity       int        `db:"quantity"`
	TotalGross     float64    `db:"total_gross"`
	DiscountAmount float64    `db:"discount_amount"`
	TotalNet       float64    `db:"total_net"`
	Status         string     `db:"status"` // PENDING, PAID, EXPIRED, CANCELLED
	ExpiresAt      time.Time  `db:"expires_at"`
	PaidAt         *time.Time `db:"paid_at"`
	CreatedAt      time.Time  `db:"created_at"`
}

// TicketHolderInput menampung input form nama & email tiap pemegang tiket (1-4 orang)
type TicketHolderInput struct {
	HolderName  string
	HolderEmail string
}

// CheckoutRequest menampung seluruh data yang dikirim dari form checkout
type CheckoutRequest struct {
	UserID    int
	EventID   int
	TierID    int
	PromoCode string
	Quantity  int
	Holders   []TicketHolderInput
}

// OrderTicketView digunakan untuk menampilkan detail tiket di halaman pembayaran
type OrderTicketView struct {
	TicketCode  string
	HolderName  string
	HolderEmail string
	UnitNet     float64
	Status      string
}

// PaymentPageData dikirim ke template/checkout/payment.html
type PaymentPageData struct {
	Order            Order
	EventTitle       string
	CategoryName     string
	TierName         string
	Tickets          []OrderTicketView
	RemainingSeconds int64 // Untuk timer countdown 15 menit di frontend
}
