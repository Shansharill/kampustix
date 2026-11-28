package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"kampustix/internal/model"
)

// PromoReportRepo: query SQL untuk modul promo & laporan keuangan (Anggota 4).
type PromoReportRepo struct {
	DB *sql.DB
}

// ---------------------------------------------------------------- CRUD PROMO

// SemuaPromo mengambil seluruh kode promo beserta nama event & kategori.
func (r *PromoReportRepo) SemuaPromo(ctx context.Context) ([]model.PromoCode, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT p.id, p.event_id, e.title, p.category_id, COALESCE(c.category_name,''),
		       p.code, p.discount_percent, p.max_uses, p.used_count,
		       p.valid_until, p.is_active, p.created_at
		FROM promo_codes p
		JOIN events e ON e.id = p.event_id
		LEFT JOIN ticket_categories c ON c.id = p.category_id
		ORDER BY p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("daftar promo: %w", err)
	}
	defer rows.Close()

	var hasil []model.PromoCode
	for rows.Next() {
		var p model.PromoCode
		var catID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.EventID, &p.EventTitle, &catID, &p.CategoryName,
			&p.Code, &p.DiscountPercent, &p.MaxUses, &p.UsedCount,
			&p.ValidUntil, &p.IsActive, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan promo: %w", err)
		}
		if catID.Valid {
			v := catID.Int64
			p.CategoryID = &v
		}
		hasil = append(hasil, p)
	}
	return hasil, rows.Err()
}

// DaftarEvent & DaftarKategori mengisi dropdown form promo.
func (r *PromoReportRepo) DaftarEvent(ctx context.Context) ([]model.PilihanEvent, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, title FROM events ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("daftar event: %w", err)
	}
	defer rows.Close()
	var hasil []model.PilihanEvent
	for rows.Next() {
		var e model.PilihanEvent
		if err := rows.Scan(&e.ID, &e.Title); err != nil {
			return nil, err
		}
		hasil = append(hasil, e)
	}
	return hasil, rows.Err()
}

func (r *PromoReportRepo) DaftarKategori(ctx context.Context) ([]model.PilihanKategori, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, event_id, category_name FROM ticket_categories ORDER BY event_id, id`)
	if err != nil {
		return nil, fmt.Errorf("daftar kategori: %w", err)
	}
	defer rows.Close()
	var hasil []model.PilihanKategori
	for rows.Next() {
		var k model.PilihanKategori
		if err := rows.Scan(&k.ID, &k.EventID, &k.Name); err != nil {
			return nil, err
		}
		hasil = append(hasil, k)
	}
	return hasil, rows.Err()
}

// KategoriMilikEvent true jika kategori tersebut memang bagian dari event.
func (r *PromoReportRepo) KategoriMilikEvent(ctx context.Context, categoryID, eventID int64) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ticket_categories WHERE id = ? AND event_id = ?`, categoryID, eventID).Scan(&n)
	return n > 0, err
}

// EventAda true jika event dengan ID tersebut ada.
func (r *PromoReportRepo) EventAda(ctx context.Context, eventID int64) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE id = ?`, eventID).Scan(&n)
	return n > 0, err
}

// KodeSudahAda cek duplikasi kode (kolom code bersifat UNIQUE).
func (r *PromoReportRepo) KodeSudahAda(ctx context.Context, kode string) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM promo_codes WHERE code = ?`, kode).Scan(&n)
	return n > 0, err
}

// TambahPromo menyimpan promo baru. CategoryID 0 disimpan sebagai NULL.
func (r *PromoReportRepo) TambahPromo(ctx context.Context, in model.PromoInput) (int64, error) {
	var cat any
	if in.CategoryID > 0 {
		cat = in.CategoryID
	}
	res, err := r.DB.ExecContext(ctx, `
		INSERT INTO promo_codes (event_id, category_id, code, discount_percent, max_uses, valid_until)
		VALUES (?, ?, ?, ?, ?, ?)`,
		in.EventID, cat, in.Code, in.DiscountPercent, in.MaxUses, in.ValidUntil)
	if err != nil {
		return 0, fmt.Errorf("tambah promo: %w", err)
	}
	return res.LastInsertId()
}

// ToggleAktif membalik status is_active (aktif <-> nonaktif).
func (r *PromoReportRepo) ToggleAktif(ctx context.Context, id int64) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE promo_codes SET is_active = 1 - is_active WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("toggle promo %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return model.ErrPromoNotFound
	}
	return nil
}

// ------------------------------------------------- DIPANGGIL MODUL CHECKOUT

