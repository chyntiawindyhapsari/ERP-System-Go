package handler

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"first-project/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PurchaseHandler struct {
	pool *pgxpool.Pool
}

func NewPurchaseHandler(pool *pgxpool.Pool) *PurchaseHandler {
	return &PurchaseHandler{pool: pool}
}

// Baca = semua role; buat/terima/batalkan = admin & manager (dijaga di router).
func (h *PurchaseHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/purchases", h.List)
	mux.HandleFunc("GET /api/v1/purchases/{id}", h.Get)
	mux.HandleFunc("POST /api/v1/purchases", h.Create)
	mux.HandleFunc("POST /api/v1/purchases/{id}/receive", h.Receive)
	mux.HandleFunc("POST /api/v1/purchases/{id}/cancel", h.Cancel)
}

type purchaseItem struct {
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name,omitempty"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
}

type purchase struct {
	ID           int64          `json:"id"`
	SupplierID   int64          `json:"supplier_id"`
	SupplierName string         `json:"supplier_name"`
	Status       string         `json:"status"`
	Notes        string         `json:"notes"`
	Total        int64          `json:"total"`
	ItemCount    int            `json:"item_count"`
	CreatedAt    time.Time      `json:"created_at"`
	ReceivedAt   *time.Time     `json:"received_at"`
	Items        []purchaseItem `json:"items,omitempty"`
}

const purchaseSelect = `
	SELECT po.id, po.supplier_id, s.name, po.status, po.notes, po.total,
	       (SELECT COUNT(*) FROM purchase_order_items i WHERE i.purchase_order_id = po.id),
	       po.created_at, po.received_at
	FROM purchase_orders po JOIN suppliers s ON s.id = po.supplier_id`

func scanPurchase(r rowScanner) (purchase, error) {
	var p purchase
	err := r.Scan(&p.ID, &p.SupplierID, &p.SupplierName, &p.Status, &p.Notes,
		&p.Total, &p.ItemCount, &p.CreatedAt, &p.ReceivedAt)
	return p, err
}

func (h *PurchaseHandler) List(w http.ResponseWriter, r *http.Request) {
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

	var total, pending int
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FILTER (WHERE $1 = '' OR status = $1),
		       COUNT(*) FILTER (WHERE status = 'pending')
		FROM purchase_orders`, status).Scan(&total, &pending); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat pembelian")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		purchaseSelect+` WHERE ($1 = '' OR po.status = $1) ORDER BY po.id DESC LIMIT $2 OFFSET $3`,
		status, limit, (page-1)*limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat pembelian")
		return
	}
	defer rows.Close()

	items := []purchase{}
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca pembelian")
			return
		}
		items = append(items, p)
	}
	writeJSON(w, http.StatusOK, Response{
		Success: true, Data: items,
		Meta: map[string]any{
			"page": page, "limit": limit, "total": total, "pending": pending,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

func (h *PurchaseHandler) load(ctx context.Context, id int64) (*purchase, error) {
	p, err := scanPurchase(h.pool.QueryRow(ctx, purchaseSelect+` WHERE po.id = $1`, id))
	if err != nil {
		return nil, err
	}
	rows, err := h.pool.Query(ctx, `
		SELECT i.product_id, pr.name, i.quantity, i.unit_price
		FROM purchase_order_items i JOIN products pr ON pr.id = i.product_id
		WHERE i.purchase_order_id = $1 ORDER BY i.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it purchaseItem
		if err := rows.Scan(&it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPrice); err != nil {
			return nil, err
		}
		p.Items = append(p.Items, it)
	}
	return &p, nil
}

func (h *PurchaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	p, err := h.load(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "pembelian tidak ditemukan")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: p})
}

type purchaseInput struct {
	SupplierID int64          `json:"supplier_id"`
	Notes      string         `json:"notes"`
	Items      []purchaseItem `json:"items"`
}

func (h *PurchaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in purchaseInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fields := map[string]string{}
	if in.SupplierID <= 0 {
		fields["supplier_id"] = "pilih supplier"
	}
	if len(in.Notes) > 500 {
		fields["notes"] = "catatan maksimal 500 karakter"
	}
	if len(in.Items) == 0 || len(in.Items) > 50 {
		fields["items"] = "tambahkan 1-50 item"
	}
	var total int64
	seen := map[int64]bool{}
	for _, it := range in.Items {
		if it.ProductID <= 0 || it.Quantity <= 0 || it.Quantity > 1_000_000 || it.UnitPrice < 0 {
			fields["items"] = "setiap item butuh produk, jumlah > 0, dan harga valid"
			break
		}
		if seen[it.ProductID] {
			fields["items"] = "produk yang sama tidak boleh dobel"
			break
		}
		seen[it.ProductID] = true
		total += int64(it.Quantity) * it.UnitPrice
	}
	if len(fields) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, Response{Success: false, Message: "validasi gagal", Errors: fields})
		return
	}

	var userID *int64
	if u, ok := middleware.UserFrom(r.Context()); ok {
		userID = &u.ID
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan pembelian")
		return
	}
	defer tx.Rollback(r.Context())

	var id int64
	err = tx.QueryRow(r.Context(), `
		INSERT INTO purchase_orders (supplier_id, notes, total, created_by)
		SELECT id, $2, $3, $4 FROM suppliers WHERE id = $1 AND is_active = TRUE
		RETURNING id`, in.SupplierID, in.Notes, total, userID).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, Response{
			Success: false, Message: "validasi gagal",
			Errors: map[string]string{"supplier_id": "supplier tidak ditemukan atau nonaktif"},
		})
		return
	}
	for _, it := range in.Items {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO purchase_order_items (purchase_order_id, product_id, quantity, unit_price)
			VALUES ($1, $2, $3, $4)`, id, it.ProductID, it.Quantity, it.UnitPrice); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, Response{
				Success: false, Message: "validasi gagal",
				Errors: map[string]string{"items": "ada produk yang tidak ditemukan"},
			})
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan pembelian")
		return
	}
	p, _ := h.load(r.Context(), id)
	writeJSON(w, http.StatusCreated, Response{Success: true, Message: "pembelian berhasil dibuat", Data: p})
}

// Receive: status pending -> received dan stok produk bertambah (satu transaksi).
func (h *PurchaseHandler) Receive(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	defer tx.Rollback(r.Context())

	var status string
	err = tx.QueryRow(r.Context(), `SELECT status FROM purchase_orders WHERE id = $1 FOR UPDATE`, id).Scan(&status)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "pembelian tidak ditemukan")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	if status != "pending" {
		writeError(w, http.StatusConflict, "hanya pembelian berstatus menunggu yang bisa diterima")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE products p SET stock = p.stock + i.quantity, updated_at = NOW()
		FROM purchase_order_items i
		WHERE i.purchase_order_id = $1 AND i.product_id = p.id`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menambah stok")
		return
	}
	if _, err := tx.Exec(r.Context(),
		`UPDATE purchase_orders SET status = 'received', received_at = NOW() WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "barang diterima, stok diperbarui"})
}

func (h *PurchaseHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE purchase_orders SET status = 'cancelled' WHERE id = $1 AND status = 'pending'`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "hanya pembelian berstatus menunggu yang bisa dibatalkan")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "pembelian dibatalkan"})
}
