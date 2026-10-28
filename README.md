# First Project – REST API Golang + PostgreSQL

REST API manajemen produk dengan operasi CRUD lengkap, dibuat sebagai project portofolio Golang.

## Fitur

- CRUD produk (Create, Read, Update, Delete)
- Pagination & pencarian (`?page=1&limit=10&search=mouse`)
- Arsitektur berlapis: **handler → service → repository → database**
- Validasi input + response error yang konsisten
- Migrasi database otomatis saat aplikasi dijalankan (embed SQL)
- Middleware: logger, panic recovery, CORS
- Health check (`/health`) yang juga mengecek koneksi database
- Graceful shutdown
- Unit test untuk service (tanpa perlu database)
- Hanya 2 dependency: `pgx` (driver PostgreSQL) dan `godotenv`

## Integrasi n8n & WhatsApp (baru)

- Notifikasi WA tiap penjualan baru, laporan harian jam 20:00, peringatan stok menipis
- Bot WA (Fonnte): kirim `stok mouse` → dibalas sisa stok
- Endpoint read-only `/api/integration/*` (API key) + webhook `sale.created`
- Workflow n8n siap import di folder `n8n/`

Panduan lengkap step by step: [PANDUAN-n8n-WA.md](PANDUAN-n8n-WA.md)

## Tech Stack

Go 1.22+ · `net/http` (standard library) · PostgreSQL 16 · pgx v5 · Docker

## Struktur Folder

```
first-project/
├── main.go                       # entry point, merakit semua layer
├── go.mod
├── docker-compose.yml            # PostgreSQL via Docker
├── .env.example                  # contoh konfigurasi
├── api.http                      # kumpulan request untuk tes API
└── internal/
    ├── config/config.go          # baca konfigurasi dari .env
    ├── database/
    │   ├── postgres.go           # koneksi pool + migrasi
    │   └── migrations/           # file SQL (001_..., 002_...)
    ├── models/product.go         # struct data
    ├── repository/               # query SQL (akses database)
    ├── service/                  # business logic + validasi (+ test)
    ├── handler/                  # HTTP handler & format response
    ├── middleware/               # logger, recover, CORS
    └── router/router.go          # daftar route
```

## Cara Menjalankan

1. Jalankan PostgreSQL: `docker compose up -d`
2. Salin konfigurasi: `copy .env.example .env` (PowerShell: `Copy-Item .env.example .env`)
3. Unduh dependency: `go mod tidy`
4. Jalankan: `go run .`
5. Buka http://localhost:8080/health

Tanpa Docker: install PostgreSQL, buat database `first_project`, lalu sesuaikan `.env`.

## Endpoint

| Method | Path                    | Keterangan                  |
|--------|-------------------------|-----------------------------|
| GET    | `/health`               | Cek server & database       |
| POST   | `/api/v1/products`      | Tambah produk               |
| GET    | `/api/v1/products`      | Daftar produk (pagination)  |
| GET    | `/api/v1/products/{id}` | Detail produk               |
| PUT    | `/api/v1/products/{id}` | Ubah produk                 |
| DELETE | `/api/v1/products/{id}` | Hapus produk                |

## Contoh Request (PowerShell)

```powershell
# Tambah produk
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/v1/products `
  -ContentType "application/json" `
  -Body '{"name":"Headset Gaming","description":"Headset 7.1","price":350000,"stock":15}'

# Daftar produk
Invoke-RestMethod http://localhost:8080/api/v1/products?page=1`&limit=5

# Detail, update, hapus
Invoke-RestMethod http://localhost:8080/api/v1/products/1
Invoke-RestMethod -Method Put -Uri http://localhost:8080/api/v1/products/1 `
  -ContentType "application/json" `
  -Body '{"name":"Keyboard RGB","description":"","price":550000,"stock":20}'
Invoke-RestMethod -Method Delete -Uri http://localhost:8080/api/v1/products/1
```

Contoh response sukses:

```json
{
  "success": true,
  "message": "produk berhasil dibuat",
  "data": { "id": 4, "name": "Headset Gaming", "price": 350000, "stock": 15 }
}
```

Contoh response validasi gagal (HTTP 422):

```json
{
  "success": false,
  "message": "validasi gagal",
  "errors": { "name": "nama minimal 3 karakter", "price": "harga harus lebih dari 0" }
}
```

## Role & Pengguna

| Role       | Menu yang tampil | Hak akses |
|------------|------------------|-----------|
| `admin`    | Semua | Semua fitur, termasuk kelola pengguna |
| `gudang`   | Ringkasan, Produk, Persediaan | Kelola produk |
| `supplier` | Supplier | Kelola data supplier |
| `kasir`    | Penjualan, Pembelian | Catat penjualan; buat, terima, dan batalkan pembelian (hanya bisa *melihat* produk & supplier untuk mengisi form) |
| `keuangan` | Keuangan | Catat transaksi kas, lihat arus kas, laba rugi, dan neraca |

