# ERP Mini — Go + PostgreSQL

Sistem ERP sederhana untuk usaha kecil: produk & persediaan, supplier, pembelian, penjualan, dan laporan keuangan, dengan hak akses berbasis peran. Backend ditulis dengan Go (hanya `net/http` standard library) dan PostgreSQL, frontend berupa single-page app yang di-embed ke dalam binary.

## Fitur

- **Produk & persediaan** — CRUD, pencarian, pagination
- **Supplier** — CRUD, filter status, pencarian
- **Pembelian** — banyak item per transaksi; saat diterima, stok bertambah dan kas keluar tercatat otomatis
- **Penjualan** — tunai / transfer / QRIS; stok berkurang dan kas masuk tercatat otomatis dalam satu transaksi database; dapat dibatalkan
- **Keuangan** — arus kas, laba rugi, dan neraca dengan filter tanggal
- **Autentikasi & RBAC** — sesi berbasis cookie, password di-hash dengan bcrypt, lima role
- **Integrasi n8n & WhatsApp** — notifikasi penjualan, laporan harian, dan peringatan stok menipis
- Migrasi database otomatis saat aplikasi dijalankan, graceful shutdown, health check, middleware logger/recover/CORS

## Tech Stack

Go 1.26 · `net/http` · PostgreSQL 16 · pgx v5 · bcrypt · n8n (opsional) · Docker (opsional)

## Arsitektur

Berlapis: **handler → service → repository → database**. Modul yang lebih baru (supplier, pembelian, penjualan, keuangan) memakai handler dengan query langsung dan transaksi database.

```
.
├── main.go                    # entry point
├── web/                       # frontend (embed ke binary)
├── n8n/                       # workflow n8n siap import
├── docker-compose.yml         # PostgreSQL (+ n8n, profil "automation")
├── api.http                   # kumpulan request uji
└── internal/
    ├── config/                # konfigurasi dari environment
    ├── database/              # koneksi pool + migrasi SQL
    ├── handler/               # HTTP handler tiap modul
    ├── middleware/            # sesi, role, logger, recover, CORS
    ├── models/ repository/ service/
    ├── notify/                # pengirim webhook keluar
    └── router/                # routing & aturan akses
```

## Menjalankan

Prasyarat: Go 1.26+ dan PostgreSQL 16 (atau Docker).

```bash
docker compose up -d          # PostgreSQL (lewati jika sudah punya PostgreSQL sendiri)
cp .env.example .env          # Windows PowerShell: Copy-Item .env.example .env
go mod tidy
go run .
```

Aplikasi berjalan di `http://localhost:8080`. Cek kesehatan: `GET /health`. Akun admin dibuat otomatis dari `ADMIN_USERNAME` / `ADMIN_PASSWORD` pada `.env`.

Menjalankan test: `go test ./...`

## Konfigurasi

| Variabel | Keterangan |
|---|---|
| `APP_PORT` | Port aplikasi (default `8080`) |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | Koneksi PostgreSQL |
| `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `ADMIN_FULL_NAME` | Admin awal |
| `INTEGRATION_API_KEY` | Kunci akses endpoint integrasi (opsional) |
| `SALE_WEBHOOK_URL`, `SALE_WEBHOOK_SECRET` | Tujuan webhook event penjualan (opsional) |

> `.env` berisi kredensial dan tidak boleh di-commit.

## Peran & Hak Akses

| Role | Akses |
|---|---|
| `admin` | Semua fitur, termasuk manajemen pengguna |
| `gudang` | Kelola produk & persediaan |
| `supplier` | Kelola data supplier |
| `kasir` | Catat penjualan dan pembelian; hanya-lihat untuk produk & supplier |
| `keuangan` | Transaksi kas, arus kas, laba rugi, neraca |

## API

Semua endpoint `/api/v1/*` (kecuali login) memerlukan sesi login dan dibatasi per role. Format respons seragam: `{ "success", "message", "data", "meta", "errors" }`.

| Modul | Endpoint |
|---|---|
| Auth | `POST /api/v1/auth/login` · `POST /api/v1/auth/logout` · `GET /api/v1/auth/me` |
| Produk | `GET, POST /api/v1/products` · `GET, PUT, DELETE /api/v1/products/{id}` |
| Supplier | `GET, POST /api/v1/suppliers` · `GET, PUT, DELETE /api/v1/suppliers/{id}` |
| Pembelian | `GET, POST /api/v1/purchases` · `GET /{id}` · `POST /{id}/receive` · `POST /{id}/cancel` |
| Penjualan | `GET, POST /api/v1/sales` · `GET /{id}` · `POST /{id}/cancel` |
| Keuangan | `GET, POST /api/v1/finance/transactions` · `DELETE /{id}` · `GET /api/v1/finance/reports?from=&to=` |
| Pengguna | `GET, POST /api/v1/users` · `PUT /api/v1/users/{id}` (admin) |

Daftar `GET` mendukung `page`, `limit`, dan (sesuai modul) `search` / `status`. Contoh siap pakai tersedia di `api.http`.

## Integrasi n8n & WhatsApp

Aplikasi dapat menghubungkan proses bisnis ke WhatsApp melalui [n8n](https://n8n.io) dan gateway WhatsApp (mis. Fonnte). Integrasi bersifat opsional dan tidak memengaruhi fungsi utama.

```
Aplikasi ──webhook sale.created──▶ n8n ──▶ WhatsApp gateway ──▶ WhatsApp
Aplikasi ◀──/api/integration (API key)── n8n (terjadwal / pesan masuk)
```

**Endpoint integrasi** (hanya-baca, autentikasi header `X-API-Key`):

| Endpoint | Fungsi |
|---|---|
| `GET /api/integration/summary?date=` | Ringkasan harian: omzet, jumlah transaksi, kas masuk/keluar |
| `GET /api/integration/low-stock?threshold=` | Produk dengan stok di bawah batas |
| `GET /api/integration/stock?q=` | Pencarian stok produk |

**Event keluar:** `sale.created` dikirim (POST JSON, header opsional `X-Webhook-Secret`) ke `SALE_WEBHOOK_URL` setiap penjualan tercatat, berisi nomor invoice, pelanggan, daftar barang, dan total.

**Workflow n8n** (folder `n8n/`, import langsung dari UI n8n):

| Berkas | Fungsi |
|---|---|
| `1-notif-penjualan.json` | Kirim notifikasi WhatsApp untuk setiap penjualan baru |
| `2-laporan-harian.json` | Laporan harian terjadwal dan peringatan stok menipis |
| `3-bot-cek-stok.json` | Balas pesan `stok <nama produk>` dengan ketersediaan stok |

Workflow membaca konfigurasi dari environment n8n: `APP_URL`, `INTEGRATION_API_KEY`, `OWNER_WA`, `FONNTE_TOKEN`. n8n dapat dijalankan lewat `docker compose --profile automation up -d` atau `npx n8n`.

## Catatan Teknis

- Harga penjualan diambil dari database (bukan dari klien); stok tidak cukup ditolak.
- Penjualan dan pembelian dieksekusi dalam satu transaksi database.
- Neraca menyederhanakan penilaian persediaan (harga jual × stok); selisihnya terhadap total pembelian ditampilkan sebagai "Selisih penilaian persediaan" pada ekuitas.

## Rencana Pengembangan

- Dockerfile untuk deployment
- Test integrasi dengan database sungguhan
- Dokumentasi OpenAPI/Swagger
- Pembatasan percobaan login (rate limiting)
- Ekspor laporan ke Excel/PDF