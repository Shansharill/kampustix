package handler

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

// rupiah memformat integer rupiah menjadi string berformat "Rp39.000.000"
func rupiah(n any) string {
	var val int64
	switch v := n.(type) {
	case int:
		val = int64(v)
	case int64:
		val = v
	case float64:
		val = int64(v)
	default:
		return fmt.Sprintf("Rp%v", n)
	}

	s := strconv.FormatInt(val, 10)
	hasil := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 && s[i-1] != '-' {
			hasil += "."
		}
		hasil += string(c)
	}
	return "Rp" + hasil
}

// formatTanggal memformat time.Time ke format tanggal Indonesia yang ramah dibaca
func formatTanggal(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	bulan := [...]string{
		"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	return fmt.Sprintf("%02d %s %d, %02d:%02d WIB",
		t.Day(), bulan[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}

func addOne(n int) int {
	return n + 1
}

// funcs berisi fungsi pembantu (helper) yang dapat dipanggil langsung dari dalam file HTML template
var funcs = template.FuncMap{
	"rupiah":         rupiah,
	"format_tanggal": formatTanggal,
	"add_one":        addOne,
}

// Renderer mengelola caching template HTML agar performa server cepat dan konsisten
type Renderer struct {
	tpl map[string]*template.Template
}

// NewRenderer mendaftarkan layout utama beserta seluruh halaman kanvas untuk 6 anggota
func NewRenderer() *Renderer {
	halaman := []string{
		// Modul Anggota 1
		"auth/login_register.html",

		// Modul Anggota 2
		"event/katalog.html",
		"event/detail.html",
		"event/admin_event.html",

		// Modul Anggota 3
		"checkout/form.html",
		"checkout/payment.html",

		// Modul Anggota 4
		"admin/promo.html",
		"admin/laporan.html",

		// Modul Anggota 5
		"tiket/tiket_saya.html",

		// Modul Anggota 6
		"gatekeeper/scan.html",
		"gatekeeper/print.html",
	}

	rd := &Renderer{tpl: make(map[string]*template.Template)}

	for _, file := range halaman {
		// Menggabungkan template layout induk dengan file halaman spesifik
		// filepath.ToSlash memastikan path kompatibel di Windows maupun Linux/Mac
		layoutPath := filepath.Join("templates", "layout.html")
		pagePath := filepath.Join("templates", filepath.FromSlash(file))

		rd.tpl[file] = template.Must(template.New("layout").Funcs(funcs).ParseFiles(
			layoutPath,
			pagePath,
		))
	}

	return rd
}

// Tampil me-render ke buffer RAM terlebih dahulu.
// Tujuannya: Jika terjadi error sintaks template, server tidak mengirim halaman setengah jadi yang rusak ke browser.
func (rd *Renderer) Tampil(w http.ResponseWriter, file string, data any) {
	t, ok := rd.tpl[file]
	if !ok {
		log.Printf("error: template '%s' belum terdaftar di NewRenderer()", file)
		http.Error(w, "Halaman tidak ditemukan", http.StatusNotFound)
		return
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("render error pada template %s: %v", file, err)
		http.Error(w, "Terjadi kesalahan pada server saat memuat halaman", http.StatusInternalServerError)
		return
	}

	buf.WriteTo(w)
}
