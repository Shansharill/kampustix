package repository

import (
	"context"
	"database/sql"
	"errors"

	"kampustix/internal/model"
)

// GatekeeperRepo mengelola operasi database untuk Gatekeeper.
type GatekeeperRepo struct {
	DB *sql.DB
}

// FindByCode mencari tiket berdasarkan kode tiket.

func (r *GatekeeperRepo) FindByCode(
	ctx context.Context,
	code string,
) (*model.GatekeeperTicket, error) {

	query := `
        SELECT
            t.id,
            t.ticket_code,
            t.holder_name,
            t.holder_email,
            t.status,
            e.status AS event_status,
            t.checked_in_at,
            t.checked_in_by
        FROM tickets t
        JOIN events e ON t.event_id = e.id
        WHERE t.ticket_code = ?
        LIMIT 1
    `

	row := r.DB.QueryRowContext(ctx, query, code)

	var ticket model.GatekeeperTicket

	err := row.Scan(
		&ticket.ID,
		&ticket.TicketCode,
		&ticket.HolderName,
		&ticket.HolderEmail,
		&ticket.Status,
		&ticket.EventStatus,
		&ticket.CheckedInAt,
		&ticket.CheckedInBy,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return &ticket, nil
}

// CheckIn mengubah status tiket menjadi USED.
func (r *GatekeeperRepo) CheckIn(
	ctx context.Context,
	code string,
	gatekeeperID int64,
) (bool, error) {

	query := `
        UPDATE tickets t
        JOIN events e ON t.event_id = e.id
        SET
            t.status = 'USED',
            t.checked_in_at = NOW(),
            t.checked_in_by = ?
        WHERE
            t.ticket_code = ?
            AND t.status = 'ACTIVE'
            AND e.status = 'ACTIVE'
    `

	result, err := r.DB.ExecContext(ctx, query, gatekeeperID, code)
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected == 1, nil
}

// FindBoardingPass mengambil informasi lengkap tiket untuk dicetak.
func (r *GatekeeperRepo) FindBoardingPass(
	ctx context.Context,
	code string,
) (*model.BoardingPass, error) {

	query := `
		SELECT
			t.ticket_code,
			t.holder_name,
			t.holder_email,
			t.unit_net_price,
			t.status,
			e.title AS event_name,
			e.venue,
			e.event_date,
			c.category_name,
			tr.tier_name
		FROM tickets t
		JOIN events e ON t.event_id = e.id
		JOIN ticket_categories c ON t.category_id = c.id
		JOIN ticket_tiers tr ON t.tier_id = tr.id
		WHERE t.ticket_code = ?
		LIMIT 1
	`

	var boardingPass model.BoardingPass

	err := r.DB.QueryRowContext(ctx, query, code).Scan(
		&boardingPass.TicketCode,
		&boardingPass.HolderName,
		&boardingPass.HolderEmail,
		&boardingPass.UnitNetPrice,
		&boardingPass.Status,
		&boardingPass.EventName,
		&boardingPass.Venue,
		&boardingPass.EventDate,
		&boardingPass.CategoryName,
		&boardingPass.TierName,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return &boardingPass, nil
}

// FindBoardingPassByBuyer mengambil boarding pass
// berdasarkan kode tiket dan ID pemilik order.
func (r *GatekeeperRepo) FindBoardingPassByBuyer(
	ctx context.Context,
	code string,
	buyerID int64,
) (*model.BoardingPass, error) {

	query := `
        SELECT
            t.ticket_code,
            t.holder_name,
            t.holder_email,
            t.unit_net_price,
            t.status,
            e.title AS event_name,
            e.venue,
            e.event_date,
            c.category_name,
            tr.tier_name
        FROM tickets t
        JOIN orders o ON t.order_id = o.id
        JOIN events e ON t.event_id = e.id
        JOIN ticket_categories c ON t.category_id = c.id
        JOIN ticket_tiers tr ON t.tier_id = tr.id
        WHERE t.ticket_code = ?
        AND o.user_id = ?
        LIMIT 1
    `

	var boardingPass model.BoardingPass

	err := r.DB.QueryRowContext(
		ctx,
		query,
		code,
		buyerID,
	).Scan(
		&boardingPass.TicketCode,
		&boardingPass.HolderName,
		&boardingPass.HolderEmail,
		&boardingPass.UnitNetPrice,
		&boardingPass.Status,
		&boardingPass.EventName,
		&boardingPass.Venue,
		&boardingPass.EventDate,
		&boardingPass.CategoryName,
		&boardingPass.TierName,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return &boardingPass, nil
}
