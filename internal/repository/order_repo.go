package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"

	"kampustix/internal/model"
)

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// RunLazyExpiration menjalankan pembersihan order PENDING yang sudah melewati
// expires_at. Semua perubahan dilakukan dalam satu transaksi agar status order,
// status tiket, kuota tier, dan pemakaian promo selalu konsisten.
// RunLazyExpiration memproses order kedaluwarsa satu per satu.
func (r *OrderRepository) RunLazyExpiration(ctx context.Context) error {
	// Ambil daftar ID order yang sudah kedaluwarsa.
	rows, err := r.db.QueryContext(ctx, `
        SELECT id
        FROM orders
        WHERE status = 'PENDING' AND expires_at <= NOW()
        ORDER BY id
    `)
	if err != nil {
		return err
	}

	var orderIDs []int

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		orderIDs = append(orderIDs, id)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}

	if err := rows.Close(); err != nil {
		return err
	}

	// Proses setiap order menggunakan transaksi terpisah.
	for _, orderID := range orderIDs {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		var tierID, quantity int
		var promoID sql.NullInt64

		// Periksa kembali status order agar tidak memproses
		// order yang sudah dibayar atau dibatalkan.
		err = tx.QueryRowContext(ctx, `
            SELECT tier_id, quantity, promo_code_id
            FROM orders
            WHERE id = ?
              AND status = 'PENDING'
              AND expires_at <= NOW()
            FOR UPDATE
        `, orderID).Scan(&tierID, &quantity, &promoID)

		if err == sql.ErrNoRows {
			tx.Rollback()
			continue
		}
		if err != nil {
			tx.Rollback()
			return err
		}

		// Batalkan tiket yang masih berstatus RESERVED.
		result, err := tx.ExecContext(ctx, `
            UPDATE tickets
            SET status = 'CANCELLED'
            WHERE order_id = ? AND status = 'RESERVED'
        `, orderID)
		if err != nil {
			tx.Rollback()
			return err
		}

		affected, err := result.RowsAffected()
		if err != nil {
			tx.Rollback()
			return err
		}

		// Jika jumlah tiket tidak sesuai, batalkan transaksi
		// order ini saja dan lanjutkan ke order berikutnya.
		if affected != int64(quantity) {
			log.Printf(
				"Anomali expiration order %d: jumlah tiket RESERVED %d, seharusnya %d",
				orderID, affected, quantity,
			)
			tx.Rollback()
			continue
		}

		// Kembalikan kuota tier.
		_, err = tx.ExecContext(ctx, `
            UPDATE ticket_tiers
            SET used_quota = GREATEST(used_quota - ?, 0)
            WHERE id = ?
        `, quantity, tierID)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Kembalikan penggunaan promo jika order memakai promo.
		if promoID.Valid {
			_, err = tx.ExecContext(ctx, `
                UPDATE promo_codes
                SET used_count = GREATEST(used_count - 1, 0)
                WHERE id = ?
            `, promoID.Int64)
			if err != nil {
				tx.Rollback()
				return err
			}
		}

		// Ubah status order menjadi EXPIRED.
		_, err = tx.ExecContext(ctx, `
            UPDATE orders
            SET status = 'EXPIRED'
            WHERE id = ?
              AND status = 'PENDING'
              AND expires_at <= NOW()
        `, orderID)
		if err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// CreateOrderWithHold membuat order PENDING dan tiket RESERVED sekaligus
// mengunci serta mengurangi kuota dalam satu transaksi database.
func (r *OrderRepository) CreateOrderWithHold(ctx context.Context, req model.CheckoutRequest) (int, error) {
	if req.UserID <= 0 || req.EventID <= 0 || req.TierID <= 0 {
		return 0, NewBusinessError("Data pengguna, event, atau tier tidak valid.")
	}
	if req.Quantity < 1 || req.Quantity > 4 {
		return 0, NewBusinessError("Jumlah tiket harus antara 1 hingga 4.")
	}
	if len(req.Holders) != req.Quantity {
		return 0, NewBusinessError("Jumlah data pemegang tiket tidak sesuai dengan jumlah tiket.")
	}

	// Jalankan lazy expiration terlebih dahulu. Jika gagal, jangan lanjut checkout
	// karena nilai kuota di database mungkin belum konsisten.
	if err := r.RunLazyExpiration(ctx); err != nil {
		return 0, fmt.Errorf("gagal memeriksa pesanan kedaluwarsa: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// A. Kunci tier yang dipilih dan pastikan tier memang milik event tersebut.
	// FOR UPDATE mencegah dua buyer mengambil kuota terakhir secara bersamaan.

	var categoryID int
	var tierPrice float64
	var quota, usedQuota int
	var notStarted, isEnded int
	var eventStatus string
	var eventPassed int

	err = tx.QueryRowContext(ctx, `
    SELECT
        tc.id,
        tt.price,
        tt.quota,
        tt.used_quota,
        (NOW() < tt.start_date) AS not_started,
        (NOW() > tt.end_date) AS is_ended,
        e.status,
        (NOW() > e.event_date) AS event_passed
    FROM ticket_tiers tt
    JOIN ticket_categories tc
        ON tc.id = tt.category_id
    JOIN events e
        ON e.id = tc.event_id
    WHERE tt.id = ?
      AND tc.event_id = ?
    FOR UPDATE
`, req.TierID, req.EventID).Scan(
		&categoryID,
		&tierPrice,
		&quota,
		&usedQuota,
		&notStarted,
		&isEnded,
		&eventStatus,
		&eventPassed,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, NewBusinessError("Tier tiket tidak ditemukan pada event yang dipilih.")
		}
		return 0, err
	}

	// Validasi status event sebelum checkout.
	if eventStatus == "CANCELLED" {
		return 0, NewBusinessError("Event sudah dibatalkan dan tidak dapat dibeli.")
	}

	if eventStatus == "COMPLETED" {
		return 0, NewBusinessError("Event sudah selesai dan tidak dapat dibeli.")
	}

	if eventStatus != "UPCOMING" && eventStatus != "ACTIVE" {
		return 0, NewBusinessError("Status event tidak memungkinkan pembelian tiket.")
	}

	// Tolak pembelian jika tanggal pelaksanaan event sudah lewat.
	if eventPassed == 1 {
		return 0, NewBusinessError("Tanggal event sudah lewat dan tiket tidak dapat dibeli.")
	}

	if notStarted == 1 {
		return 0, NewBusinessError("Masa penjualan tier tiket ini belum dimulai.")
	}
	if isEnded == 1 {
		return 0, NewBusinessError("Masa penjualan tier tiket ini sudah berakhir.")
	}
	if usedQuota+req.Quantity > quota {
		return 0, NewBusinessError("Sisa kuota tiket tidak mencukupi untuk jumlah yang dipesan.")
	}

	// B. Hitung harga kotor dan validasi kode promo bila buyer mengisinya.
	totalGross := roundMoney(tierPrice * float64(req.Quantity))
	discountAmount := 0.0
	var promoID *int

	if req.PromoCode != "" {
		var id, maxUses, usedCount int
		var discountPercent float64
		var isActive, promoExpired int
		var promoCategoryID sql.NullInt64

		err = tx.QueryRowContext(ctx, `
			SELECT id, category_id, discount_percent, max_uses, used_count,
			       is_active, (NOW() > valid_until) AS promo_expired
			FROM promo_codes
			WHERE code = ? AND event_id = ?
			FOR UPDATE
		`, req.PromoCode, req.EventID).Scan(
			&id, &promoCategoryID, &discountPercent, &maxUses, &usedCount,
			&isActive, &promoExpired,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, NewBusinessError("Kode promo tidak ditemukan pada event ini.")
			}
			return 0, err
		}

		if isActive == 0 || promoExpired == 1 || usedCount >= maxUses {
			return 0, NewBusinessError("Kode promo sudah tidak aktif, kedaluwarsa, atau kuotanya habis.")
		}
		if discountPercent < 0 || discountPercent > 100 {
			return 0, NewBusinessError("Persentase diskon promo tidak valid.")
		}
		if promoCategoryID.Valid && int(promoCategoryID.Int64) != categoryID {
			return 0, NewBusinessError("Kode promo tidak berlaku untuk kategori tiket ini.")
		}

		discountAmount = roundMoney(totalGross * (discountPercent / 100.0))
		promoID = &id

		result, err := tx.ExecContext(ctx, `
			UPDATE promo_codes
			SET used_count = used_count + 1
			WHERE id = ? AND used_count < max_uses AND is_active = 1
		`, id)
		if err != nil {
			return 0, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if affected != 1 {
			return 0, NewBusinessError("Kuota penggunaan kode promo sudah habis.")
		}
	}

	totalNet := roundMoney(totalGross - discountAmount)
	unitNetPrice := roundMoney(tierPrice - (discountAmount / float64(req.Quantity)))

	// C. Tambahkan kuota terpakai pada tier yang sudah dikunci.
	result, err := tx.ExecContext(ctx, `
		UPDATE ticket_tiers
		SET used_quota = used_quota + ?
		WHERE id = ? AND used_quota + ? <= quota
	`, req.Quantity, req.TierID, req.Quantity)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected != 1 {
		return 0, NewBusinessError("Sisa kuota tiket tidak mencukupi untuk jumlah yang dipesan.")
	}

	// D. Buat kode order unik agar dua checkout dalam detik yang sama tidak bentrok.
	randomOrder, err := randomHex(6)
	if err != nil {
		return 0, err
	}
	orderCode := fmt.Sprintf("ORD-%d-%s", req.UserID, randomOrder)

	var promoValue any
	if promoID != nil {
		promoValue = *promoID
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO orders (
			order_code, user_id, event_id, tier_id, promo_code_id,
			quantity, total_gross, discount_amount, total_net,
			status, expires_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', DATE_ADD(NOW(), INTERVAL 15 MINUTE))
	`, orderCode, req.UserID, req.EventID, req.TierID, promoValue,
		req.Quantity, totalGross, discountAmount, totalNet)
	if err != nil {
		return 0, err
	}

	orderID64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	orderID := int(orderID64)

	// E. Buat satu tiket RESERVED untuk setiap pemegang tiket yang dikirim buyer.
	for _, holder := range req.Holders {
		randomTicket, err := randomHex(8)
		if err != nil {
			return 0, err
		}
		ticketCode := fmt.Sprintf("TIX-%d-%s", orderID, randomTicket)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO tickets (
				order_id, event_id, category_id, tier_id, ticket_code,
				holder_name, holder_email, unit_base_price, unit_net_price, status
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'RESERVED')
		`, orderID, req.EventID, categoryID, req.TierID, ticketCode,
			holder.HolderName, holder.HolderEmail, tierPrice, unitNetPrice)
		if err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return orderID, nil
}

// GetPaymentPageData mengambil detail order, event, tier, dan tiket untuk halaman pembayaran.
func (r *OrderRepository) GetPaymentPageData(ctx context.Context, orderID int, currentUserID int) (*model.PaymentPageData, error) {
	if err := r.RunLazyExpiration(ctx); err != nil {
		return nil, fmt.Errorf("gagal memeriksa pesanan kedaluwarsa: %w", err)
	}

	var data model.PaymentPageData
	var promoID sql.NullInt64
	var paidAt sql.NullTime
	var remainingSec int64

	err := r.db.QueryRowContext(ctx, `
		SELECT o.id, o.order_code, o.user_id, o.event_id, o.tier_id, o.promo_code_id,
		       o.quantity, o.total_gross, o.discount_amount, o.total_net,
		       o.status, o.expires_at, o.paid_at, o.created_at,
	       e.title, tc.category_name, tt.tier_name,
	       GREATEST(0, TIMESTAMPDIFF(SECOND, NOW(), o.expires_at)) AS remaining_sec
		FROM orders o
		JOIN events e ON e.id = o.event_id
		JOIN ticket_tiers tt ON tt.id = o.tier_id
		JOIN ticket_categories tc ON tc.id = tt.category_id
		        WHERE o.id = ? AND o.user_id = ?
    `, orderID, currentUserID).Scan(
		&data.Order.ID, &data.Order.OrderCode, &data.Order.UserID,
		&data.Order.EventID, &data.Order.TierID, &promoID,
		&data.Order.Quantity, &data.Order.TotalGross,
		&data.Order.DiscountAmount, &data.Order.TotalNet,
		&data.Order.Status, &data.Order.ExpiresAt, &paidAt, &data.Order.CreatedAt,
		&data.EventTitle, &data.CategoryName, &data.TierName,
		&remainingSec,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
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
    FROM tickets
    WHERE order_id = ?
      AND EXISTS (
          SELECT 1
          FROM orders o
          WHERE o.id = tickets.order_id
            AND o.user_id = ?
      )
    ORDER BY id
`, orderID, currentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ticket model.OrderTicketView
		if err := rows.Scan(
			&ticket.TicketCode, &ticket.HolderName, &ticket.HolderEmail,
			&ticket.UnitNet, &ticket.Status,
		); err != nil {
			return nil, err
		}
		data.Tickets = append(data.Tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &data, nil
}

// releaseReservedOrderResources membatalkan tiket RESERVED dan mengembalikan
// kuota tier serta pemakaian promo untuk satu order yang masih PENDING.
// Pemanggil harus sudah mengunci baris order dengan SELECT ... FOR UPDATE.
func (r *OrderRepository) releaseReservedOrderResources(
	ctx context.Context,
	tx *sql.Tx,
	orderID, tierID, quantity int,
	promoID sql.NullInt64,
) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET status = 'CANCELLED'
		WHERE order_id = ? AND status = 'RESERVED'
	`, orderID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(quantity) {
		return fmt.Errorf("jumlah tiket RESERVED untuk order %d tidak sesuai dengan quantity order", orderID)
	}

	result, err = tx.ExecContext(ctx, `
    UPDATE ticket_tiers
    SET used_quota = GREATEST(used_quota - ?, 0)
    WHERE id = ?
`, quantity, tierID)
	if err != nil {
		return err
	}

	if promoID.Valid {
		_, err = tx.ExecContext(ctx, `
        UPDATE promo_codes
        SET used_count = GREATEST(used_count - 1, 0)
        WHERE id = ?
    `, promoID.Int64)
		if err != nil {
			return err
		}
	}

	return nil
}

// ConfirmPayment mengubah order PENDING menjadi PAID dan seluruh tiket
// RESERVED menjadi ACTIVE, hanya jika pemilik benar dan waktu hold belum habis.
func (r *OrderRepository) ConfirmPayment(ctx context.Context, orderID int, userID int) error {
	if err := r.RunLazyExpiration(ctx); err != nil {
		return fmt.Errorf("gagal memeriksa pesanan kedaluwarsa: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ownerID, tierID, quantity, isExpired int
	var status string
	var eventStatus string
	var promoID sql.NullInt64

	err = tx.QueryRowContext(ctx, `
    SELECT
        o.user_id,
        o.status,
        o.tier_id,
        o.quantity,
        o.promo_code_id,
        (o.expires_at <= NOW()) AS is_expired,
        e.status
    FROM orders o
    JOIN events e
        ON e.id = o.event_id
    WHERE o.id = ?
    FOR UPDATE
`, orderID).Scan(
		&ownerID,
		&status,
		&tierID,
		&quantity,
		&promoID,
		&isExpired,
		&eventStatus,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}

	// Pastikan order memang milik buyer yang sedang login.
	if ownerID != userID {
		return ErrForbidden
	}

	// Pastikan order masih menunggu pembayaran.
	if status != "PENDING" {
		return NewBusinessError(fmt.Sprintf("Pesanan tidak dapat dibayar karena statusnya %s.", status))
	}

	// Tolak pembayaran apabila event sudah dibatalkan atau selesai.
	if eventStatus == "CANCELLED" {
		return NewBusinessError("Pembayaran ditolak karena event sudah dibatalkan.")
	}
	if eventStatus == "COMPLETED" {
		return NewBusinessError("Pembayaran ditolak karena event sudah selesai.")
	}

	if isExpired == 1 {
		if err := r.releaseReservedOrderResources(ctx, tx, orderID, tierID, quantity, promoID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'EXPIRED' WHERE id = ? AND status = 'PENDING'`, orderID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return NewBusinessError("Waktu pembayaran 15 menit sudah habis. Pesanan berstatus EXPIRED.")
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE tickets
		SET status = 'ACTIVE'
		WHERE order_id = ? AND status = 'RESERVED'
	`, orderID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(quantity) {
		return errors.New("jumlah tiket RESERVED tidak sesuai; pembayaran tidak dapat diproses")
	}

	result, err = tx.ExecContext(ctx, `
		UPDATE orders
		SET status = 'PAID', paid_at = NOW()
		WHERE id = ? AND status = 'PENDING' AND expires_at > NOW()
	`, orderID)
	if err != nil {
		return err
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return NewBusinessError("status pesanan berubah; pembayaran tidak dapat diproses")
	}

	return tx.Commit()
}

// CancelOrder membatalkan order PENDING atas permintaan pemiliknya sebelum
// expires_at. Status order/tiket dan rollback kuota dilakukan atomik.
func (r *OrderRepository) CancelOrder(ctx context.Context, orderID int, userID int) error {
	if err := r.RunLazyExpiration(ctx); err != nil {
		return fmt.Errorf("gagal memeriksa pesanan kedaluwarsa: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ownerID, tierID, quantity, isExpired int
	var status string
	var promoID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, status, tier_id, quantity, promo_code_id,
		      (expires_at <= NOW()) AS is_expired
		FROM orders
		WHERE id = ?
		FOR UPDATE
	`, orderID).Scan(&ownerID, &status, &tierID, &quantity, &promoID, &isExpired)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrOrderNotFound
		}
		return err
	}

	if ownerID != userID {
		return ErrForbidden
	}
	if status != "PENDING" {
		return NewBusinessError(fmt.Sprintf("Pesanan tidak dapat dibatalkan karena statusnya %s.", status))
	}

	if isExpired == 1 {
		if err := r.releaseReservedOrderResources(ctx, tx, orderID, tierID, quantity, promoID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE orders SET status = 'EXPIRED' WHERE id = ? AND status = 'PENDING'
		`, orderID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return NewBusinessError("Batas waktu pembayaran sudah habis. Pesanan berstatus EXPIRED, bukan CANCELLED.")
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET status = 'CANCELLED'
		WHERE id = ? AND status = 'PENDING' AND expires_at > NOW()
	`, orderID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return NewBusinessError("Pesanan sudah kedaluwarsa atau statusnya telah berubah.")
	}

	if err := r.releaseReservedOrderResources(ctx, tx, orderID, tierID, quantity, promoID); err != nil {
		return err
	}

	return tx.Commit()
}

func randomHex(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
