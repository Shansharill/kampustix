# DOKUMEN SPESIFIKASI KEBUTUHAN PERANGKAT LUNAK (PRD), RBAC & ARSITEKTUR LENGKAP
## Sistem Penjualan Tiket Event Musik Kampus (KampusTix)

---

- **Mata Kuliah:** Pemrograman Web
- **Topik Penugasan:** Topik 6 — Sistem Penjualan Tiket Event Komunitas Musik Kampus
- **Tech Stack:** Golang 1.22+ Server-Side Rendering (`html/template` + Bootstrap 5), MySQL 8 (InnoDB), Gorilla Sessions / Net-HTTP Cookie Auth
- **Arsitektur:** Monolitik Modular (*Vertical Slice Architecture* untuk 6 Anggota)
- **Status Dokumen:** Master Reference (Super Lengkap - Zero Omission)

---

## 1. PENDAHULUAN & LATAR BELAKANG BISNIS

### 1.1 Deskripsi & Tujuan Sistem
Sistem Penjualan Tiket Event Komunitas Musik Kampus (**KampusTix**) adalah aplikasi web berbasis *Server-Side Rendering* (SSR) yang dirancang untuk mengotomatisasi proses bisnis reservasi tiket konser musik dan seminar berskala kampus dengan kapasitas antara **300 hingga 1.000 penonton**.

Sebelum sistem ini dibangun, proses penjualan dilakukan secara konvensional menggunakan formulir daring (Google Form) dan verifikasi bukti transfer manual via aplikasi perpesanan. Cara lama ini menimbulkan kelemahan fatal:
1. **Overbooking (Kelebihan Kapasitas):** Terjadi penjualan tiket melebihi kapasitas fisik venue konser karena tidak adanya penguncian kuota secara *real-time*.
2. **Kekacauan Tahap Harga:** Harga presale (Presale 1, Presale 2, dan Normal) tidak berpindah secara otomatis ketika kuota habis atau batas waktu promosi terlewati.
3. **Tiket Ganda & Pemalsuan di Pintu Masuk:** Tidak adanya validasi terkomputerisasi yang menjamin satu tiket hanya dapat digunakan masuk satu kali (*single-use validation*).
4. **Kerugian Finansial & Inkonsistensi Pembukuan:** Kesulitan dalam melacak pemakaian kuota kode promo, pemotongan pengembalian dana mandiri (75%), dan refund massal ketika acara dibatalkan penyelenggara (100%).

KampusTix hadir untuk menyelesaikan permasalahan di atas dengan menerapkan:
- Mekanisme penguncian kuota sementara (**15-Minute Hold**).
- Otomatisasi rollover tahapan harga (**Tier Rollover Algorithm**).
- Pembersihan otomatis pesanan kedaluwarsa secara mandiri (**Lazy Expiration Engine tanpa Cron Job eksternal**).
- Tata kelola hak akses berbasis peran (**Role-Based Access Control / RBAC**) yang memisahkan tanggung jawab 4 peran pengguna.
- Audit rekonsiliasi keuangan bersih yang akurat.

---

## 2. PERAN PENGGUNA (USER ROLES) & SPESIFIKASI RBAC

Sistem mengimplementasikan 4 (empat) tingkatan peran pengguna dengan pemisahan tanggung jawab (*Separation of Duties*) yang tegas:

### 2.1 Deskripsi Aktor & Batasan Tanggung Jawab
1. **Guest (Publik):**
   - **Fungsi Utama:** Melakukan eksplorasi sistem sebelum memiliki akun, melihat katalog event aktif, membaca detail acara (jadwal, lokasi, sisa kuota tiket aktif), registrasi akun baru, dan autentikasi (login).
   - **Batas Tanggung Jawab:** Hanya memiliki hak baca (*Read-Only*) pada data publik. Tidak dapat melakukan reservasi tiket, melihat e-tiket, atau mengakses data pengguna lain.
   - **Catatan Keamanan:** Dilarang mengakses rute pemesanan (`/checkout`) atau halaman profil. Jika memaksa mengakses rute privat, sistem langsung mengarahkan (*redirect*) ke halaman Login.

2. **Pembeli (BUYER):**
   - **Fungsi Utama:** Melakukan pemesanan tiket (1 hingga 4 tiket per order), melakukan simulasi konfirmasi pembayaran (dalam batas waktu 15 menit), mengunduh e-tiket (kode QR unik / boarding pass), memindahkan kepemilikan tiket kepada orang lain (maksimal 1 kali), dan mengajukan pembatalan mandiri (*refund* 75% maksimal H-7 event).
   - **Batas Tanggung Jawab:** Hanya berhak melihat, memodifikasi, dan melakukan aksi terhadap entitas pesanan (`orders`) dan tiket (`tickets`) miliknya sendiri (`user_id` yang sesuai). Dilarang keras mengelola master event, mengubah status event, atau mengakses data finansial global.
   - **Catatan Keamanan:** Wajib melalui validasi kepemilikan data (*Owner Check*) di level backend untuk seluruh parameter ID rute guna mencegah celah keamanan **IDOR** (*Insecure Direct Object Reference*).

3. **Petugas Gate (GATEKEEPER):**
   - **Fungsi Utama:** Mengoperasikan antarmuka scanner/input kode tiket di pintu masuk pada hari penyelenggaraan event untuk memvalidasi e-tiket penonton secara instan.
   - **Batas Tanggung Jawab:** Hanya berhak membaca status tiket dan mengubah status tiket `ACTIVE` menjadi `USED`. Dilarang membeli tiket, dilarang melakukan refund, dan dilarang mengubah master data event.
   - **Catatan Keamanan:** Antarmuka dioptimalkan untuk kecepatan pemindaian. Akses sangat dibatasi hanya pada rute operasional pintu masuk (`/gatekeeper/*`).

4. **Penyelenggara (ADMIN):**
   - **Fungsi Utama:** Bertindak sebagai *superuser* yang mengelola keseluruhan master data (Event, Kategori Tiket, Tier Harga, Kode Promo), membatalkan event secara sepihak (otomatis memicu refund 100% untuk semua tiket aktif), dan memantau dasbor laporan keuangan (pendapatan bersih).
   - **Batas Tanggung Jawab:** Mengelola konfigurasi dan operasional sistem secara keseluruhan. Memiliki akses cadangan untuk melakukan scan tiket pintu masuk (bertindak ganda sebagai Gatekeeper jika diperlukan).
   - **Catatan Keamanan:** Otorisasi tingkat tertinggi. Validasi peran (`role == 'ADMIN'`) diterapkan secara global melalui middleware grup routing `/admin/*` untuk mencegah eskalasi privilege.

