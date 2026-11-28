package model

import "time"

// GatekeeperTicket menyimpan informasi tiket
// yang dibutuhkan untuk proses validasi dan check-in.
type GatekeeperTicket struct {
	ID          int64  `json:"id"`
	TicketCode  string `json:"ticket_code"`
	HolderName  string `json:"holder_name"`
	HolderEmail string `json:"holder_email"`
	Status      string `json:"status"`

	EventID     int64  `json:"event_id"`
	EventName   string `json:"event_name"`
	Venue       string `json:"venue"`
	EventStatus string `json:"event_status"`

	CheckedInAt *time.Time `json:"checked_in_at"`
	CheckedInBy *int64     `json:"checked_in_by"`
}

// BoardingPass menyimpan informasi lengkap untuk cetak tiket.
type BoardingPass struct {
	TicketCode   string  `json:"ticket_code"`
	HolderName   string  `json:"holder_name"`
	HolderEmail  string  `json:"holder_email"`
	UnitNetPrice float64 `json:"unit_net_price"`
	Status       string  `json:"status"`

	EventName    string    `json:"event_name"`
	Venue        string    `json:"venue"`
	EventDate    time.Time `json:"event_date"`
	EventStatus  string    `json:"event_status"`
	CategoryName string    `json:"category_name"`
	TierName     string    `json:"tier_name"`
}

// GatekeeperEvent menyimpan informasi event
// yang dibutuhkan oleh modul Gatekeeper.
type GatekeeperEvent struct {
	ID     int64
	Title  string
	Venue  string
	Status string
}
