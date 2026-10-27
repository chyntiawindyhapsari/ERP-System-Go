package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"first-project/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SaleHandler struct {
	pool *pgxpool.Pool
}

func NewSaleHandler(pool *pgxpool.Pool) *SaleHandler {
	return &SaleHandler{pool: pool}
}

// Hak akses (dijaga di router): admin & kasir.
func (h *SaleHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/sales", h.List)
	mux.HandleFunc("GET /api/v1/sales/{id}", h.Get)
	mux.HandleFunc("POST /api/v1/sales", h.Create)
	mux.HandleFunc("POST /api/v1/sales/{id}/cancel", h.Cancel)
}

type saleItem struct {
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name,omitempty"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
}

type sale struct {
	ID            int64      `json:"id"`
	CustomerName  string     `json:"customer_name"`
	PaymentMethod string     `json:"payment_method"`
	Status        string     `json:"status"`
	Notes         string     `json:"notes"`
	Total         int64      `json:"total"`
	ItemCount     int        `json:"item_count"`
	CreatedAt     time.Time  `json:"created_at"`
	Items         []saleItem `json:"items,omitempty"`
}

const saleSelect = `
	SELECT s.id, s.customer_name, s.payment_method, s.status, s.notes, s.total,
	       (SELECT COUNT(*) FROM sale_items i WHERE i.sale_id = s.id), s.created_at
	FROM sales s`

func scanSale(r rowScanner) (sale, error) {
	var s sale
	err := r.Scan(&s.ID, &s.CustomerName, &s.PaymentMethod, &s.Status, &s.Notes, &s.Total, &s.ItemCount, &s.CreatedAt)
	return s, err
}