---

### 2.2 Matriks Hak Akses Literal (Fitur vs Role)

| Aksi / Fitur Sistem | Guest (Publik) | Pembeli (BUYER) | Petugas Gate (GATEKEEPER) | Penyelenggara (ADMIN) |
|---|:---:|:---:|:---:|:---:|
| **Melihat Katalog & Detail Event** | Boleh | Boleh | Boleh | Boleh |
| **Login & Registrasi Akun** | Boleh (Hanya saat session kosong) | Tidak Boleh | Tidak Boleh | Tidak Boleh |
| **Membuat Pesanan (Checkout)** | Tidak Boleh (Redirect /login) | Boleh (Maksimal 4 tiket per pesanan) | Tidak Boleh | Tidak Boleh |
| **Simulasi Pembayaran (15 Min Hold)** | Tidak Boleh | Boleh (Syarat: Pemilik pesanan sah, belum lewat 15 menit) | Tidak Boleh | Tidak Boleh |
| **Melihat Daftar & QR E-Tiket** | Tidak Boleh | Boleh (Syarat: Tiket miliknya sendiri) | Tidak Boleh | Tidak Boleh |
| **Transfer Kepemilikan Tiket** | Tidak Boleh | Boleh (Syarat: Status aktif, maks 1 kali transfer) | Tidak Boleh | Tidak Boleh |
| **Pembatalan Mandiri (Refund H-7)** | Tidak Boleh | Boleh (Syarat: Maks H-7 event, status aktif, refund 75%) | Tidak Boleh | Tidak Boleh |
| **Validasi Check-in di Pintu Masuk** | Tidak Boleh | Tidak Boleh | Boleh (Syarat: Status tiket Aktif, diubah jadi USED maks 1x) | Boleh (Akses Cadangan) |
| **Manajemen Master Event & Promo** | Tidak Boleh | Tidak Boleh | Tidak Boleh | Boleh |
| **Pembatalan Event Massal** | Tidak Boleh | Tidak Boleh | Tidak Boleh | Boleh (Efek: Refund 100% ke seluruh tiket aktif) |
| **Melihat Laporan Keuangan** | Tidak Boleh | Tidak Boleh | Tidak Boleh | Boleh |

---

### 2.3 Matriks Hak Akses Rute (Authorization Matrix & Route Contract)

| HTTP Method | URL Path | Role yang Diizinkan | Respon Sistem Jika Akses Ditolak | Validasi Khusus / Catatan Backend | Penanggung Jawab |
|---|---|---|---|---|:---:|
| `GET`, `POST` | `/login`, `/register` | Guest | Redirect ke `/` (Katalog Event) jika sudah memiliki session aktif. | Mencegah user yang sudah login mengakses kembali form auth. | Anggota 1 |
| `POST` | `/logout` | BUYER, ADMIN, GATEKEEPER | Redirect ke `/login` | Hapus session cookie sepenuhnya di browser dan server. | Anggota 1 |
| `GET` | `/` | Publik (Semua Role) | - (Akses Publik Bebas) | Tampilkan katalog event berstatus UPCOMING / ACTIVE. | Anggota 2 |
| `GET` | `/events/{id}` | Publik (Semua Role) | - (Akses Publik Bebas) | Tampilkan detail event, kategori tiket, dan tier harga aktif saat ini. | Anggota 2 |
| `GET` | `/admin/events` | ADMIN | Redirect ke `/login` (Guest), 403 Forbidden (Buyer/Gate) | List seluruh event beserta statistik singkat. | Anggota 2 |
| `POST` | `/admin/events` | ADMIN | 403 Forbidden | Input master event baru (Judul, deskripsi, tanggal, venue, total kuota). | Anggota 2 |
| `POST` | `/admin/events/{id}/update` | ADMIN | 403 Forbidden | Edit data event (Judul, tanggal, venue, status). | Anggota 2 |
| `POST` | `/admin/events/{id}/categories`| ADMIN | 403 Forbidden | Tambah kategori tiket (VIP, Festival) ke event tertentu. | Anggota 2 |
| `POST` | `/admin/categories/{id}/tiers` | ADMIN | 403 Forbidden | Tambah tier harga berurutan (Presale 1, Presale 2, Normal). | Anggota 2 |
| `POST` | `/admin/tiers/{id}/update` | ADMIN | 403 Forbidden | Update harga, kuota, atau batas tanggal tier harga. | Anggota 2 |
| `POST` | `/checkout` | BUYER | Redirect ke `/login` (Guest), 403 Forbidden (Admin/Gate) | Verifikasi kuota via `SELECT ... FOR UPDATE`, validasi array pemegang tiket (1-4 orang). | Anggota 3 |
| `GET` | `/orders/{id}/payment` | BUYER | Redirect ke `/login` / 403 Forbidden | **Owner Check:** Cocokkan ID Session dengan `orders.user_id`. Cek masa berlaku countdown 15 menit. | Anggota 3 |
| `POST` | `/orders/{id}/pay-simulate` | BUYER | Redirect ke `/login` / 403 Forbidden | **Owner Check:** Wajib `orders.user_id`. Update status order jadi `PAID` dan tiket jadi `ACTIVE`. | Anggota 3 |
| `GET` | `/admin/promos` | ADMIN | 403 Forbidden | List master kode promo aktif dan riwayat pemakaian. | Anggota 4 |
| `POST` | `/admin/promos` | ADMIN | 403 Forbidden | Tambah kode promo baru (kode, persentase diskon, kuota pakai, batas tanggal). | Anggota 4 |
| `POST` | `/admin/promos/{id}/toggle` | ADMIN | 403 Forbidden | Mengaktifkan/menonaktifkan kode promo secara manual. | Anggota 4 |
| `GET` | `/admin/reports/revenue` | ADMIN | 403 Forbidden | Dasbor analitik pendapatan: Kotor, Potongan Promo, Total Refund, dan Pendapatan Bersih. | Anggota 4 |
| `GET` | `/my-tickets` | BUYER | Redirect ke `/login` / 403 Forbidden | Query database: `WHERE user_id = ?` mengambil user ID dari session yang sedang aktif. | Anggota 5 |
| `POST` | `/tickets/{id}/transfer` | BUYER | Redirect ke `/login` / 403 Forbidden | **Owner Check:** Tolak jika `transfer_count >= 1` atau status tiket bukan `ACTIVE`. | Anggota 5 |
| `POST` | `/tickets/{id}/refund` | BUYER | Redirect ke `/login` / 403 Forbidden | **Owner Check:** Tolak jika tanggal saat ini melewati H-7 `event_date` atau status bukan `ACTIVE`. | Anggota 5 |
| `POST` | `/admin/events/{id}/cancel` | ADMIN | 403 Forbidden | Ubah status event menjadi `CANCELLED`. Pemicu refund massal 100% untuk semua tiket aktif. | Anggota 5 |
| `GET` | `/tickets/{code}/print` | BUYER | Redirect ke `/login` / 403 Forbidden | **Owner Check:** Cetak tiket resmi / boarding pass lengkap dengan QR Code unik. | Anggota 6 |
| `GET` | `/gatekeeper/scan` | GATEKEEPER, ADMIN | Redirect ke `/login` (Guest), 403 Forbidden (BUYER) | Antarmuka kamera scanner atau input kode tiket pintu masuk. | Anggota 6 |
| `POST` | `/gatekeeper/validate` | GATEKEEPER, ADMIN | 403 Forbidden | Validasi atomik status tiket `ACTIVE` -> `USED`. Deteksi & tolak duplikasi scan seketika. | Anggota 6 |

