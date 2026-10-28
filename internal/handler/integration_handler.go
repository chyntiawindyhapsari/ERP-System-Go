package handler

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// IntegrationHandler: endpoint READ-ONLY untuk n8n / bot WhatsApp.
// Dilindungi API key (header X-API-Key), bukan login cookie.
type IntegrationHandler struct {
	pool   *pgxpool.Pool
	apiKey string
}

func NewIntegrationHandler(pool *pgxpool.Pool, apiKey string) *IntegrationHandler {
	return &IntegrationHandler{pool: pool, apiKey: apiKey}
}

func (h *IntegrationHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/integration/summary", h.auth(h.Summary))
	mux.Handle("GET /api/integration/low-stock", h.auth(h.LowStock))
	mux.Handle("GET /api/integration/stock", h.auth(h.Stock))
}

func (h *IntegrationHandler) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.apiKey == "" {
			writeError(w, http.StatusServiceUnavailable, "INTEGRATION_API_KEY belum diisi di .env")
			return
		}
		got := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(got), []byte(h.apiKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "API key salah")
			return
		}
		next(w, r)
	})
}

// Summary: ringkasan harian (default hari ini, atau ?date=2026-10-28).
func (h *IntegrationHandler) Summary(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "format date harus YYYY-MM-DD")
		return
	}
	var salesCount int
	var revenue, cashIn, cashOut int64
	var lowStock int
	ctx := r.Context()
	if err := h.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total), 0) FROM sales
		WHERE status = 'completed' AND created_at::date = $1`, d).Scan(&salesCount, &revenue); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan")
		return
	}
	if err := h.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount) FILTER (WHERE type = 'in'), 0),
		       COALESCE(SUM(amount) FILTER (WHERE type = 'out'), 0)
		FROM cash_transactions WHERE tx_date = $1`, d).Scan(&cashIn, &cashOut); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat ringkasan")
		return
	}
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM products WHERE stock <= 5`).Scan(&lowStock)

	writeJSON(w, http.StatusOK, Response{Success: true, Data: map[string]any{
		"date":            date,
		"sales_count":     salesCount,
		"revenue":         revenue,
		"cash_in":         cashIn,
		"cash_out":        cashOut,
		"low_stock_items": lowStock,
		"text": "Ringkasan " + date + "\n" +
			"Penjualan: " + strconv.Itoa(salesCount) + " transaksi\n" +
			"Omzet: Rp " + rupiah(revenue) + "\n" +
			"Kas masuk: Rp " + rupiah(cashIn) + "\n" +
			"Kas keluar: Rp " + rupiah(cashOut) + "\n" +
			"Stok menipis: " + strconv.Itoa(lowStock) + " produk",
	}})
}

type stockRow struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Stock int    `json:"stock"`
	Price int64  `json:"price"`
}

func (h *IntegrationHandler) queryStock(r *http.Request, where string, args ...any) ([]stockRow, error) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, stock, price FROM products WHERE `+where+` ORDER BY stock ASC, name LIMIT 50`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []stockRow{}
	for rows.Next() {
		var s stockRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Stock, &s.Price); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func stockText(title string, items []stockRow) string {
	if len(items) == 0 {
		return title + ": tidak ada"
	}
	var b strings.Builder
	b.WriteString(title + ":\n")
	for _, s := range items {
		b.WriteString("- " + s.Name + ": " + strconv.Itoa(s.Stock) + "\n")
	}
	return strings.TrimSpace(b.String())
}

// LowStock: produk dengan stok <= ?threshold (default 5).
func (h *IntegrationHandler) LowStock(w http.ResponseWriter, r *http.Request) {
	th, err := strconv.Atoi(r.URL.Query().Get("threshold"))
	if err != nil || th < 0 {
		th = 5
	}
	items, err := h.queryStock(r, `stock <= $1`, th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat stok")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: map[string]any{
		"count": len(items), "items": items, "text": stockText("Stok menipis", items),
	}})
}

// Stock: cari stok produk, mis. /api/integration/stock?q=mouse (untuk bot WA).
func (h *IntegrationHandler) Stock(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "parameter q wajib diisi")
		return
	}
	items, err := h.queryStock(r, `name ILIKE '%' || $1 || '%'`, q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat stok")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: map[string]any{
		"count": len(items), "items": items, "text": stockText("Stok '"+q+"'", items),
	}})
}

// rupiah memformat 1500000 -> "1.500.000".
func rupiah(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