func (h *SaleHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	status := q.Get("status")

	var total int
	var revenueToday int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM sales WHERE $1 = '' OR status = $1`, status).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat penjualan")
		return
	}
	_ = h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(total), 0) FROM sales
		WHERE status = 'completed' AND created_at::date = CURRENT_DATE`).Scan(&revenueToday)

	rows, err := h.pool.Query(r.Context(),
		saleSelect+` WHERE ($1 = '' OR s.status = $1) ORDER BY s.id DESC LIMIT $2 OFFSET $3`,
		status, limit, (page-1)*limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat penjualan")
		return
	}
	defer rows.Close()

	items := []sale{}
	for rows.Next() {
		s, err := scanSale(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca penjualan")
			return
		}
		items = append(items, s)
	}
	writeJSON(w, http.StatusOK, Response{
		Success: true, Data: items,
		Meta: map[string]any{
			"page": page, "limit": limit, "total": total, "revenue_today": revenueToday,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

func (h *SaleHandler) load(ctx context.Context, id int64) (*sale, error) {
	s, err := scanSale(h.pool.QueryRow(ctx, saleSelect+` WHERE s.id = $1`, id))
	if err != nil {
		return nil, err
	}
	rows, err := h.pool.Query(ctx, `
		SELECT i.product_id, p.name, i.quantity, i.unit_price
		FROM sale_items i JOIN products p ON p.id = i.product_id
		WHERE i.sale_id = $1 ORDER BY i.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it saleItem
		if err := rows.Scan(&it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPrice); err != nil {
			return nil, err
		}
		s.Items = append(s.Items, it)
	}
	return &s, nil
}

func (h *SaleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	s, err := h.load(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "penjualan tidak ditemukan")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: s})
}

type saleInput struct {
	CustomerName  string `json:"customer_name"`
	PaymentMethod string `json:"payment_method"`
	Notes         string `json:"notes"`
	Items         []struct {
		ProductID int64 `json:"product_id"`
		Quantity  int   `json:"quantity"`
	} `json:"items"`
}

func validationFail(w http.ResponseWriter, field, msg string) {
	writeJSON(w, http.StatusUnprocessableEntity, Response{
		Success: false, Message: "validasi gagal", Errors: map[string]string{field: msg},
	})
}

// Create: kurangi stok + simpan penjualan + catat kas masuk, semuanya satu transaksi.
// Harga selalu diambil dari database (bukan dari klien).
func (h *SaleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in saleInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.CustomerName = strings.TrimSpace(in.CustomerName)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.PaymentMethod == "" {
		in.PaymentMethod = "tunai"
	}

	switch {
	case len(in.CustomerName) > 100:
		validationFail(w, "customer_name", "nama pelanggan maksimal 100 karakter")
		return
	case in.PaymentMethod != "tunai" && in.PaymentMethod != "transfer" && in.PaymentMethod != "qris":
		validationFail(w, "payment_method", "metode bayar tidak valid")
		return
	case len(in.Notes) > 500:
		validationFail(w, "notes", "catatan maksimal 500 karakter")
		return
	case len(in.Items) == 0 || len(in.Items) > 50:
		validationFail(w, "items", "tambahkan 1-50 item")
		return
	}
	seen := map[int64]bool{}
	for _, it := range in.Items {
		if it.ProductID <= 0 || it.Quantity <= 0 || it.Quantity > 100000 {
			validationFail(w, "items", "setiap item butuh produk dan jumlah > 0")
			return
		}
		if seen[it.ProductID] {
			validationFail(w, "items", "produk yang sama tidak boleh dobel")
			return
		}
		seen[it.ProductID] = true
	}
	// Kunci baris produk dengan urutan tetap agar tidak terjadi deadlock.
	sort.Slice(in.Items, func(i, j int) bool { return in.Items[i].ProductID < in.Items[j].ProductID })

	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penjualan")
		return
	}
	defer tx.Rollback(ctx)

	lines := make([]saleItem, 0, len(in.Items))
	var total int64
	for _, it := range in.Items {
		var name string
		var price int64
		var stock int
		err := tx.QueryRow(ctx, `SELECT name, price, stock FROM products WHERE id = $1 FOR UPDATE`, it.ProductID).Scan(&name, &price, &stock)
		if err == pgx.ErrNoRows {
			validationFail(w, "items", "ada produk yang tidak ditemukan")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan penjualan")
			return
		}
		if stock < it.Quantity {
			validationFail(w, "items", fmt.Sprintf("stok %s tidak cukup (tersisa %d)", name, stock))
			return
		}
		if _, err := tx.Exec(ctx, `UPDATE products SET stock = stock - $2, updated_at = NOW() WHERE id = $1`, it.ProductID, it.Quantity); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mengurangi stok")
			return
		}
		lines = append(lines, saleItem{ProductID: it.ProductID, Quantity: it.Quantity, UnitPrice: price})
		total += int64(it.Quantity) * price
	}

	var userID *int64
	if u, ok := middleware.UserFrom(ctx); ok {
		userID = &u.ID
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO sales (customer_name, payment_method, notes, total, created_by)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		in.CustomerName, in.PaymentMethod, in.Notes, total, userID).Scan(&id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penjualan")
		return
	}
	for _, l := range lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sale_items (sale_id, product_id, quantity, unit_price) VALUES ($1, $2, $3, $4)`,
			id, l.ProductID, l.Quantity, l.UnitPrice); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal menyimpan item")
			return
		}
	}
	if total > 0 {
		desc := fmt.Sprintf("INV-%04d", id)
		if in.CustomerName != "" {
			desc += " · " + in.CustomerName
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cash_transactions (tx_date, type, category, description, amount, sale_id, created_by)
			VALUES (CURRENT_DATE, 'in', 'Penjualan', $1, $2, $3, $4)`, desc, total, id, userID); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal mencatat kas masuk")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan penjualan")
		return
	}
	s, _ := h.load(ctx, id)
	writeJSON(w, http.StatusCreated, Response{Success: true, Message: "penjualan berhasil dicatat", Data: s})
}

// Cancel: stok dikembalikan dan kas masuk terkait dihapus.
func (h *SaleHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM sales WHERE id = $1 FOR UPDATE`, id).Scan(&status)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "penjualan tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	if status != "completed" {
		writeError(w, http.StatusConflict, "penjualan sudah dibatalkan")
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE products p SET stock = p.stock + i.quantity, updated_at = NOW()
		FROM sale_items i WHERE i.sale_id = $1 AND i.product_id = p.id`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal mengembalikan stok")
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cash_transactions WHERE sale_id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membatalkan kas masuk")
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE sales SET status = 'cancelled' WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "penjualan dibatalkan, stok dikembalikan"})
}