// PakaiKuota memvalidasi sekaligus memotong 1 kuota promo SECARA ATOMIK di dalam
// transaksi `tx` milik Checkout. Semua syarat dicek dalam satu UPDATE, sehingga dua
// pembeli yang checkout bersamaan tidak bisa sama-sama menembus batas max_uses.
// Mengembalikan (nil, nil) jika kode tidak memenuhi syarat.
func (r *PromoReportRepo) PakaiKuota(ctx context.Context, tx *sql.Tx, kode string, eventID, categoryID int64) (*model.PromoCode, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE promo_codes
		SET used_count = used_count + 1
		WHERE code = ?
		  AND event_id = ?
		  AND (category_id IS NULL OR category_id = ?)
		  AND is_active = 1
		  AND valid_until >= NOW()
		  AND used_count < max_uses`, kode, eventID, categoryID)
	if err != nil {
		return nil, fmt.Errorf("pakai kuota promo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}

	var p model.PromoCode
	err = tx.QueryRowContext(ctx,
		`SELECT id, code, discount_percent FROM promo_codes WHERE code = ?`, kode,
	).Scan(&p.ID, &p.Code, &p.DiscountPercent)
	if err != nil {
		return nil, fmt.Errorf("baca promo: %w", err)
	}
	return &p, nil
}

// LepasKuota mengembalikan 1 kuota promo (hold 15 menit habis / order dibatalkan).
func (r *PromoReportRepo) LepasKuota(ctx context.Context, tx *sql.Tx, promoID int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE promo_codes SET used_count = used_count - 1 WHERE id = ? AND used_count > 0`, promoID)
	if err != nil {
		return fmt.Errorf("lepas kuota promo %d: %w", promoID, err)
	}
	return nil
}

// ------------------------------------------------------------------ LAPORAN

// Ringkasan menghitung 5 metrik utama laporan keuangan (PRD 3.8).
func (r *PromoReportRepo) Ringkasan(ctx context.Context) (model.RevenueReport, error) {
	var rep model.RevenueReport
	err := r.DB.QueryRowContext(ctx, `
		SELECT CAST(COALESCE(SUM(total_gross),0) AS SIGNED),
		       CAST(COALESCE(SUM(discount_amount),0) AS SIGNED)
		FROM orders WHERE status = 'PAID'`).Scan(&rep.Gross, &rep.TotalDiskon)
	if err != nil {
		return rep, fmt.Errorf("ringkasan order: %w", err)
	}
	err = r.DB.QueryRowContext(ctx,
		`SELECT CAST(COALESCE(SUM(refund_amount),0) AS SIGNED) FROM refunds`).Scan(&rep.TotalRefund)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return rep, fmt.Errorf("ringkasan refund: %w", err)
	}
	rep.NetSebelumRefund = rep.Gross - rep.TotalDiskon
	rep.NetAkhir = rep.NetSebelumRefund - rep.TotalRefund
	return rep, nil
}

// PenjualanPerTier merinci omzet per event, kategori, dan tier harga.
// Subquery dipakai agar join order & refund tidak saling menggandakan baris.
func (r *PromoReportRepo) PenjualanPerTier(ctx context.Context) ([]model.BarisPenjualan, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT e.title, c.category_name, t.tier_name,
		       CAST(COALESCE(o.qty,0)   AS SIGNED),
		       CAST(COALESCE(o.gross,0) AS SIGNED),
		       CAST(COALESCE(o.disc,0)  AS SIGNED),
		       CAST(COALESCE(rf.refund,0) AS SIGNED)
		FROM ticket_tiers t
		JOIN ticket_categories c ON c.id = t.category_id
		JOIN events e            ON e.id = c.event_id
		LEFT JOIN (
			SELECT tier_id, SUM(quantity) qty, SUM(total_gross) gross, SUM(discount_amount) disc
			FROM orders WHERE status = 'PAID' GROUP BY tier_id
		) o ON o.tier_id = t.id
		LEFT JOIN (
			SELECT tk.tier_id, SUM(r.refund_amount) refund
			FROM refunds r JOIN tickets tk ON tk.id = r.ticket_id
			GROUP BY tk.tier_id
		) rf ON rf.tier_id = t.id
		ORDER BY e.id, c.id, t.sequence_order`)
	if err != nil {
		return nil, fmt.Errorf("penjualan per tier: %w", err)
	}
	defer rows.Close()

	var hasil []model.BarisPenjualan
	for rows.Next() {
		var b model.BarisPenjualan
		if err := rows.Scan(&b.EventTitle, &b.Kategori, &b.Tier,
			&b.TiketTerjual, &b.Gross, &b.Diskon, &b.Refund); err != nil {
			return nil, fmt.Errorf("scan penjualan: %w", err)
		}
		b.Net = b.Gross - b.Diskon - b.Refund
		hasil = append(hasil, b)
	}
	return hasil, rows.Err()
}
