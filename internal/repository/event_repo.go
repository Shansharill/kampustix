
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"kampustix/internal/model"
)

// EventRepo bertanggung jawab mengelola data event di MySQL.
type EventRepo struct {
	DB *sql.DB
}

// SemuaEvent mengambil seluruh event untuk halaman admin.
func (r *EventRepo) SemuaEvent(ctx context.Context) ([]model.Event, error) {
	query := `
		SELECT id, title, event_type, description, venue,
		       event_date, total_capacity, status, created_by, created_at
		FROM events
		ORDER BY event_date ASC
	`

	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar event: %w", err)
	}
	defer rows.Close()

	var hasil []model.Event

	for rows.Next() {
		var e model.Event

		err := rows.Scan(
			&e.ID,
			&e.Title,
			&e.EventType,
			&e.Description,
			&e.Venue,
			&e.EventDate,
			&e.TotalCapacity,
			&e.Status,
			&e.CreatedBy,
			&e.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("membaca data event: %w", err)
		}

		hasil = append(hasil, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hasil, nil
}

// CariEventByID mengambil satu event berdasarkan ID.
func (r *EventRepo) CariEventByID(
	ctx context.Context,
	id int64,
) (model.Event, error) {
	var e model.Event

	query := `
		SELECT id, title, event_type, description, venue,
		       event_date, total_capacity, status, created_by, created_at
		FROM events
		WHERE id = ?
	`

	err := r.DB.QueryRowContext(ctx, query, id).Scan(
		&e.ID,
		&e.Title,
		&e.EventType,
		&e.Description,
		&e.Venue,
		&e.EventDate,
		&e.TotalCapacity,
		&e.Status,
		&e.CreatedBy,
		&e.CreatedAt,
	)

	if err != nil {
		return e, fmt.Errorf("mencari event: %w", err)
	}

	return e, nil
}


// TambahEvent menyimpan event baru ke database.
func (r *EventRepo) TambahEvent(
	ctx context.Context,
	e *model.Event,
) error {
	query := `
		INSERT INTO events (
			title,
			event_type,
			description,
			venue,
			event_date,
			total_capacity,
			status,
			created_by
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.DB.ExecContext(
		ctx,
		query,
		e.Title,
		e.EventType,
		e.Description,
		e.Venue,
		e.EventDate,
		e.TotalCapacity,
		e.Status,
		e.CreatedBy,
	)

	if err != nil {
		return fmt.Errorf("menambahkan event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("mengambil ID event: %w", err)
	}

	e.ID = id

	return nil
}


 // UpdateEvent mengubah informasi event berdasarkan ID.
func (r *EventRepo) UpdateEvent(
	ctx context.Context,
	e *model.Event,
) error {
	query := `
		UPDATE events
		SET title = ?,
			event_type = ?,
			description = ?,
			venue = ?,
			event_date = ?,
			total_capacity = ?,
			status = ?
		WHERE id = ?
	`

	_, err := r.DB.ExecContext(
    ctx,
    query,
    e.Title,
    e.EventType,
    e.Description,
    e.Venue,
    e.EventDate,
    e.TotalCapacity,
    e.Status,
    e.ID,
)

	if err != nil {
		return fmt.Errorf("mengubah event: %w", err)
	}

	return nil
}


 // HapusEvent menghapus event berdasarkan ID.
func (r *EventRepo) HapusEvent(
	ctx context.Context,
	id int64,
) error {
	query := `DELETE FROM events WHERE id = ?`

	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("menghapus event: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("memeriksa penghapusan event: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("event dengan ID %d tidak ditemukan", id)
	}

	return nil
}


// SemuaKategoriByEventID mengambil seluruh kategori tiket milik suatu event.
func (r *EventRepo) SemuaKategoriByEventID(
	ctx context.Context,
	eventID int64,
) ([]model.TicketCategory, error) {
	query := `
		SELECT id, event_id, category_name, total_quota, created_at
		FROM ticket_categories
		WHERE event_id = ?
		ORDER BY id ASC
	`

	rows, err := r.DB.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("mengambil kategori tiket: %w", err)
	}
	defer rows.Close()

	var hasil []model.TicketCategory

	for rows.Next() {
		var kategori model.TicketCategory

		if err := rows.Scan(
			&kategori.ID,
			&kategori.EventID,
			&kategori.CategoryName,
			&kategori.TotalQuota,
			&kategori.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("membaca kategori tiket: %w", err)
		}

		hasil = append(hasil, kategori)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hasil, nil
}

// TambahKategori menyimpan kategori tiket baru.
func (r *EventRepo) TambahKategori(
	ctx context.Context,
	kategori *model.TicketCategory,
) error {
	query := `
		INSERT INTO ticket_categories (
			event_id, category_name, total_quota
		)
		VALUES (?, ?, ?)
	`

	result, err := r.DB.ExecContext(
		ctx,
		query,
		kategori.EventID,
		kategori.CategoryName,
		kategori.TotalQuota,
	)
	if err != nil {
		return fmt.Errorf("menambahkan kategori tiket: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("mengambil ID kategori: %w", err)
	}

	kategori.ID = id
	return nil
}

// SemuaTierByCategoryID mengambil seluruh tier dalam suatu kategori.
func (r *EventRepo) SemuaTierByCategoryID(
	ctx context.Context,
	categoryID int64,
) ([]model.TicketTier, error) {
	query := `
		SELECT id, category_id, tier_name, sequence_order,
		       price, quota, used_quota, start_date, end_date, created_at
		FROM ticket_tiers
		WHERE category_id = ?
		ORDER BY sequence_order ASC
	`

	rows, err := r.DB.QueryContext(ctx, query, categoryID)
	if err != nil {
		return nil, fmt.Errorf("mengambil tier tiket: %w", err)
	}
	defer rows.Close()

	var hasil []model.TicketTier

	for rows.Next() {
		var tier model.TicketTier

		if err := rows.Scan(
			&tier.ID,
			&tier.CategoryID,
			&tier.TierName,
			&tier.SequenceOrder,
			&tier.Price,
			&tier.Quota,
			&tier.UsedQuota,
			&tier.StartDate,
			&tier.EndDate,
			&tier.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("membaca tier tiket: %w", err)
		}

		hasil = append(hasil, tier)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hasil, nil
}

// TambahTier menyimpan tahapan harga baru.
func (r *EventRepo) TambahTier(
	ctx context.Context,
	tier *model.TicketTier,
) error {
	query := `
		INSERT INTO ticket_tiers (
			category_id, tier_name, sequence_order,
			price, quota, used_quota, start_date, end_date
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.DB.ExecContext(
		ctx,
		query,
		tier.CategoryID,
		tier.TierName,
		tier.SequenceOrder,
		tier.Price,
		tier.Quota,
		tier.UsedQuota,
		tier.StartDate,
		tier.EndDate,
	)
	if err != nil {
		return fmt.Errorf("menambahkan tier tiket: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("mengambil ID tier: %w", err)
	}

	tier.ID = id
	return nil
}

// TierAktifByCategoryID mencari tier dengan urutan terkecil
// yang kuotanya masih tersedia dan belum melewati end_date.
func (r *EventRepo) TierAktifByCategoryID(
	ctx context.Context,
	categoryID int64,
) (*model.TicketTier, error) {
	query := `
		
			SELECT id, category_id, tier_name, sequence_order,
				price, quota, used_quota, start_date, end_date, created_at
			FROM ticket_tiers
			WHERE category_id = ?
			AND used_quota < quota
			AND start_date <= NOW()
			AND end_date >= NOW()
			ORDER BY sequence_order ASC
			LIMIT 1
	`

	var tier model.TicketTier

	err := r.DB.QueryRowContext(ctx, query, categoryID).Scan(
		&tier.ID,
		&tier.CategoryID,
		&tier.TierName,
		&tier.SequenceOrder,
		&tier.Price,
		&tier.Quota,
		&tier.UsedQuota,
		&tier.StartDate,
		&tier.EndDate,
		&tier.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("mencari tier aktif: %w", err)
	}

	return &tier, nil
}


func (r *EventRepo) CariKategoriByID(
	ctx context.Context,
	id int64,
) (model.TicketCategory, error) {
	var kategori model.TicketCategory

	query := `
		SELECT id, event_id, category_name, total_quota, created_at
		FROM ticket_categories
		WHERE id = ?
	`

	err := r.DB.QueryRowContext(ctx, query, id).Scan(
		&kategori.ID,
		&kategori.EventID,
		&kategori.CategoryName,
		&kategori.TotalQuota,
		&kategori.CreatedAt,
	)

	if err != nil {
		return kategori, fmt.Errorf("mencari kategori tiket: %w", err)
	}

	return kategori, nil
}
