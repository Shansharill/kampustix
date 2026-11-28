package repository

import (
	"context"
	"database/sql"
	"errors"

	"kampustix/internal/model"
)

// RefundRepo mengelola query tiket pembeli, transfer, dan refund (Anggota 5)
type RefundRepo struct {
	DB *sql.DB
}

// TiketMilikUser mengambil semua tiket dari order milik user.
// Catatan: tabel tickets tidak punya user_id, jadi pemilik dicari lewat orders.user_id.
func (r *RefundRepo) TiketMilikUser(ctx context.Context, userID int64) ([]model.TiketSaya, error) {
	query := `SELECT t.id, t.ticket_code, t.holder_name, t.holder_email, t.status, t.transfer_count,
			t.unit_net_price, o.order_code, e.title, e.venue, e.event_date, e.status,
			c.category_name, tr.tier_name, NOW() <= DATE_SUB(e.event_date, INTERVAL 7 DAY),
			rf.refund_amount, rf.refund_percent
		FROM tickets t
		JOIN orders o            ON o.id = t.order_id
		JOIN events e            ON e.id = t.event_id
		JOIN ticket_categories c ON c.id = t.category_id
		JOIN ticket_tiers tr     ON tr.id = t.tier_id
		LEFT JOIN refunds rf     ON rf.ticket_id = t.id
		WHERE o.user_id = ?
		ORDER BY e.event_date, t.id`
	rows, err := r.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []model.TiketSaya
	for rows.Next() {
		var t model.TiketSaya
		if err := rows.Scan(&t.ID, &t.TicketCode, &t.HolderName, &t.HolderEmail, &t.Status, &t.TransferCount,
			&t.UnitNetPrice, &t.OrderCode, &t.EventTitle, &t.Venue, &t.EventDate, &t.EventStatus,
			&t.Kategori, &t.Tier, &t.MasihH7, &t.RefundAmount, &t.RefundPercent); err != nil {
			return nil, err
		}
		hasil = append(hasil, t)
	}
	return hasil, rows.Err()
}

// KunciTiketMilik mengambil & mengunci (FOR UPDATE) tiket HANYA jika milik userID (Owner Check / anti-IDOR).
// Mengembalikan nil jika tiket tidak ada atau bukan milik user.
func (r *RefundRepo) KunciTiketMilik(ctx context.Context, tx *sql.Tx, ticketID, userID int64) (*model.TiketUntukAksi, error) {
	query := `SELECT t.id, t.order_id, t.tier_id, t.status, t.transfer_count, t.unit_net_price, e.status,
			NOW() <= DATE_SUB(e.event_date, INTERVAL 7 DAY)
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		JOIN events e ON e.id = t.event_id
		WHERE t.id = ? AND o.user_id = ?
		FOR UPDATE`
	var t model.TiketUntukAksi
	err := tx.QueryRowContext(ctx, query, ticketID, userID).Scan(&t.ID, &t.OrderID, &t.TierID, &t.Status,
		&t.TransferCount, &t.UnitNetPrice, &t.EventStatus, &t.MasihH7)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GantiPemegang menyimpan pemegang baru dan mengunci transfer (transfer_count = 1).
// Syarat transfer_count < 1 dan status ACTIVE ikut ditulis di WHERE sebagai pengaman kedua (PRD 3.6).
// Mengembalikan false jika tidak ada baris yang berubah (tiket sudah pernah ditransfer / tidak aktif).
func (r *RefundRepo) GantiPemegang(ctx context.Context, tx *sql.Tx, ticketID int64, in model.TransferInput) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE tickets
		SET holder_name = ?, holder_email = ?, transfer_count = transfer_count + 1, transferred_at = NOW()
		WHERE id = ? AND transfer_count < 1 AND status = 'ACTIVE'`, in.HolderName, in.HolderEmail, ticketID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// CatatRefundMandiri: tiket → REFUNDED, simpan baris refunds, lepas 1 kuota tier agar bisa dijual lagi
func (r *RefundRepo) CatatRefundMandiri(ctx context.Context, tx *sql.Tx, rf model.Refund, tierID int64) error {
	if _, err := tx.ExecContext(ctx, `UPDATE tickets SET status = 'REFUNDED' WHERE id = ?`, rf.TicketID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO refunds
		(ticket_id, order_id, requested_by, refund_percent, refund_amount, refund_type, reason)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		rf.TicketID, rf.OrderID, rf.RequestedBy, rf.RefundPercent, rf.RefundAmount, rf.RefundType, rf.Reason); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE ticket_tiers SET used_quota = used_quota - 1 WHERE id = ? AND used_quota > 0`, tierID)
	return err
}

// KunciStatusEvent mengunci baris event dan mengembalikan statusnya ("" jika event tidak ada)
func (r *RefundRepo) KunciStatusEvent(ctx context.Context, tx *sql.Tx, eventID int64) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM events WHERE id = ? FOR UPDATE`, eventID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return status, err
}

// BatalkanEvent: refund 100% semua tiket ACTIVE, ubah tiket → REFUNDED, event → CANCELLED.
// Pesanan yang masih PENDING (belum bayar) ikut dibatalkan agar tidak bisa dibayar setelah event batal.
// Kuota tier & promo sengaja TIDAK dilepas karena event tidak dijual lagi (PRD 3.5 poin 2).
// Mengembalikan jumlah tiket yang di-refund.
func (r *RefundRepo) BatalkanEvent(ctx context.Context, tx *sql.Tx, eventID, adminID int64) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO refunds
		(ticket_id, order_id, requested_by, refund_percent, refund_amount, refund_type, reason)
		SELECT t.id, t.order_id, ?, 100, t.unit_net_price, 'EVENT_CANCELLED', 'Event dibatalkan penyelenggara'
		FROM tickets t WHERE t.event_id = ? AND t.status = 'ACTIVE'`, adminID, eventID)
	if err != nil {
		return 0, err
	}
	jumlah, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tickets SET status = 'REFUNDED' WHERE event_id = ? AND status = 'ACTIVE'`, eventID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tickets SET status = 'CANCELLED' WHERE event_id = ? AND status = 'RESERVED'`, eventID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'CANCELLED' WHERE event_id = ? AND status = 'PENDING'`, eventID); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE events SET status = 'CANCELLED' WHERE id = ?`, eventID)
	return jumlah, err
}
