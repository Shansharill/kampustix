package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"kampustix/internal/config"
	"kampustix/internal/handler"
	"kampustix/internal/middleware"
	"kampustix/internal/repository"
	"kampustix/internal/service"
)

func main() {
	// 1. TAHAP KONFIGURASI: Membaca file .env
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 2. TAHAP DATABASE: Buka koneksi ke MySQL
	db, err := bukaDB(cfg.DBDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 3. TAHAP VIEW: Inisialisasi template renderer HTML
	view := handler.NewRenderer()

	// 4. PERAKITAN DEPENDENCY (Anggota 1: User & Auth)
	userRepo := &repository.UserRepo{DB: db}
	authSvc := &service.AuthService{UserRepo: userRepo}
	authMw := middleware.NewAuthMiddleware(userRepo)
	authHandler := &handler.AuthHandler{AuthSvc: authSvc, View: view}

	// 4b. PERAKITAN DEPENDENCY (Anggota 6: Gatekeeper)
	gatekeeperRepo := &repository.GatekeeperRepo{DB: db}
	gatekeeperSvc := &service.GatekeeperService{Repo: gatekeeperRepo}
	gatekeeperHandler := &handler.GatekeeperHandler{
		GatekeeperSvc: gatekeeperSvc,
		View:          view,
	}

	// PERAKITAN DEPENDENCY (Anggota 3: Order & Checkout)
	orderRepo := repository.NewOrderRepository(db)
	orderSvc := service.NewOrderService(orderRepo)
	orderHandler := handler.NewOrderHandler(orderSvc, view)

	// 5. TAHAP PENDAFTARAN RUTE (ROUTING GO 1.22+)
	mux := http.NewServeMux()

	// --- A. Rute Publik / Katalog (Placeholder Anggota 2) ---
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		view.Tampil(w, "event/katalog.html", map[string]any{
			"Title": "Katalog Event - KampusTix",
			"User":  middleware.CurrentUser(r),
		})
	})

	// --- B. Rute Autentikasi (Anggota 1) ---
	// Rute /login & /register dibungkus 'RequireGuest':
	// Jika user sudah login, otomatis dilempar ke katalog (/)
	mux.Handle("GET /login", authMw.RequireGuest(http.HandlerFunc(authHandler.HalamanLoginRegister)))
	mux.Handle("POST /login", authMw.RequireGuest(http.HandlerFunc(authHandler.ProsesLogin)))
	mux.Handle("GET /register", authMw.RequireGuest(http.HandlerFunc(authHandler.HalamanLoginRegister)))
	mux.Handle("POST /register", authMw.RequireGuest(http.HandlerFunc(authHandler.ProsesRegister)))

	// Logout untuk user yang sedang login
	mux.HandleFunc("POST /logout", authHandler.ProsesLogout)

	// --- C. Rute Terproteksi Pembeli (Placeholder Anggota 3 & 5) ---
	mux.Handle("GET /my-tickets", authMw.RequireRole("BUYER")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view.Tampil(w, "tiket/tiket_saya.html", map[string]any{
			"Title": "Tiket Saya - KampusTix",
			"User":  middleware.CurrentUser(r),
		})
	})))

	// --- D. Rute Terproteksi Admin (Placeholder Anggota 2 & 4) ---
	mux.Handle("GET /admin/events", authMw.RequireRole("ADMIN")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view.Tampil(w, "event/admin_event.html", map[string]any{
			"Title": "Kelola Event - KampusTix Admin",
			"User":  middleware.CurrentUser(r),
		})
	})))
	mux.Handle("GET /admin/promos", authMw.RequireRole("ADMIN")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view.Tampil(w, "admin/promo.html", map[string]any{
			"Title": "Kelola Promo - KampusTix Admin",
			"User":  middleware.CurrentUser(r),
		})
	})))
	mux.Handle("GET /admin/reports/revenue", authMw.RequireRole("ADMIN")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view.Tampil(w, "admin/laporan.html", map[string]any{
			"Title": "Laporan Keuangan - KampusTix Admin",
			"User":  middleware.CurrentUser(r),
		})
	})))

	// --- E. Rute Terproteksi Gatekeeper / Pintu Masuk (Anggota 6) ---
	mux.Handle("GET /gatekeeper/scan", authMw.RequireRole("GATEKEEPER", "ADMIN")(http.HandlerFunc(gatekeeperHandler.ScanPage)))
	mux.Handle("POST /gatekeeper/validate", authMw.RequireRole("GATEKEEPER", "ADMIN")(http.HandlerFunc(gatekeeperHandler.ValidateTicket)))
	mux.Handle("GET /gatekeeper/ticket/{code}/print", authMw.RequireRole("GATEKEEPER", "ADMIN")(http.HandlerFunc(gatekeeperHandler.PrintTicket)))

	// --- F. Rute Terproteksi Buyer (Anggota 3: Order & Checkout) ---

	// Menampilkan halaman checkout
	mux.Handle("GET /checkout",
		authMw.RequireRole("BUYER")(
			http.HandlerFunc(orderHandler.ShowCheckoutForm),
		),
	)

	// Memproses checkout dan membuat pesanan
	mux.Handle("POST /checkout",
		authMw.RequireRole("BUYER")(
			http.HandlerFunc(orderHandler.HandleCheckout),
		),
	)

	// Menampilkan halaman pembayaran
	mux.Handle("GET /orders/{id}/payment",
		authMw.RequireRole("BUYER")(
			http.HandlerFunc(orderHandler.ShowPaymentPage),
		),
	)

	// Simulasi pembayaran berhasil
	mux.Handle("POST /orders/{id}/pay-simulate",
		authMw.RequireRole("BUYER")(
			http.HandlerFunc(orderHandler.HandlePaySimulate),
		),
	)

	// 6. MIDDLEWARE GLOBAL CHAINING
	// Request Masuk -> Logging -> Authenticate (Periksa Cookie) -> Mux Router
	handlerDenganAuth := authMw.Authenticate(mux)
	handlerUtama := middleware.Logging(handlerDenganAuth)

	// 7. JALANKAN SERVER
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handlerUtama,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("server KampusTix jalan di %s", cfg.Addr)
	log.Fatal(srv.ListenAndServe())
}

func bukaDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	log.Println("database MySQL terhubung")
	return db, nil
}