---

## 3. ATURAN BISNIS INTI & MEKANISME TEKNIS

### 3.1 Batas Pembelian & Identitas Pemegang Tiket
1. Satu pesanan (`orders`) hanya dapat memuat **1 hingga 4 tiket** dalam kategori dan tier yang sama.
2. Pembeli wajib mencantumkan data personal untuk masing-masing tiket:
   - Nama Lengkap Pemegang Tiket (`holder_name`)
   - Alamat Email Pemegang Tiket (`holder_email`)
3. Data tiket individual ini dicatat langsung di tabel `tickets` sehingga setiap penonton memiliki identitas tiket tersendiri saat masuk venue.

### 3.2 Penahanan Kuota 15 Menit (*15-Minute Hold*) & Anti-Overbooking
1. **Pemesanan Awal:**
   - Saat pembeli mengklik checkout, sistem membuat baris `orders` dengan status `PENDING` dan menetapkan `expires_at = NOW() + INTERVAL 15 MINUTE`.
   - Sistem secara bersamaan membuat baris pada tabel `tickets` sebanyak jumlah tiket yang dipesan dengan status `RESERVED`.
   - Kuota pada tier terkait dikunci dengan menambahkan `used_quota` di dalam transaksi database yang terisolasi (`SELECT ... FOR UPDATE`).
   - Jika pesanan memanfaatkan kode promo yang sah, `used_count` pada tabel `promo_codes` dinaikkan 1.
2. **Lazy Expiration Engine (Tanpa Kebutuhan Cron Job):**
   - Setiap kali sistem memproses query pengecekan kuota, saat katalog event dibuka, atau saat pembeli mengakses halaman pembayaran, sistem mengeksekusi query pembersihan otomatis:
     ```sql
     -- 1. Tandai order yang expired
     UPDATE orders SET status = 'EXPIRED' 
     WHERE status = 'PENDING' AND expires_at < NOW();

     -- 2. Batalkan tiket yang masih reserved pada order expired
     UPDATE tickets t
     JOIN orders o ON t.order_id = o.id
     SET t.status = 'CANCELLED'
     WHERE o.status = 'EXPIRED' AND t.status = 'RESERVED';

     -- 3. Kembalikan kuota tier
     UPDATE ticket_tiers tt
     JOIN orders o ON o.tier_id = tt.id
     SET tt.used_quota = tt.used_quota - o.quantity
     WHERE o.status = 'EXPIRED';

     -- 4. Kembalikan kuota pemakaian promo (jika ada)
     UPDATE promo_codes p
     JOIN orders o ON o.promo_code_id = p.id
     SET p.used_count = p.used_count - 1
     WHERE o.status = 'EXPIRED';
     ```
3. **Konfirmasi Pembayaran:**
   - Dalam rentang 15 menit (`NOW() <= expires_at`), pembeli mengonfirmasi pembayaran simulasi.
   - Status pesanan berubah menjadi `PAID` dan seluruh status tiket di dalamnya berubah dari `RESERVED` menjadi `ACTIVE`.
   - Jika pembeli secara manual membatalkan pesanan sebelum 15 menit, status pesanan menjadi `CANCELLED`, seluruh tiket menjadi `CANCELLED`, serta `used_quota` dan kuota promo segera dikembalikan.

### 3.3 Otomatisasi Rollover Tahapan Harga (*Tier Rollover*)
1. Setiap kategori tiket (misal: VIP, Festival) memiliki tahapan harga yang berurutan sesuai kolom `sequence_order`:
   - `sequence_order = 1` -> Presale 1
   - `sequence_order = 2` -> Presale 2
   - `sequence_order = 3` -> Normal
2. Setiap tier memiliki kuota tersendiri (`quota`) dan batas akhir tanggal penjualan (`end_date`).
3. **Algoritma Penentuan Harga Aktif:**
   Sistem mencari tier pada kategori terkait dengan `sequence_order` terkecil yang memenuhi dua kondisi:
   - `used_quota < quota` (kuota tier belum habis).
   - `NOW() <= end_date` (batas tanggal penjualan belum terlewati).
   Apabila Presale 1 telah habis kuotanya ATAU batas tanggalnya telah lewat, sistem otomatis mengalihkan penjualan ke Presale 2. Jika seluruh tahapan pada kategori tersebut habis/kedaluwarsa, kategori dinyatakan **SOLD OUT**.

### 3.4 Aturan Kode Promo & Perhitungan Diskon
1. Kode promo memiliki persentase diskon (`discount_percent`), batas kuota pemakaian (`max_uses`), status aktif (`is_active`), dan dapat dibatasi pada `category_id` tertentu (atau `NULL` jika berlaku universal untuk seluruh kategori di event tersebut).
2. **Perhitungan Finansial Diskon:**
   - $\text{Total Gross} = \text{Unit Base Price} \times \text{Quantity}$
   - $\text{Discount Amount} = \text{Total Gross} \times \left(\frac{\text{Discount Percent}}{100}\right)$
   - $\text{Total Net} = \text{Total Gross} - \text{Discount Amount}$
3. **Pencatatan Harga Bersih per Tiket (`unit_net_price`):**
   Setiap tiket di tabel `tickets` wajib mencatat harga bersih proporsional:
   $$\text{unit\_net\_price} = \text{unit\_base\_price} - \left(\frac{\text{Discount Amount}}{\text{Quantity}}\right)$$
   Pencatatan ini krusial agar pengembalian dana (*refund*) dihitung berdasarkan uang riil yang dibayarkan pembeli, bukan harga kotor sebelum diskon.

