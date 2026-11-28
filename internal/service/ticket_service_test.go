package service

import (
	"testing"

	"kampustix/internal/model"
)

func TestHitungRefund(t *testing.T) {
	cases := []struct{ net, persen, mau float64 }{
		{240000, 75, 180000},
		{150000, 100, 150000},
		{87499.12, 75, 65624.34},
		{0, 75, 0},
	}
	for _, c := range cases {
		if got := model.HitungRefund(c.net, c.persen); got != c.mau {
			t.Errorf("HitungRefund(%v, %v) = %v, mau %v", c.net, c.persen, got, c.mau)
		}
	}
}

func TestAturanTransferRefund(t *testing.T) {
	aktif := model.TiketUntukAksi{Status: "ACTIVE", EventStatus: "UPCOMING", MasihH7: true}

	cek := func(nama string, got, mau error) {
		if got != mau {
			t.Errorf("%s: dapat %v, mau %v", nama, got, mau)
		}
	}
	cek("transfer normal", CekBolehTransfer(&aktif), nil)
	cek("refund normal", CekBolehRefund(&aktif), nil)

	sudah := aktif
	sudah.TransferCount = 1
	cek("transfer kedua", CekBolehTransfer(&sudah), ErrSudahPernahTransfer)
	cek("refund tiket yang sudah ditransfer tetap boleh", CekBolehRefund(&sudah), nil)

	lewat := aktif
	lewat.MasihH7 = false
	cek("refund lewat H-7", CekBolehRefund(&lewat), ErrLewatH7)
	cek("transfer lewat H-7 tetap boleh", CekBolehTransfer(&lewat), nil)

	for _, st := range []string{"USED", "REFUNDED", "CANCELLED", "RESERVED"} {
		x := aktif
		x.Status = st
		cek("transfer status "+st, CekBolehTransfer(&x), ErrTiketTidakAktif)
		cek("refund status "+st, CekBolehRefund(&x), ErrTiketTidakAktif)
	}

	batal := aktif
	batal.EventStatus = "CANCELLED"
	cek("refund event batal", CekBolehRefund(&batal), ErrEventSudahBatal)
}

func TestValidasiTransfer(t *testing.T) {
	ok := []model.TransferInput{{HolderName: "Budi", HolderEmail: "budi@mail.com"}, {HolderName: "Ani Lestari", HolderEmail: "ani.l@kampus.ac.id"}}
	for _, in := range ok {
		if err := ValidasiTransfer(in); err != nil {
			t.Errorf("%+v harus lolos: %v", in, err)
		}
	}
	salah := []model.TransferInput{
		{HolderName: "", HolderEmail: "budi@mail.com"},
		{HolderName: "Budi", HolderEmail: ""},
		{HolderName: "Budi", HolderEmail: "bukan-email"},
		{HolderName: "Budi", HolderEmail: "Budi <budi@mail.com>"},
	}
	for _, in := range salah {
		if ValidasiTransfer(in) == nil {
			t.Errorf("%+v harus ditolak", in)
		}
	}
}

func TestFilterTab(t *testing.T) {
	semua := []model.TiketSaya{{ID: 1, Status: "ACTIVE"}, {ID: 2, Status: "USED"}, {ID: 3, Status: "REFUNDED"},
		{ID: 4, Status: "CANCELLED"}, {ID: 5, Status: "RESERVED"}}
	jumlah := map[string]int{"": 2, "aktif": 2, "riwayat": 1, "dibatalkan": 2, "ngawur": 2}
	for tab, mau := range jumlah {
		if got := len(FilterTab(semua, tab)); got != mau {
			t.Errorf("tab %q: %d tiket, mau %d", tab, got, mau)
		}
	}
}
