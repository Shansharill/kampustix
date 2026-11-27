package handler

import (
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

// Helper mengambil ID user yang sedang login dari middleware Anggota 1
func getLoggedUserID(r *http.Request) int {
	if u := middleware.CurrentUser(r); u != nil {
		return int(u.ID)
	}
	return 0
}

// 0. GET /checkout (Menampilkan halaman form checkout 1-4 tiket)
func (h *OrderHandler) ShowCheckoutForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	eventID, _ := strconv.Atoi(r.URL.Query().Get("event_id"))
	tierID, _ := strconv.Atoi(r.URL.Query().Get("tier_id"))
	if eventID == 0 {
		eventID = 1
	}
	if tierID == 0 {
		tierID = 1
	}

	data := map[string]any{
		"Judul":   "Checkout Tiket - KampusTix",
		"User":    user,
		"Pesan":   r.URL.Query().Get("pesan"),
		"EventID": eventID,
		"TierID":  tierID,
	}
	h.rd.Tampil(w, "checkout/form.html", data)
}

// 1. POST /checkout (Memproses form pemesanan 1-4 tiket & kunci kuota 15 menit)
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
		http.Error(w, "Gagal membaca form", http.StatusBadRequest)
		return
	}

	eventID, _ := strconv.Atoi(r.FormValue("event_id"))
	tierID, _ := strconv.Atoi(r.FormValue("tier_id"))
	quantity, _ := strconv.Atoi(r.FormValue("quantity"))
	promoCode := r.FormValue("promo_code")

	names := r.Form["holder_name[]"]
	emails := r.Form["holder_email[]"]

	var holders []model.TicketHolderInput
	for i := 0; i < len(names) && i < len(emails); i++ {
		holders = append(holders, model.TicketHolderInput{
			HolderName:  names[i],
			HolderEmail: emails[i],
		})
	}

	req := model.CheckoutRequest{
		UserID:    userID,
		EventID:   eventID,
		TierID:    tierID,
		PromoCode: promoCode,
		Quantity:  quantity,
		Holders:   holders,
	}

	orderID, err := h.orderService.ProcessCheckout(r.Context(), req)
	if err != nil {
		pesanErr := url.QueryEscape("Checkout Gagal: " + err.Error())
		http.Redirect(w, r, "/checkout?event_id="+strconv.Itoa(eventID)+"&tier_id="+strconv.Itoa(tierID)+"&pesan="+pesanErr, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/orders/"+strconv.Itoa(orderID)+"/payment", http.StatusSeeOther)
}

// 2. GET /orders/{id}/payment (Halaman Konfirmasi Pembayaran & Countdown 15 Menit)
func (h *OrderHandler) ShowPaymentPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.CurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login?pesan=Silakan+masuk+terlebih+dahulu", http.StatusSeeOther)
		return
	}

	orderID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "ID Pesanan tidak valid", http.StatusBadRequest)
		return
	}

	pageData, err := h.orderService.GetOrderForPayment(r.Context(), orderID, int(user.ID))
	if err != nil {
		if err.Error() == "FORBIDDEN" {
			http.Error(w, "403 Forbidden: Anda bukan pemilik pesanan ini", http.StatusForbidden)
			return
		}
		http.Error(w, "Pesanan tidak ditemukan", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Judul":            "Konfirmasi Pembayaran - KampusTix",
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

// 3. POST /orders/{id}/pay-simulate (Eksekusi Simulasi Pembayaran Berhasil)
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
	if err != nil {
		http.Error(w, "ID Pesanan tidak valid", http.StatusBadRequest)
		return
	}

	err = h.orderService.SimulatePayment(r.Context(), orderID, userID)
	if err != nil {
		if err.Error() == "FORBIDDEN" {
			http.Error(w, "403 Forbidden: Anda tidak berhak membayar pesanan ini", http.StatusForbidden)
			return
		}
		pesanErr := url.QueryEscape("Pembayaran Gagal: " + err.Error())
		http.Redirect(w, r, "/orders/"+strconv.Itoa(orderID)+"/payment?pesan="+pesanErr, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/my-tickets?pesan=Pembayaran+Berhasil!+E-Tiket+Anda+Telah+Aktif", http.StatusSeeOther)
}
