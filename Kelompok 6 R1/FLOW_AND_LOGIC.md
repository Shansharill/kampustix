# Dokumentasi Alur Logika Sistem (System Flow & State Machine)
## Sistem Penjualan Tiket Event Musik Kampus (KampusTix)

Dokumen ini melengkapi dan merinci 2 diagram alur yang ada pada folder `Flow/`:
1. `pemweb-Flowchart Penentu Harga Aktif (Tier Rollover).png`
2. `pemweb-State Machine Pesanan & Tiket (15-Minute Hold).drawio.png`

---

## 1. Alur Penentu Harga Aktif Otomatis (*Tier Rollover*)

Berdasarkan flowchart di `pemweb-Flowchart Penentu Harga Aktif (Tier Rollover).png`:

```mermaid
flowchart TD
    Start(["Mulai: Request Detail Kategori Tiket"]) --> CleanUp["Lazy Cleanup: Batalkan order PENDING yang lewat expires_at<br/>& pulihkan used_quota"]
    CleanUp --> FetchTiers["Ambil semua tier pada kategori ini<br/>ORDER BY sequence_order ASC (Presale 1 -> Presale 2 -> Normal)"]
    FetchTiers --> Loop{"Iterasi Tier Berikutnya Masih Ada?"}
    Loop -- "Tidak Ada" --> SoldOut(["Tampilkan Status: SOLD OUT / Penjualan Ditutup"])
    Loop -- "Ada" --> CheckDate{"Cek Batas Waktu:<br/>NOW() <= tier.end_date?"}
    CheckDate -- "Tidak (Tanggal Lewat)" --> Loop
    CheckDate -- "Ya (Belum Lewat)" --> CheckQuota{"Cek Kuota:<br/>tier.used_quota < tier.quota?"}
    CheckQuota -- "Tidak (Kuota Habis)" --> Loop
    CheckQuota -- "Ya (Tersedia)" --> SetActive["Tetapkan tier ini sebagai HARGA AKTIF"]
    SetActive --> CalcRemain["Hitung Sisa Kuota Tier = quota - used_quota"]
    CalcRemain --> Render(["Render Badge Tier Aktif & Form Beli<br/>(Maksimal 4 Tiket, Sisa Kuota)"])
```

### Implementasi Query Go (Repository):
```sql
SELECT id, category_id, tier_name, price, quota, used_quota, sequence_order, end_date
FROM ticket_tiers
WHERE category_id = ?
  AND end_date >= NOW()
  AND used_quota < quota
ORDER BY sequence_order ASC
LIMIT 1;
```
*Jika query mengembalikan `sql.ErrNoRows`, maka kategori tiket dinyatakan **SOLD OUT**.*

---

## 2. Diagram State Machine Pesanan (`orders`) & Tiket (`tickets`)

Berdasarkan `pemweb-State Machine Pesanan & Tiket (15-Minute Hold).drawio.png`:

```mermaid
stateDiagram-v2
    [*] --> PENDING : Pembeli Submit Checkout (1-4 Tiket)<br/>[Kuot Tier Dikunci & Status Tiket RESERVED]
    
    state PENDING {
        [*] --> Countdown : Timer 15 Menit (expires_at)
    }

    PENDING --> PAID : Klik 'Konfirmasi Pembayaran' (NOW <= expires_at)<br/>[Tiket -> ACTIVE]
    PENDING --> EXPIRED : Waktu Habis (NOW > expires_at)<br/>[Tiket -> CANCELLED, Kuota Tier & Promo Dikembalikan]
    PENDING --> CANCELLED : Dibatalkan Pembeli Sebelum Bayar<br/>[Tiket -> CANCELLED & Kuota Dikembalikan]

    PAID --> REFUNDED_75 : Pembeli Ajukan Refund (Maks H-7)<br/>[Tiket -> REFUNDED, Kuota Tier Dikembalikan]
    PAID --> REFUNDED_100 : Admin Batalkan Event<br/>[Tiket -> REFUNDED 100%]
    PAID --> USED : Gatekeeper Scan QR Saat Hari-H<br/>[Check-in Berhasil 1x]

    EXPIRED --> [*]
    CANCELLED --> [*]
    REFUNDED_75 --> [*]
    REFUNDED_100 --> [*]
    USED --> [*]
```

### Matriks Transisi Status Tiket:

| Status Asal | Event / Aksi | Status Tujuan | Efek Samping (Side Effect) |
|---|---|---|---|
| *(None)* | Pembeli Checkout | `RESERVED` | `used_quota` tier bertambah sejumlah kuantiti. |
| `RESERVED` | Bayar Terkonfirmasi | `ACTIVE` | Tiket siap dicetak / QR Code aktif. |
| `RESERVED` | Expired / Dibatalkan | `CANCELLED` | `used_quota` tier dikurangi kembali; kuota promo dipulihkan. |
| `ACTIVE` | Pemindahan Tiket | `ACTIVE` | `holder_name` & `holder_email` berubah; `transfer_count` = 1 (fitur terkunci). |
| `ACTIVE` | Refund Mandiri (H-7) | `REFUNDED` | `used_quota` tier dikurangi 1 (bisa dibeli orang lain); dana kembali 75%. |
| `ACTIVE` | Event Dibatalkan | `REFUNDED` | Tiket mati; dana kembali 100%. Kuota tidak dilepas. |
| `ACTIVE` | Check-in Gatekeeper | `USED` | `checked_in_at = NOW()`; scan kedua ditolak. |

---

## 3. Alur Detail Validasi Gatekeeper (Pintu Masuk)

```mermaid
sequenceDiagram
    autonumber
    actor Penonton
    actor Gatekeeper
    participant Web as Sistem KampusTix
    participant DB as Database MySQL

    Penonton->>Gatekeeper: Menunjukkan E-Tiket (QR Code / Kode Unik)
    Gatekeeper->>Web: Scan / Input ticket_code di /gatekeeper/scan
    Web->>DB: UPDATE tickets SET status = 'USED', checked_in_at = NOW(), checked_in_by = ? WHERE ticket_code = ? AND status = 'ACTIVE'
    alt Rows Affected == 1
        DB-->>Web: Berhasil di-update
        Web-->>Gatekeeper: ✅ SUKSES (Suara Beep Hijau): Nama Pemegang & Status Valid
        Gatekeeper-->>Penonton: Silakan Masuk Venue
    else Rows Affected == 0
        Web->>DB: SELECT status, checked_in_at, holder_name FROM tickets WHERE ticket_code = ?
        DB-->>Web: Data Tiket Ditemukan
        alt Status == 'USED'
            Web-->>Gatekeeper: ❌ DITOLAK (Merah): Tiket Sudah Digunakan pada [checked_in_at]!
        else Status == 'REFUNDED' / 'CANCELLED'
            Web-->>Gatekeeper: ❌ DITOLAK: Tiket Tidak Berlaku (Telah Dibatalkan / Refund)
        else Tiket Tidak Ada
            Web-->>Gatekeeper: ❌ DITOLAK: Kode Tiket Tidak Dikenali!
        end
    end
```
