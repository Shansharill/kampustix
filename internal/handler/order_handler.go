package handler

import (
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

// orderBusinessMessage mengambil pesan aman dari BusinessError.
// Error database biasa tidak dikembalikan ke browser.
func orderBusinessMessage(err error) (string, bool) {
	var businessErr *service.BusinessError
	if errors.As(err, &businessErr) && businessErr != nil {
		return businessErr.Message, true
	}
	return "", false
}

// checkoutURL mempertahankan event_id dan tier_id ketika menampilkan pesan error.
func checkoutURL(eventID, tierID int, queryKey, message string) string {
	values := url.Values{}
	if eventID > 0 {
		values.Set("event_id", strconv.Itoa(eventID))
	}
	if tierID > 0 {
		values.Set("tier_id", strconv.Itoa(tierID))
	}
	if queryKey != "" && message != "" {
		values.Set(queryKey, message)
	}
	if encoded := values.Encode(); encoded != "" {
		return "/checkout?" + encoded
	}
	return "/checkout"
}

// paymentURL membuat URL halaman pembayaran dengan query flash error/success.
func paymentURL(orderID int, queryKey, message string) string {
	path := "/orders/" + strconv.Itoa(orderID) + "/payment"
	if queryKey == "" || message == "" {
		return path
	}
	values := url.Values{}
	values.Set(queryKey, message)
	return path + "?" + values.Encode()
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

	data := map[string]any{
		"Title":         "Checkout Tiket - KampusTix",
		"User":          user,
		"Error":         r.URL.Query().Get("error"),
		"Success":       r.URL.Query().Get("success"),
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
		http.Redirect(w, r, checkoutURL(eventID, tierID, "error", "Event, tier, atau jumlah tiket tidak valid."), http.StatusSeeOther)
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
		if errors.Is(err, service.ErrForbidden) {
			http.Error(w, "403 Forbidden: akses pesanan ditolak", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrOrderNotFound) {
			http.NotFound(w, r)
			return
		}
		if message, ok := orderBusinessMessage(err); ok {
			http.Redirect(w, r, checkoutURL(eventID, tierID, "error", message), http.StatusSeeOther)
			return
		}

		log.Printf("gagal membuat order: user_id=%d event_id=%d tier_id=%d: %v", userID, eventID, tierID, err)
		http.Redirect(w, r, checkoutURL(eventID, tierID, "error", "Terjadi kesalahan pada server. Silakan coba lagi."), http.StatusSeeOther)
		return
	}

	// Pesan sukses ditampilkan melalui slot .Success pada layout.
	destination := paymentURL(orderID, "success", "Pesanan berhasil dibuat. Selesaikan pembayaran dalam 15 menit.")
	http.Redirect(w, r, destination, http.StatusSeeOther)
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
		if errors.Is(err, service.ErrForbidden) {
			http.Error(w, "403 Forbidden: Anda bukan pemilik pesanan ini", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrOrderNotFound) {
			http.NotFound(w, r)
			return
		}
		log.Printf("gagal membuka halaman pembayaran order %d: %v", orderID, err)
		http.Error(w, "Terjadi kesalahan pada server", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Title":            "Konfirmasi Pembayaran - KampusTix",
		"User":             user,
		"Error":            r.URL.Query().Get("error"),
		"Success":          r.URL.Query().Get("success"),
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
		if errors.Is(err, service.ErrForbidden) {
			http.Error(w, "403 Forbidden: Anda tidak berhak membayar pesanan ini", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrOrderNotFound) {
			http.NotFound(w, r)
			return
		}
		if message, ok := orderBusinessMessage(err); ok {
			http.Redirect(w, r, paymentURL(orderID, "error", message), http.StatusSeeOther)
			return
		}

		log.Printf("gagal memproses simulasi pembayaran order %d oleh user %d: %v", orderID, userID, err)
		http.Redirect(w, r, paymentURL(orderID, "error", "Terjadi kesalahan pada server. Silakan coba lagi."), http.StatusSeeOther)
		return
	}

	// Halaman payment membaca status PAID dan menampilkan banner statusnya sendiri.
	// Tidak memakai ?success agar tidak menampilkan banner sukses dua kali.
	http.Redirect(w, r, paymentURL(orderID, "", ""), http.StatusSeeOther)
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
		if errors.Is(err, service.ErrForbidden) {
			http.Error(w, "403 Forbidden: Anda bukan pemilik pesanan ini", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrOrderNotFound) {
			http.NotFound(w, r)
			return
		}
		if message, ok := orderBusinessMessage(err); ok {
			http.Redirect(w, r, paymentURL(orderID, "error", message), http.StatusSeeOther)
			return
		}

		log.Printf("gagal membatalkan order %d oleh user %d: %v", orderID, userID, err)
		http.Redirect(w, r, paymentURL(orderID, "error", "Terjadi kesalahan pada server. Silakan coba lagi."), http.StatusSeeOther)
		return
	}

	// Status CANCELLED sudah ditampilkan oleh payment.html, jadi tidak perlu banner sukses kedua.
	http.Redirect(w, r, paymentURL(orderID, "", ""), http.StatusSeeOther)
}