### 3.5 Kebijakan Pembatalan & Pengembalian Dana (*Refund*)
1. **Pengajuan Refund Mandiri oleh Pembeli:**
   - Syarat waktu: $\text{NOW}() \le \text{DATE\_SUB}(\text{event\_date}, \text{INTERVAL 7 DAY})$ (maksimal H-7 sebelum hari pelaksanaan event).
   - Syarat status: Tiket berstatus `ACTIVE` (belum pernah dilakukan check-in).
   - Besaran pengembalian: **75% dari harga beli bersih**:
     $$\text{Refund Amount} = \text{unit\_net\_price} \times 0.75$$
   - Status tiket berubah menjadi `REFUNDED`.
   - Tiket yang dibatalkan pembeli dapat dijual kembali: sistem mengurangi `used_quota` pada tier terkait sebanyak 1 (`used_quota = used_quota - 1`).
2. **Pembatalan Event oleh Penyelenggara (Admin):**
   - Apabila status event diubah menjadi `CANCELLED`, seluruh tiket yang berstatus `ACTIVE` pada event tersebut otomatis dibatalkan dan direfund **100%**:
     $$\text{Refund Amount} = \text{unit\_net\_price} \times 1.00$$
   - Karena acara batal total, tiket tidak dijual kembali ke publik.

### 3.6 Pemindahan Kepemilikan Tiket (*Ticket Transfer*)
1. Pemilik tiket berhak mengalihkan tiketnya kepada orang lain dengan mengubah `holder_name` dan `holder_email`.
2. Pengalihan kepemilikan hanya dapat dilakukan **maksimal 1 (satu) kali**:
   - Sistem memeriksa kondisi `transfer_count < 1`.
   - Setelah data pemegang tiket diperbarui, sistem mencatat `transfer_count = 1` dan `transferred_at = NOW()`.
   - Tombol transfer pada antarmuka pengguna otomatis dinonaktifkan/dikunci permanen.
3. Fitur transfer hanya dapat dilakukan selama tiket masih berstatus `ACTIVE`.

### 3.7 Pemeriksaan Tiket di Pintu Masuk (*Gate Check-In*)
1. Petugas pintu memeriksa tiket penonton menggunakan pemindaian QR Code atau pengetikan kode alfanumerik unik (`ticket_code`).
2. Syarat lolos: Tiket berstatus `ACTIVE` dan terdaftar pada event yang sedang diselenggarakan.
3. **Pemberian Tanda Single-Use:**
   - Saat verifikasi sukses, sistem mengubah status tiket menjadi `USED`, mencatat timestamp `checked_in_at = NOW()`, serta mencatat ID petugas `checked_in_by`.
   - Percobaan check-in kedua pada kode tiket yang sama langsung ditolak dengan peringatan visual berwarna merah dan menampilkan informasi waktu pemindaian pertama.

### 3.8 Formula Laporan Finansial & Pendapatan Bersih
Dasbor laporan keuangan wajib menampilkan 5 metrik utama:
1. **Pendapatan Kotor (Gross Revenue):**
   $$\text{Gross} = \sum \text{total\_gross} \quad \text{dari seluruh order berstatus PAID}$$
2. **Total Potongan Promo:**
   $$\text{Total Diskon} = \sum \text{discount\_amount} \quad \text{dari order berstatus PAID}$$
3. **Pendapatan Bersih Sebelum Refund:**
   $$\text{Net Before Refund} = \text{Gross} - \text{Total Diskon}$$
4. **Total Pengembalian Dana (Refund Outflow):**
   $$\text{Total Refund} = \sum \text{refund\_amount} \quad (\text{Refund Mandiri 75\%} + \text{Refund Pembatalan Event 100\%})$$
5. **Pendapatan Akhir Bersih (Final Net Revenue):**
   $$\text{Final Net Revenue} = \text{Net Before Refund} - \text{Total Refund}$$

---

## 4. MATRIKS PEMBAGIAN KERJA 6 ANGGOTA (VERTICAL SLICE ARCHITECTURE)

Struktur pengerjaan proyek dibagi secara vertikal (*vertical slice*), di mana setiap anggota memegang tanggung jawab penuh dari *Handler* (routing & request validation), *Service* (business logic), *Repository* (query SQL), hingga *View* (rendering template HTML Bootstrap 5):

```
+---------------------------------------------------------------------------------------------------+
|                                  ARSITEKTUR VERTICAL SLICE                                        |
+---------------------+---------------------+-------------------------------------------------------+
| Anggota             | Modul Utama         | Komponen Tanggung Jawab End-to-End                     |
+---------------------+---------------------+-------------------------------------------------------+
| ANGGOTA 1           | Auth, Middleware &  | - Koneksi Database MySQL & Session Engine              |
|                     | Core Architecture   | - Registrasi, Login & Logout (Bcrypt Password)         |
|                     |                     | - Role-Based Access Middleware (Guest, Buyer, Gate,   |
|                     |                     |   Admin) & Redirect Guard                             |
|                     |                     | - Base Layout Bootstrap 5, Navbar Dinamis & Flash      |
|                     |                     |   Message Banner                                      |
+---------------------+---------------------+-------------------------------------------------------+
| ANGGOTA 2           | Master Event &      | - CRUD Master Event & Kategori Tiket (Admin)           |
|                     | Tier Rollover       | - CRUD Tier Harga & Pengaturan Kuota/Tanggal (Admin)  |
|                     |                     | - Katalog Event Publik & Halaman Detail Event          |
|                     |                     | - Algoritma Penentuan Tier Harga Aktif Otomatis       |
+---------------------+---------------------+-------------------------------------------------------+
| ANGGOTA 3           | Checkout, Hold      | - Form Pemesanan Tiket (1-4 tiket per transaksi)      |
|                     | 15 Menit & Simulasi | - Input Nama & Email masing-masing pemegang tiket     |
|                     | Pembayaran          | - Locking Kuota Atomik (SELECT ... FOR UPDATE)        |
|                     |                     | - Lazy Expiration Engine (Rollback kuota & promo)     |
|                     |                     | - Halaman Simulasi Pembayaran & Countdown 15 Menit    |
+---------------------+---------------------+-------------------------------------------------------+
| ANGGOTA 4           | Manajemen Promo &   | - CRUD Kode Promo per Event & Kategori (Admin)        |
|                     | Laporan Keuangan    | - Validasi & Penerapan Diskon Promo saat Checkout     |
|                     |                     | - Dasbor Analitik Penjualan per Kategori & Tier       |
|                     |                     | - Rekonsiliasi Finansial Pendapatan Bersih            |
+---------------------+---------------------+-------------------------------------------------------+
| ANGGOTA 5           | Tiket Saya, Refund  | - Dasbor Tiket Pembeli (/my-tickets)                  |
|                     | Mandiri & Transfer  | - Fitur Transfer Kepemilikan Tiket (Maksimal 1 Kali)  |
|                     |                     | - Pengajuan Refund Mandiri H-7 Event (75%)            |
|                     |                     | - Pembatalan Massal Event oleh Admin (Refund 100%)    |
+---------------------+---------------------+-------------------------------------------------------+
| ANGGOTA 6           | E-Tiket Boarding    | - Render E-Tiket Resmi / Boarding Pass (Cetak/PDF)    |
|                     | Pass & Gatekeeper   | - Pembangkitan QR Code Unik per Tiket                 |
|                     | Check-in            | - Antarmuka Pemindai Pintu Masuk (/gatekeeper/scan)   |
|                     |                     | - Validasi Tiket Single-Use & Deteksi Duplikasi Scan  |
+---------------------+---------------------+-------------------------------------------------------+
```

