package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"kampustix/internal/middleware"
	"kampustix/internal/model"
	"kampustix/internal/service"
)

type OrderHandler struct {
	orderService *service.OrderService
	rd           *Renderer
}

func NewOrderHandler(orderService *service.OrderService, rd *Renderer) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
		rd:           rd,
	}
}

// getLoggedUserID mengambil ID buyer dari session yang sudah diperiksa middleware.
func getLoggedUserID(r *http.Request) int {
	if user := middleware.CurrentUser(r); user != nil {
		return int(user.ID)
	}
	return 0
}

// ShowCheckoutForm menampilkan form checkout.
// event_id dan tier_id harus berasal dari tier yang dipilih; tidak ada default ID buatan.
func (h *OrderHandler) ShowCheckoutForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	eventID, eventErr := strconv.Atoi(r.URL.Query().Get("event_id"))
	tierID, tierErr := strconv.Atoi(r.URL.Query().Get("tier_id"))
	checkoutReady := eventErr == nil && tierErr == nil && eventID > 0 && tierID > 0

	pesan := r.URL.Query().Get("pesan")
	if !checkoutReady && pesan == "" {
		pesan = "Pilih event dan tier tiket terlebih dahulu. Untuk pengujian langsung, buka /checkout?event_id=ID_EVENT&tier_id=ID_TIER."
	}

	data := map[string]any{
		"Title":         "Checkout Tiket - KampusTix",
		"User":          user,
		"Pesan":         pesan,
		"EventID":       eventID,
		"TierID":        tierID,
		"CheckoutReady": checkoutReady,
	}
	h.rd.Tampil(w, "checkout/form.html", data)
}

// HandleCheckout memproses POST /checkout dan membuat order dengan hold 15 menit.
func (h *OrderHandler) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := getLoggedUserID(r)
	if userID == 0 {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Gagal membaca form checkout", http.StatusBadRequest)
		return
	}

	eventID, eventErr := strconv.Atoi(r.FormValue("event_id"))
	tierID, tierErr := strconv.Atoi(r.FormValue("tier_id"))
	quantity, quantityErr := strconv.Atoi(r.FormValue("quantity"))
	if eventErr != nil || tierErr != nil || quantityErr != nil || eventID <= 0 || tierID <= 0 {
		http.Redirect(w, r, "/checkout?pesan="+url.QueryEscape("Event, tier, atau jumlah tiket tidak valid."), http.StatusSeeOther)
		return
	}

	names := r.Form["holder_name[]"]
	emails := r.Form["holder_email[]"]
	holderCount := len(names)
	if len(emails) > holderCount {
		holderCount = len(emails)
	}

	holders := make([]model.TicketHolderInput, 0, holderCount)
	for i := 0; i < holderCount; i++ {
		var name, email string
		if i < len(names) {
			name = names[i]
		}
		if i < len(emails) {
			email = emails[i]
		}
		holders = append(holders, model.TicketHolderInput{
			HolderName:  name,
			HolderEmail: email,
		})
	}

	req := model.CheckoutRequest{
		UserID:    userID,
		EventID:   eventID,
		TierID:    tierID,
		PromoCode: r.FormValue("promo_code"),
		Quantity:  quantity,
		Holders:   holders,
	}

	orderID, err := h.orderService.ProcessCheckout(r.Context(), req)
	if err != nil {
		pesanErr := url.QueryEscape("Checkout gagal: " + err.Error())
		destination := "/checkout?event_id=" + strconv.Itoa(eventID) +
			"&tier_id=" + strconv.Itoa(tierID) + "&pesan=" + pesanErr
		http.Redirect(w, r, destination, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/orders/"+strconv.Itoa(orderID)+"/payment", http.StatusSeeOther)
}

// ShowPaymentPage menampilkan detail order dan countdown berdasarkan expires_at di MySQL.
func (h *OrderHandler) ShowPaymentPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		http.Error(w, "ID pesanan tidak valid", http.StatusBadRequest)
		return
	}

	pageData, err := h.orderService.GetOrderForPayment(r.Context(), orderID, int(user.ID))
	if err != nil {
		if err.Error() == "FORBIDDEN" {
			http.Error(w, "403 Forbidden: Anda bukan pemilik pesanan ini", http.StatusForbidden)
			return
		}
		if errors.Is(err, sql.ErrNoRows) || err.Error() == "pesanan tidak ditemukan" {
			http.NotFound(w, r)
			return
		}
		log.Printf("gagal membuka halaman pembayaran order %d: %v", orderID, err)
		http.Error(w, "Terjadi kesalahan saat mengambil data pembayaran", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":            "Konfirmasi Pembayaran - KampusTix",
		"User":             user,
		"Pesan":            r.URL.Query().Get("pesan"),
		"Order":            pageData.Order,
		"EventTitle":       pageData.EventTitle,
		"CategoryName":     pageData.CategoryName,
		"TierName":         pageData.TierName,
		"Tickets":          pageData.Tickets,
		"RemainingSeconds": pageData.RemainingSeconds,
	}
	h.rd.Tampil(w, "checkout/payment.html", data)
}

// HandlePaySimulate memproses POST /orders/{id}/pay-simulate.
func (h *OrderHandler) HandlePaySimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := getLoggedUserID(r)
	if userID == 0 {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		http.Error(w, "ID pesanan tidak valid", http.StatusBadRequest)
		return
	}

	if err := h.orderService.SimulatePayment(r.Context(), orderID, userID); err != nil {
		if err.Error() == "FORBIDDEN" {
			http.Error(w, "403 Forbidden: Anda tidak berhak membayar pesanan ini", http.StatusForbidden)
			return
		}
		pesanErr := url.QueryEscape("Pembayaran gagal: " + err.Error())
		http.Redirect(w, r, "/orders/"+strconv.Itoa(orderID)+"/payment?pesan="+pesanErr, http.StatusSeeOther)
		return
	}

	// Setelah transaksi pembayaran berhasil, tampilkan kembali halaman ringkasan order.
	// Handler GET akan membaca status PAID dan tiket ACTIVE langsung dari database.
	destination := "/orders/" + strconv.Itoa(orderID) + "/payment?pesan=" +
		url.QueryEscape("Pembayaran berhasil. Status pesanan PAID dan tiket telah aktif.")
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

// HandleCancelOrder memproses pembatalan manual buyer sebelum waktu hold 15 menit habis.
func (h *OrderHandler) HandleCancelOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := getLoggedUserID(r)
	if userID == 0 {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || orderID <= 0 {
		http.Error(w, "ID pesanan tidak valid", http.StatusBadRequest)
		return
	}

	if err := h.orderService.CancelOrder(r.Context(), orderID, userID); err != nil {
		if err.Error() == "FORBIDDEN" {
			http.Error(w, "403 Forbidden: Anda bukan pemilik pesanan ini", http.StatusForbidden)
			return
		}
		pesanErr := url.QueryEscape("Pesanan tidak dapat dibatalkan: " + err.Error())
		http.Redirect(w, r, "/orders/"+strconv.Itoa(orderID)+"/payment?pesan="+pesanErr, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r,
		"/orders/"+strconv.Itoa(orderID)+"/payment?pesan=Pesanan+berhasil+dibatalkan.+Kuota+telah+dikembalikan",
		http.StatusSeeOther,
	)
}
