
package model

import "time"

// Event menyimpan informasi utama sebuah acara.
type Event struct {
	ID            int64
	Title         string
	EventType     string
	Description   string
	Venue         string
	EventDate     time.Time
	TotalCapacity int
	Status        string
	CreatedBy     int64
	CreatedAt     time.Time
}

// TicketCategory menyimpan kategori tiket dalam sebuah event.
type TicketCategory struct {
	ID           int64
	EventID      int64
	CategoryName string
	TotalQuota   int
	CreatedAt    time.Time
}

// TicketTier menyimpan tahapan harga tiket.
type TicketTier struct {
	ID           int64
	CategoryID   int64
	TierName     string
	SequenceOrder int
	Price        float64
	Quota        int
	UsedQuota    int
	StartDate    time.Time
	EndDate      time.Time
	CreatedAt    time.Time
}

// EventDetail menggabungkan informasi event,
// kategori tiket, dan tier harga untuk halaman detail.
type EventDetail struct {
	Event      Event
	Categories []TicketCategory
	Tiers      []TicketTier
}