Admin dari `.env` otomatis berole `admin`. Admin bisa membuat pengguna baru (tersimpan di tabel `users`) lewat menu **Pengguna** atau endpoint:

| Method | Path                 | Keterangan                      |
|--------|----------------------|---------------------------------|
| GET    | `/api/v1/users`      | Daftar pengguna (admin)         |
| POST   | `/api/v1/users`      | Buat pengguna (admin)           |
| PUT    | `/api/v1/users/{id}` | Ubah role / aktif-nonaktif      |

## Modul Supplier

CRUD supplier (nama, kontak, telepon, email, kota, alamat, catatan, status aktif) dengan pencarian, filter status, dan pagination. Dikelola oleh role `supplier` dan `admin` (kasir hanya bisa melihat untuk form pembelian).

| Method | Path                      | Keterangan                                   |
|--------|---------------------------|----------------------------------------------|
| GET    | `/api/v1/suppliers`       | Daftar (`page`, `limit`, `search`, `status`) |
| GET    | `/api/v1/suppliers/{id}`  | Detail                                       |
| POST   | `/api/v1/suppliers`       | Tambah                                       |
| PUT    | `/api/v1/suppliers/{id}`  | Ubah                                         |
| DELETE | `/api/v1/suppliers/{id}`  | Hapus (admin)                                |

## Modul Pembelian

Pembelian barang ke supplier (satu pembelian bisa berisi banyak produk). Status: **Menunggu** → **Diterima** (stok produk otomatis bertambah dalam satu transaksi database) atau **Dibatalkan**. Dikelola oleh role `kasir` dan `admin`. Supplier/produk yang sudah punya riwayat pembelian tidak bisa dihapus (supplier cukup dinonaktifkan).

| Method | Path                             | Keterangan                         |
|--------|----------------------------------|------------------------------------|
| GET    | `/api/v1/purchases`              | Daftar (`page`, `limit`, `status`) |
| GET    | `/api/v1/purchases/{id}`         | Detail + item                      |
| POST   | `/api/v1/purchases`              | Buat pembelian                     |
| POST   | `/api/v1/purchases/{id}/receive` | Terima barang, tambah stok         |
| POST   | `/api/v1/purchases/{id}/cancel`  | Batalkan                           |

## Modul Penjualan

Kasir mencatat penjualan (banyak produk per transaksi, metode tunai/transfer/QRIS). Dalam satu transaksi database: stok produk berkurang, penjualan tersimpan, dan kas masuk "Penjualan" tercatat otomatis di Keuangan. Harga diambil dari database (bukan dari klien) dan stok yang tidak cukup ditolak. Penjualan bisa **dibatalkan**: stok dikembalikan dan kas masuknya dihapus. Dikelola oleh role `kasir` dan `admin`.

| Method | Path                         | Keterangan                       |
|--------|------------------------------|----------------------------------|
| GET    | `/api/v1/sales`              | Daftar (`page`, `limit`, `status`) |
| GET    | `/api/v1/sales/{id}`         | Detail + item                    |
| POST   | `/api/v1/sales`              | Catat penjualan, kurangi stok    |
| POST   | `/api/v1/sales/{id}/cancel`  | Batalkan, kembalikan stok        |

## Modul Keuangan

Menu **Keuangan** (role `keuangan` dan `admin`) berisi tiga laporan dengan filter tanggal:

- **Arus kas** – saldo awal/akhir, kas dari operasi & pendanaan, dan daftar transaksi.
- **Laba rugi** – pendapatan dikurangi beban pada periode terpilih.
- **Neraca** – aset (kas + persediaan) = liabilitas + ekuitas, per tanggal "sampai".

Transaksi kas dicatat manual (kas masuk: Penjualan, Pendapatan lain, Modal, Pinjaman; kas keluar: Gaji, Operasional, Beban lain, Pembayaran pinjaman, Prive). Saat pembelian **diterima**, kas keluar "Pembelian barang" tercatat otomatis (dianggap dibayar tunai) dan tidak bisa dihapus manual. Pembelian barang tidak dihitung sebagai beban, melainkan menambah persediaan.

Catatan penyederhanaan: persediaan di neraca dihitung dari harga jual × stok saat ini, dan selisihnya terhadap total pembelian muncul sebagai "Selisih penilaian persediaan" di ekuitas.

| Method | Path                                    | Keterangan                          |
|--------|-----------------------------------------|-------------------------------------|
| GET    | `/api/v1/finance/reports?from=&to=`     | Arus kas, laba rugi, neraca         |
| GET    | `/api/v1/finance/transactions`          | Daftar transaksi (`from`, `to`, `page`, `limit`) |
| POST   | `/api/v1/finance/transactions`          | Catat transaksi kas                 |
| DELETE | `/api/v1/finance/transactions/{id}`     | Hapus transaksi manual              |

## Menjalankan Test

```
go test ./...
```

## Ide Pengembangan Berikutnya

- Autentikasi JWT + tabel `users`
- Dockerfile untuk men-deploy API
- Test integrasi dengan database sungguhan
- Dokumentasi Swagger/OpenAPI
