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
	aktif := model.StatusTiket{Status: "ACTIVE", EventStatus: "UPCOMING", MasihH7: true, EventBelumMulai: true}

	cek := func(nama string, got, mau error) {
		t.Helper()
		if got != mau {
			t.Errorf("%s: dapat %v, mau %v", nama, got, mau)
		}
	}
	cek("transfer normal", aktif.AlasanTolakTransfer(), nil)
	cek("refund normal", aktif.AlasanTolakRefund(), nil)

	sudah := aktif
	sudah.TransferCount = 1
	cek("transfer kedua", sudah.AlasanTolakTransfer(), ErrSudahPernahTransfer)
	cek("refund tiket yang sudah ditransfer tetap boleh", sudah.AlasanTolakRefund(), nil)

	lewatH7 := aktif
	lewatH7.MasihH7 = false
	cek("refund lewat H-7", lewatH7.AlasanTolakRefund(), ErrLewatH7)
	cek("transfer lewat H-7 (event belum mulai) tetap boleh", lewatH7.AlasanTolakTransfer(), nil)

	// Review PR #1 poin 3: transfer ditolak jika event sudah berlangsung / selesai
	mulai := lewatH7
	mulai.EventBelumMulai = false
	cek("transfer saat event sudah lewat", mulai.AlasanTolakTransfer(), ErrEventSudahLewat)
	selesai := aktif
	selesai.EventStatus = "COMPLETED"
	cek("transfer event COMPLETED", selesai.AlasanTolakTransfer(), ErrEventSudahLewat)
	cek("refund event COMPLETED", selesai.AlasanTolakRefund(), ErrLewatH7)

	for _, st := range []string{"USED", "REFUNDED", "CANCELLED", "RESERVED"} {
		x := aktif
		x.Status = st
		cek("transfer status "+st, x.AlasanTolakTransfer(), ErrTiketTidakAktif)
		cek("refund status "+st, x.AlasanTolakRefund(), ErrTiketTidakAktif)
	}

	batal := aktif
	batal.EventStatus = "CANCELLED"
	cek("transfer event batal", batal.AlasanTolakTransfer(), ErrEventSudahBatal)
	cek("refund event batal", batal.AlasanTolakRefund(), ErrEventSudahBatal)

	// Tampilan (template) memakai aturan yang sama dengan service
	tiket := model.TiketSaya{StatusTiket: mulai}
	if tiket.BolehTransfer() || tiket.BolehRefund() {
		t.Error("tiket event yang sudah lewat tidak boleh menampilkan tombol transfer/refund")
	}
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
	var semua []model.TiketSaya
	for i, st := range []string{"ACTIVE", "USED", "REFUNDED", "CANCELLED", "RESERVED"} {
		semua = append(semua, model.TiketSaya{ID: int64(i + 1), StatusTiket: model.StatusTiket{Status: st}})
	}
	jumlah := map[string]int{"": 2, "aktif": 2, "riwayat": 1, "dibatalkan": 2, "ngawur": 2}
	for tab, mau := range jumlah {
		if got := len(FilterTab(semua, tab)); got != mau {
			t.Errorf("tab %q: %d tiket, mau %d", tab, got, mau)
		}
	}
}