---

## 5. SKEMA DATABASE KHUSUS MYSQL 8 INNODB (DDL & DBML)

### 5.1 Kepemilikan Tabel Database (CRUD Ownership Matrix)
- **Anggota 1:** `users`
- **Anggota 2:** `events`, `ticket_categories`, `ticket_tiers`
- **Anggota 3:** `orders`, `tickets` (Insert saat checkout & Update saat pembayaran berhasil)
- **Anggota 4:** `promo_codes` (beserta Read-Only Aggregation Query ke `orders`, `tickets`, `refunds`)
- **Anggota 5:** `refunds`, `tickets` (Update untuk transfer pemegang & status REFUNDED)
- **Anggota 6:** `tickets` (Update status menjadi USED dan pencatatan timestamp check-in)

---

### 5.2 Skrip SQL DDL Lengkap (MySQL 8 InnoDB)

```sql
-- Inisialisasi Database KampusTix
CREATE DATABASE IF NOT EXISTS kampustix DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE kampustix;

SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS refunds;
DROP TABLE IF EXISTS tickets;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS promo_codes;
DROP TABLE IF EXISTS ticket_tiers;
DROP TABLE IF EXISTS ticket_categories;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS users;
SET FOREIGN_KEY_CHECKS = 1;

-- 1. TABEL PENGGUNA (Auth & RBAC) - Milik Anggota 1
CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(120) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role ENUM('BUYER', 'ADMIN', 'GATEKEEPER') NOT NULL DEFAULT 'BUYER',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 2. TABEL EVENT MASTER - Milik Anggota 2
CREATE TABLE events (
    id INT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(150) NOT NULL,
    event_type ENUM('CONCERT', 'SEMINAR') NOT NULL DEFAULT 'CONCERT',
    description TEXT NULL,
    venue VARCHAR(150) NOT NULL,
    event_date DATETIME NOT NULL,
    total_capacity INT NOT NULL,
    status ENUM('UPCOMING', 'ACTIVE', 'COMPLETED', 'CANCELLED') NOT NULL DEFAULT 'UPCOMING',
    created_by INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. TABEL KATEGORI TIKET - Milik Anggota 2
CREATE TABLE ticket_categories (
    id INT AUTO_INCREMENT PRIMARY KEY,
    event_id INT NOT NULL,
    category_name VARCHAR(50) NOT NULL,
    total_quota INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 4. TABEL TAHAPAN HARGA (TIER ROLLOVER) - Milik Anggota 2
CREATE TABLE ticket_tiers (
    id INT AUTO_INCREMENT PRIMARY KEY,
    category_id INT NOT NULL,
    tier_name VARCHAR(50) NOT NULL,
    sequence_order INT NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    quota INT NOT NULL,
    used_quota INT NOT NULL DEFAULT 0,
    start_date DATETIME NOT NULL,
    end_date DATETIME NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES ticket_categories(id) ON DELETE CASCADE,
    CONSTRAINT chk_tier_quota CHECK (used_quota <= quota)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 5. TABEL KODE PROMO - Milik Anggota 4
CREATE TABLE promo_codes (
    id INT AUTO_INCREMENT PRIMARY KEY,
    event_id INT NOT NULL,
    category_id INT NULL,
    code VARCHAR(30) NOT NULL UNIQUE,
    discount_percent DECIMAL(5, 2) NOT NULL,
    max_uses INT NOT NULL,
    used_count INT NOT NULL DEFAULT 0,
    valid_until DATETIME NOT NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE,
    FOREIGN KEY (category_id) REFERENCES ticket_categories(id) ON DELETE SET NULL,
    CONSTRAINT chk_promo_uses CHECK (used_count <= max_uses)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 6. TABEL PESANAN (ORDERS - 15 MINUTE HOLD) - Milik Anggota 3
CREATE TABLE orders (
    id INT AUTO_INCREMENT PRIMARY KEY,
    order_code VARCHAR(30) NOT NULL UNIQUE,
    user_id INT NOT NULL,
    event_id INT NOT NULL,
    tier_id INT NOT NULL,
    promo_code_id INT NULL,
    quantity TINYINT NOT NULL,
    total_gross DECIMAL(10, 2) NOT NULL,
    discount_amount DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    total_net DECIMAL(10, 2) NOT NULL,
    status ENUM('PENDING', 'PAID', 'EXPIRED', 'CANCELLED') NOT NULL DEFAULT 'PENDING',
    expires_at DATETIME NOT NULL,
    paid_at DATETIME NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE RESTRICT,
    FOREIGN KEY (tier_id) REFERENCES ticket_tiers(id) ON DELETE RESTRICT,
    FOREIGN KEY (promo_code_id) REFERENCES promo_codes(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 7. TABEL TIKET INDIVIDUAL (BOARDING PASS) - Milik Anggota 3, 5, 6
CREATE TABLE tickets (
    id INT AUTO_INCREMENT PRIMARY KEY,
    order_id INT NOT NULL,
    event_id INT NOT NULL,
    category_id INT NOT NULL,
    tier_id INT NOT NULL,
    ticket_code VARCHAR(40) NOT NULL UNIQUE,
    holder_name VARCHAR(100) NOT NULL,
    holder_email VARCHAR(120) NOT NULL,
    unit_base_price DECIMAL(10, 2) NOT NULL,
    unit_net_price DECIMAL(10, 2) NOT NULL,
    transfer_count TINYINT NOT NULL DEFAULT 0,
    transferred_at DATETIME NULL,
    status ENUM('RESERVED', 'ACTIVE', 'USED', 'REFUNDED', 'CANCELLED') NOT NULL DEFAULT 'RESERVED',
    checked_in_at DATETIME NULL,
    checked_in_by INT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE RESTRICT,
    FOREIGN KEY (category_id) REFERENCES ticket_categories(id) ON DELETE RESTRICT,
    FOREIGN KEY (tier_id) REFERENCES ticket_tiers(id) ON DELETE RESTRICT,
    FOREIGN KEY (checked_in_by) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT chk_ticket_transfer CHECK (transfer_count <= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 8. TABEL REFUNDS (PENGEMBALIAN DANA) - Milik Anggota 5
CREATE TABLE refunds (
    id INT AUTO_INCREMENT PRIMARY KEY,
    ticket_id INT NOT NULL UNIQUE,
    order_id INT NOT NULL,
    requested_by INT NOT NULL,
    refund_percent DECIMAL(5, 2) NOT NULL,
    refund_amount DECIMAL(10, 2) NOT NULL,
    refund_type ENUM('BUYER_INITIATED', 'EVENT_CANCELLED') NOT NULL,
    reason VARCHAR(255) NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE RESTRICT,
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT,
    FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

---

### 5.3 Sintaks DBML (Untuk Visualisasi di dbdiagram.io)

```dbml
Table users {
  id int [pk, increment]
  name varchar(100) [not null]
  email varchar(120) [unique, not null]
  password_hash varchar(255) [not null]
  role varchar(20) [note: 'BUYER, ADMIN, GATEKEEPER', default: 'BUYER', not null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table events {
  id int [pk, increment]
  title varchar(150) [not null]
  event_type varchar(20) [note: 'CONCERT, SEMINAR', default: 'CONCERT', not null]
  description text
  venue varchar(150) [not null]
  event_date datetime [not null]
  total_capacity int [not null]
  status varchar(20) [note: 'UPCOMING, ACTIVE, COMPLETED, CANCELLED', default: 'UPCOMING', not null]
  created_by int [ref: > users.id, not null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table ticket_categories {
  id int [pk, increment]
  event_id int [ref: > events.id, not null]
  category_name varchar(50) [not null]
  total_quota int [not null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table ticket_tiers {
  id int [pk, increment]
  category_id int [ref: > ticket_categories.id, not null]
  tier_name varchar(50) [not null]
  sequence_order int [not null]
  price decimal(10,2) [not null]
  quota int [not null]
  used_quota int [default: 0, not null]
  start_date datetime [not null]
  end_date datetime [not null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table promo_codes {
  id int [pk, increment]
  event_id int [ref: > events.id, not null]
  category_id int [ref: > ticket_categories.id, null]
  code varchar(30) [unique, not null]
  discount_percent decimal(5,2) [not null]
  max_uses int [not null]
  used_count int [default: 0, not null]
  valid_until datetime [not null]
  is_active tinyint [default: 1, not null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table orders {
  id int [pk, increment]
  order_code varchar(30) [unique, not null]
  user_id int [ref: > users.id, not null]
  event_id int [ref: > events.id, not null]
  tier_id int [ref: > ticket_tiers.id, not null]
  promo_code_id int [ref: > promo_codes.id, null]
  quantity tinyint [not null]
  total_gross decimal(10,2) [not null]
  discount_amount decimal(10,2) [default: 0.00, not null]
  total_net decimal(10,2) [not null]
  status varchar(20) [note: 'PENDING, PAID, EXPIRED, CANCELLED', default: 'PENDING', not null]
  expires_at datetime [not null]
  paid_at datetime [null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table tickets {
  id int [pk, increment]
  ticket_code varchar(40) [unique, not null]
  order_id int [ref: > orders.id, not null]
  event_id int [ref: > events.id, not null]
  category_id int [ref: > ticket_categories.id, not null]
  tier_id int [ref: > ticket_tiers.id, not null]
  unit_base_price decimal(10,2) [not null]
  unit_net_price decimal(10,2) [not null]
  holder_name varchar(100) [not null]
  holder_email varchar(120) [not null]
  transfer_count tinyint [default: 0, not null]
  transferred_at datetime [null]
  status varchar(20) [note: 'RESERVED, ACTIVE, USED, REFUNDED, CANCELLED', default: 'RESERVED', not null]
  checked_in_at datetime [null]
  checked_in_by int [ref: > users.id, null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}

Table refunds {
  id int [pk, increment]
  ticket_id int [ref: - tickets.id, not null, unique]
  order_id int [ref: > orders.id, not null]
  requested_by int [ref: > users.id, not null]
  refund_percent decimal(5,2) [not null]
  refund_amount decimal(10,2) [not null]
  refund_type varchar(30) [note: 'BUYER_INITIATED, EVENT_CANCELLED', not null]
  reason varchar(255) [null]
  created_at timestamp [default: `CURRENT_TIMESTAMP`]
}
```

---

## 6. PEMETAAN ANTARMUKA PENGGUNA (FIGMA UI SPECIFICATION)

Setiap halaman di desain Figma (`KOe5FIiJDhIPN2h82X3fAr`) berkorespondensi 1-ke-1 dengan rute dan modul:

| No | Frame ID | Nama Layar Figma | Route URL | Aktor | Elemen Kunci UI |
|:---:|---|---|---|:---:|---|
| 1 | `1:2` | KampusTix Authentication Screen | `/login`, `/register` | Guest | Tab switching login/registrasi, form input (email, nama, password), dismissible alert banner. |
| 2 | `1:626` | Katalog Event KampusTix | `/` | Publik | Hero banner, grid kartu event (banner, tanggal, venue, harga mulai dari), badge status event. |
| 3 | `1:120` | Detail Event - Annual Spring Symphonia | `/events/{id}` | Publik | Header event, rundown/deskripsi, kartu kategori tiket (VIP/Festival), indikator sisa kuota, badge harga aktif (Presale 1/Normal), tombol checkout. |
| 4 | `1:1142`| Checkout Tiket - KampusTix | `/checkout` | BUYER | Stepper alur pemesanan, form dinamis 1-4 nama & email pemegang tiket, input kode promo & tombol validasi, rincian subtotal & potongan. |
| 5 | `1:1570`| Konfirmasi Pembayaran - KampusTix | `/orders/{id}/payment` | BUYER | Timer mundur 15 menit (*real-time countdown*), nomor pesanan unik, rincian rekening/QRIS simulasi, tombol "Simulasikan Pembayaran Berhasil" & tombol batal. |
| 6 | `1:1903`| Admin Dashboard - KampusTix | `/admin/events`, `/admin/reports/revenue` | ADMIN | Metrik ringkasan kartu (Total Terjual, Gross, Diskon, Refund, Net Revenue), tabel CRUD event & tier, log transaksi terbaru. |
| 7 | `1:2537`| Tiket Saya - KampusTix | `/my-tickets` | BUYER | Tab filter tiket (Aktif, Riwayat, Dibatalkan), kartu tiket dengan tombol aksi: "Lihat E-Tiket", "Transfer Pemegang" (badge limit 1x), "Ajukan Refund" (badge H-7). |
| 8 | `1:3107`| Verifikasi Check-in Gatekeeper | `/gatekeeper/scan` | GATEKEEPER | Viewport pemindai kamera QR / kolom input manual kode tiket, kartu status verifikasi (Hijau: VALID + detail pemegang; Merah: DITOLAK + log waktu pemindaian terdahulu). |
| 9 | `1:3613`| E-Tiket Resmi (Boarding Pass) | `/tickets/{code}/print` | BUYER | Layout cetak ramah kertas A4/mobile, QR code SVG beresolusi tinggi, barcode alfanumerik unik, syarat & ketentuan masuk konser. |

---

## 7. KESIMPULAN & CHECKLIST KESIAPAN PRODUKSI

Dengan dokumen ini, seluruh aspek sistem telah didefinisikan secara tuntas:
1. **Tidak ada ambiguitas logika:** Penanganan kuota 15 menit, pembatalan, tier rollover, dan pembagian diskon promo proporsional telah terikat oleh aturan matematika dan query database yang pasti.
2. **Keamanan terjamin:** Dilengkapi pencegahan celah IDOR (Owner Check), pencegahan privilege escalation via RBAC Middleware, serta perlindungan ACID Transaction (`SELECT ... FOR UPDATE`).
3. **Pemisahan tugas adil dan independen:** Matriks *Vertical Slice Architecture* untuk 6 anggota mempermudah pengerjaan paralel di Git tanpa resiko *merge conflict* yang tinggi.


---

## 8. STRUKTUR FOLDER PROYEK & ALOKASI FILE TIAP ANGGOTA (SKELETON & CANVAS TEMPLATE)

Untuk memfasilitasi proses belajar bersama dan mencegah konflik pengerjaan (*merge conflict*) di Git, proyek menggunakan strategi **Shared Skeleton with Canvas Placeholders**. File referensi kode bawaan dosen dipertahankan dengan awalan `d_` sebagai bahan komparasi.

### 8.1 Kebijakan File Referensi Dosen (`d_*`)
Seluruh file proyek awal dosen di-rename dengan prefix `d_` agar tidak bentrok dengan modul baru:
- `db/schema.sql` $\to$ `db/d_schema.sql`
- `internal/model/produk.go` $\to$ `internal/model/d_produk.go`
- `internal/repository/produk.go` $\to$ `internal/repository/d_produk.go`
- `internal/service/produk.go` $\to$ `internal/service/d_produk.go`
- `internal/handler/produk.go` $\to$ `internal/handler/d_produk.go`
- `internal/handler/halaman.go` $\to$ `internal/handler/d_halaman.go`
- `templates/home.html` $\to$ `templates/d_home.html`
- `templates/profil.html` $\to$ `templates/d_profil.html`
- `templates/layout.html` $\to$ `templates/d_layout.html`
- `templates/produk/` $\to$ `templates/d_produk/`

---

### 8.2 Matriks Alokasi File Per Anggota (End-to-End Ownership)

| Anggota | Modul Utama | Lapisan Model | Lapisan Repository | Lapisan Service | Lapisan Handler | Lapisan Template View (`templates/`) | Layar Figma Terkait |
|:---:|---|---|---|---|---|---|---|
| **Pondasi Bersama** | Core Engine | - | - | - | `render.go` | `layout.html`, `footer.html` | Header/Nav (1:3) |
| **Anggota 1** | Auth, Middleware & Session | `model/user.go` | `repository/user_repo.go` | `service/auth_service.go` | `handler/auth_handler.go`, `middleware/auth.go` | `auth/login_register.html` | 1:2 (Auth Screen) |
| **Anggota 2** | Master Event & Tier Rollover | `model/event.go` | `repository/event_repo.go` | `service/event_service.go` | `handler/event_handler.go` | `event/katalog.html`, `event/detail.html`, `event/admin_event.html` | 1:626 (Katalog), 1:120 (Detail) |
| **Anggota 3** | Checkout, 15-Min Hold & Simulasi Bayar | `model/order.go` | `repository/order_repo.go` | `service/order_service.go` | `handler/order_handler.go` | `checkout/form.html`, `checkout/payment.html` | 1:1142 (Checkout), 1:1570 (Payment) |
| **Anggota 4** | Manajemen Promo & Laporan Finansial | `model/promo_report.go` | `repository/promo_report_repo.go` | `service/promo_report_service.go` | `handler/report_handler.go` | `admin/promo.html`, `admin/laporan.html` | 1:1903 (Dashboard Keuangan) |
| **Anggota 5** | Tiket Saya, Refund Mandiri & Transfer | `model/ticket_refund.go` | `repository/refund_repo.go` | `service/ticket_service.go` | `handler/ticket_handler.go` | `tiket/tiket_saya.html` | 1:2537 (Tiket Saya) |
| **Anggota 6** | E-Tiket Boarding Pass & Scan Gatekeeper | `model/gatekeeper.go` | `repository/gatekeeper_repo.go` | `service/gatekeeper_service.go` | `handler/gatekeeper_handler.go` | `gatekeeper/scan.html`, `gatekeeper/print.html` | 1:3107 (Scan), 1:3613 (Boarding Pass) |

---

### 8.3 Rincian Tanggung Jawab Kode Tiap File

#### 1. Core & Anggota 1 (Auth & Keamanan)
- `internal/model/user.go`: Struct data tabel `users` (ID, Name, Email, PasswordHash, Role, CreatedAt).
- `internal/repository/user_repo.go`: Query SQL pencarian user by email (`SELECT`), registrasi user baru (`INSERT`).
- `internal/service/auth_service.go`: Logika hashing password (`bcrypt.GenerateFromPassword`), verifikasi password (`bcrypt.CompareHashAndPassword`), penerbitan sesi login.
- `internal/middleware/auth.go`: Middleware pemeriksa session cookie, proteksi rute berbasis role (Guest only, RequireAuth, RequireBuyer, RequireAdmin, RequireGatekeeper).
- `internal/handler/auth_handler.go`: Handler form login & registrasi (`GET/POST /login`, `GET/POST /register`, `POST /logout`).
- `templates/auth/login_register.html`: Tampilan form login dan registrasi dengan fitur tab switching dan alert banner error/sukses.

#### 2. Anggota 2 (Master Event, Kategori & Tier Rollover)
- `internal/model/event.go`: Struct data `Event`, `TicketCategory`, `TicketTier`, dan DTO view `EventDetail`.
- `internal/repository/event_repo.go`: Query list katalog event aktif, query detail event beserta relasi kategori, query SQL penentu tier aktif (`sequence_order ASC`, `used_quota < quota`, `end_date >= NOW()`), dan operasi CRUD Admin.
- `internal/service/event_service.go`: Algoritma otomatisasi tier rollover harga, kalkulasi sisa kuota kategori.
- `internal/handler/event_handler.go`: Handler rute publik (`GET /`, `GET /events/{id}`) dan rute admin (`GET/POST /admin/events`, `/admin/events/{id}/categories`, dll).
- `templates/event/`:
  - `katalog.html`: Grid kartu event dengan filter dan badge harga mulai dari.
  - `detail.html`: Halaman info acara, lineup/rundown, opsi kategori (Festival, VIP), dan tier harga yang aktif saat ini.
  - `admin_event.html`: Antarmuka formulir kelola event, kategori, dan tahapan harga bagi admin.

#### 3. Anggota 3 (Checkout, 15-Minute Hold & Simulasi Pembayaran)
- `internal/model/order.go`: Struct data `Order` dan input DTO array pemegang tiket (1-4 orang: nama & email).
- `internal/repository/order_repo.go`: Transaksi atomik database (`SELECT ... FOR UPDATE` pada `ticket_tiers`), insert `orders` (status `PENDING`, `expires_at = NOW() + 15 MINUTE`), insert `tickets` (status `RESERVED`), update status `PAID` saat bayar, dan query pembersihan *Lazy Expiration*.
- `internal/service/order_service.go`: Validasi form batas 1-4 tiket, validasi masa aktif hold 15 menit, eksekusi pembersihan lazy expiration saat transaksi dibuka.
- `internal/handler/order_handler.go`: Handler rute pemesanan (`POST /checkout`), halaman tunggu bayar (`GET /orders/{id}/payment`), dan aksi bayar (`POST /orders/{id}/pay-simulate`).
- `templates/checkout/`:
  - `form.html`: Form input data pemesan dan nama/email tiket dinamis (1-4 baris), input kode voucher promo.
  - `payment.html`: Halaman instruksi transfer / QRIS simulasi lengkap dengan widget countdown waktu 15 menit dan tombol eksekusi simulasi bayar.

#### 4. Anggota 4 (Manajemen Kode Promo & Laporan Finansial)
- `internal/model/promo_report.go`: Struct data `PromoCode` dan DTO agregasi laporan pendapatan (`RevenueReport`).
- `internal/repository/promo_report_repo.go`: Query CRUD master promo, query validasi kode promo saat checkout (`used_count < max_uses` dan tanggal valid), query agregasi finansial (Gross Revenue, Total Potongan Promo, Total Refund, Pendapatan Bersih).
- `internal/service/promo_report_service.go`: Logika kalkulasi diskon persentase proporsional ke tiap unit tiket, perhitungan net revenue akhir.
- `internal/handler/report_handler.go`: Handler kelola promo (`GET/POST /admin/promos`, toggle aktif) dan dasbor laporan keuangan (`GET /admin/reports/revenue`).
- `templates/admin/`:
  - `promo.html`: Tabel kode promo, kuota pemakaian, dan formulir tambah diskon.
  - `laporan.html`: Dasbor analitik finansial dengan kartu KPI (Gross, Diskon, Refund, Net) dan tabel rincian omzet per kategori/tier.

#### 5. Anggota 5 (Tiket Saya, Transfer Tiket & Pengembalian Dana)
- `internal/model/ticket_refund.go`: Struct data tiket pembeli dan permohonan `Refund`.
- `internal/repository/refund_repo.go`: Query tiket milik user (`WHERE user_id = ?`), query update nama/email pemegang tiket dengan batas `transfer_count < 1`, query pengajuan refund mandiri 75% dengan validasi H-7 acara, dan query pembatalan massal event 100%.
- `internal/service/ticket_service.go`: Logika verifikasi selisih hari H-7 (`event_date - NOW() >= 7 hari`), penguncian tombol transfer setelah 1 kali pakai.
- `internal/handler/ticket_handler.go`: Handler tiket pembeli (`GET /my-tickets`), aksi transfer (`POST /tickets/{id}/transfer`), aksi refund pembeli (`POST /tickets/{id}/refund`), dan pembatalan event admin (`POST /admin/events/{id}/cancel`).
- `templates/tiket/`:
  - `tiket_saya.html`: Daftar kartu tiket aktif milik pembeli dengan tombol aksi transfer nama (maks 1x) dan tombol ajukan refund.

#### 6. Anggota 6 (E-Tiket Boarding Pass & Validasi Gatekeeper)
- `internal/model/gatekeeper.go`: Struct audit log gatekeeper dan tiket boarding pass.
- `internal/repository/gatekeeper_repo.go`: Query tiket by `ticket_code`, query atomik check-in (`UPDATE tickets SET status = 'USED', checked_in_at = NOW(), checked_in_by = ? WHERE ticket_code = ? AND status = 'ACTIVE'`).
- `internal/service/gatekeeper_service.go`: Logika penolakan scan duplikasi (mengecek apakah tiket sudah berstatus `USED` dan menampilkan jam scan sebelumnya).
- `internal/handler/gatekeeper_handler.go`: Handler pencetakan e-tiket (`GET /tickets/{code}/print`) dan antarmuka scanner pintu masuk (`GET /gatekeeper/scan`, `POST /gatekeeper/validate`).
- `templates/gatekeeper/`:
  - `scan.html`: Antarmuka viewport scanner / input kode tiket dengan feedback visual (Hijau untuk valid, Merah untuk ditolak/bekas).
  - `print.html`: Layout e-tiket resmi (boarding pass) yang ramah cetak, dilengkapi kode QR dan syarat masuk venue.
