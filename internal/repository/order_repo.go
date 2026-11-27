package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"kampustix/internal/model"
)

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// 1. LAZY EXPIRATION ENGINE (Murni menggunakan jam MySQL NOW() agar akurat)
func (r *OrderRepository) RunLazyExpiration(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Kembalikan kuota tier untuk order yang sudah melewati batas 15 menit
	_, err = tx.ExecContext(ctx, `
		UPDATE ticket_tiers tt
		JOIN orders o ON o.tier_id = tt.id
		SET tt.used_quota = tt.used_quota - o.quantity
		WHERE o.status = 'PENDING' AND o.expires_at < NOW()
	`)
	if err != nil {
		return err
	}

	// Kembalikan kuota pemakaian promo (jika order menggunakan promo)
	_, err = tx.ExecContext(ctx, `
		UPDATE promo_codes p
		JOIN orders o ON o.promo_code_id = p.id
		SET p.used_count = p.used_count - 1
		WHERE o.status = 'PENDING' AND o.expires_at < NOW() AND o.promo_code_id IS NOT NULL
	`)
	if err != nil {
		return err
	}

	// Ubah status tiket RESERVED menjadi CANCELLED
	_, err = tx.ExecContext(ctx, `
		UPDATE tickets t
		JOIN orders o ON t.order_id = o.id
		SET t.status = 'CANCELLED'
		WHERE o.status = 'PENDING' AND o.expires_at < NOW() AND t.status = 'RESERVED'
	`)
	if err != nil {
		return err
	}

	// Terakhir, tandai status order menjadi EXPIRED
	_, err = tx.ExecContext(ctx, `
		UPDATE orders
		SET status = 'EXPIRED'
		WHERE status = 'PENDING' AND expires_at < NOW()
	`)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// 2. TRANSAKSI CHECKOUT ATOMIK (15-Minute Hold & SELECT FOR UPDATE)
func (r *OrderRepository) CreateOrderWithHold(ctx context.Context, req model.CheckoutRequest) (int, error) {
	_ = r.RunLazyExpiration(ctx)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// A. Kunci baris tier dengan SELECT ... FOR UPDATE & cek tanggal langsung di MySQL
	var tierPrice float64
	var quota, usedQuota int
	var categoryID int
	var isExpired int

	err = tx.QueryRowContext(ctx, `
		SELECT category_id, price, quota, used_quota, (NOW() > end_date) AS is_expired
		FROM ticket_tiers
		WHERE id = ? FOR UPDATE
	`, req.TierID).Scan(&categoryID, &tierPrice, &quota, &usedQuota, &isExpired)
	if err != nil {
		return 0, errors.New("tier tiket tidak ditemukan")
	}

	if isExpired == 1 {
		return 0, errors.New("masa penjualan tier tiket ini sudah berakhir")
	}
	if usedQuota+req.Quantity > quota {
		return 0, errors.New("sisa kuota tiket tidak mencukupi untuk jumlah yang dipesan")
	}

	// B. Hitung harga kotor & cek kode promo (jika diisi)
	totalGross := tierPrice * float64(req.Quantity)
	discountAmount := 0.0
	var promoID *int

	if req.PromoCode != "" {
		var pID, maxUses, usedCount int
		var discPercent float64
		var isActive, promoExpired int
		var promoCatID sql.NullInt64

		err = tx.QueryRowContext(ctx, `
			SELECT id, category_id, discount_percent, max_uses, used_count, is_active, (NOW() > valid_until) AS promo_expired
			FROM promo_codes
			WHERE code = ? AND event_id = ? FOR UPDATE
		`, req.PromoCode, req.EventID).Scan(&pID, &promoCatID, &discPercent, &maxUses, &usedCount, &isActive, &promoExpired)

		if err != nil {
			return 0, errors.New("kode promo tidak ditemukan pada event ini")
		}
		if isActive == 0 || promoExpired == 1 || usedCount >= maxUses {
			return 0, errors.New("kode promo sudah tidak aktif atau kuota habis")
		}
		if promoCatID.Valid && int(promoCatID.Int64) != categoryID {
			return 0, errors.New("kode promo tidak berlaku untuk kategori tiket ini")
		}

		discountAmount = totalGross * (discPercent / 100.0)
		promoID = &pID

		_, err = tx.ExecContext(ctx, `UPDATE promo_codes SET used_count = used_count + 1 WHERE id = ?`, pID)
		if err != nil {
			return 0, err
		}
	}

	totalNet := totalGross - discountAmount
	unitNetPrice := tierPrice - (discountAmount / float64(req.Quantity))

	// C. Tambahkan used_quota pada ticket_tiers
	_, err = tx.ExecContext(ctx, `
		UPDATE ticket_tiers SET used_quota = used_quota + ? WHERE id = ?
	`, req.Quantity, req.TierID)
	if err != nil {
		return 0, err
	}

	// D. Insert ke tabel orders menggunakan DATE_ADD(NOW(), INTERVAL 15 MINUTE) sesuai PRD!
	orderCode := fmt.Sprintf("ORD-%d-%d", req.UserID, time.Now().Unix())

	res, err := tx.ExecContext(ctx, `
		INSERT INTO orders (order_code, user_id, event_id, tier_id, promo_code_id, quantity, total_gross, discount_amount, total_net, status, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', DATE_ADD(NOW(), INTERVAL 15 MINUTE))
	`, orderCode, req.UserID, req.EventID, req.TierID, promoID, req.Quantity, totalGross, discountAmount, totalNet)
	if err != nil {
		return 0, err
	}

	orderID64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	orderID := int(orderID64)

	// E. Insert masing-masing pemegang tiket ke tabel tickets dengan status RESERVED
	for i, holder := range req.Holders {
		ticketCode := fmt.Sprintf("TIX-%d-%d-%d", orderID, i+1, time.Now().UnixNano()%10000)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO tickets (order_id, event_id, category_id, tier_id, ticket_code, holder_name, holder_email, unit_base_price, unit_net_price, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'RESERVED')
		`, orderID, req.EventID, categoryID, req.TierID, ticketCode, holder.HolderName, holder.HolderEmail, tierPrice, unitNetPrice)
		if err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return orderID, nil
}

// 3. AMBIL DATA ORDER UNTUK HALAMAN PEMBAYARAN (Hitung sisa detik langsung dari MySQL)
func (r *OrderRepository) GetPaymentPageData(ctx context.Context, orderID int) (*model.PaymentPageData, error) {
	_ = r.RunLazyExpiration(ctx)

	var data model.PaymentPageData
	var promoID sql.NullInt64
	var paidAt sql.NullTime
	var remainingSec int64

	err := r.db.QueryRowContext(ctx, `
		SELECT o.id, o.order_code, o.user_id, o.event_id, o.tier_id, o.promo_code_id,
		       o.quantity, o.total_gross, o.discount_amount, o.total_net, o.status, o.expires_at, o.paid_at, o.created_at,
		       e.title, tc.category_name, tt.tier_name,
		       GREATEST(0, TIMESTAMPDIFF(SECOND, NOW(), o.expires_at)) AS remaining_sec
		FROM orders o
		JOIN events e ON o.event_id = e.id
		JOIN ticket_tiers tt ON o.tier_id = tt.id
		JOIN ticket_categories tc ON tt.category_id = tc.id
		WHERE o.id = ?
	`, orderID).Scan(
		&data.Order.ID, &data.Order.OrderCode, &data.Order.UserID, &data.Order.EventID, &data.Order.TierID, &promoID,
		&data.Order.Quantity, &data.Order.TotalGross, &data.Order.DiscountAmount, &data.Order.TotalNet,
		&data.Order.Status, &data.Order.ExpiresAt, &paidAt, &data.Order.CreatedAt,
		&data.EventTitle, &data.CategoryName, &data.TierName,
		&remainingSec,
	)
	if err != nil {
		return nil, err
	}

	if promoID.Valid {
		id := int(promoID.Int64)
		data.Order.PromoCodeID = &id
	}
	if paidAt.Valid {
		data.Order.PaidAt = &paidAt.Time
	}

	if data.Order.Status != "PENDING" {
		remainingSec = 0
	}
	data.RemainingSeconds = remainingSec

	rows, err := r.db.QueryContext(ctx, `
		SELECT ticket_code, holder_name, holder_email, unit_net_price, status
		FROM tickets WHERE order_id = ?
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t model.OrderTicketView
		if err := rows.Scan(&t.TicketCode, &t.HolderName, &t.HolderEmail, &t.UnitNet, &t.Status); err != nil {
			return nil, err
		}
		data.Tickets = append(data.Tickets, t)
	}

	return &data, nil
}

// 4. SIMULASI PEMBAYARAN BERHASIL (Ubah Order -> PAID & Tiket -> ACTIVE)
func (r *OrderRepository) ConfirmPayment(ctx context.Context, orderID int, userID int) error {
	_ = r.RunLazyExpiration(ctx)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ownerID int
	var status string
	var isExpired int

	err = tx.QueryRowContext(ctx, `
		SELECT user_id, status, (NOW() > expires_at) AS is_expired
		FROM orders WHERE id = ? FOR UPDATE
	`, orderID).Scan(&ownerID, &status, &isExpired)
	if err != nil {
		return errors.New("pesanan tidak ditemukan")
	}

	if ownerID != userID {
		return errors.New("FORBIDDEN")
	}
	if status != "PENDING" || isExpired == 1 {
		return errors.New("pesanan sudah kedaluwarsa atau sudah dibayar")
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE orders SET status = 'PAID', paid_at = NOW() WHERE id = ?
	`, orderID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE tickets SET status = 'ACTIVE' WHERE order_id = ? AND status = 'RESERVED'
	`, orderID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
