package handler

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"first-project/internal/middleware"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FinanceHandler struct {
	pool *pgxpool.Pool
}

func NewFinanceHandler(pool *pgxpool.Pool) *FinanceHandler {
	return &FinanceHandler{pool: pool}
}

// Hak akses (dijaga di router): admin & keuangan.
func (h *FinanceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/finance/transactions", h.ListTransactions)
	mux.HandleFunc("POST /api/v1/finance/transactions", h.CreateTransaction)
	mux.HandleFunc("DELETE /api/v1/finance/transactions/{id}", h.DeleteTransaction)
	mux.HandleFunc("GET /api/v1/finance/reports", h.Reports)
}

// Kategori tetap supaya laporan konsisten.
const (
	catPurchase  = "Pembelian barang" // otomatis dari modul pembelian
	catCapital   = "Modal"
	catLoan      = "Pinjaman"
	catLoanPaid  = "Pembayaran pinjaman"
	catWithdraw  = "Prive"
	catSales     = "Penjualan"
	catOtherInc  = "Pendapatan lain"
	catSalary    = "Gaji"
	catOperating = "Operasional"
	catOtherExp  = "Beban lain"
)

var incomeCategories = []string{catSales, catOtherInc, catCapital, catLoan}
var expenseCategories = []string{catSalary, catOperating, catOtherExp, catLoanPaid, catWithdraw}

func validCategory(txType, cat string) bool {
	list := incomeCategories
	if txType == "out" {
		list = expenseCategories
	}
	for _, c := range list {
		if c == cat {
			return true
		}
	}
	return false
}

type cashTx struct {
	ID          int64     `json:"id"`
	Date        string    `json:"date"`
	Type        string    `json:"type"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	Auto        bool      `json:"auto"`
	CreatedAt   time.Time `json:"created_at"`
}

// parseRange membaca ?from=&to= (YYYY-MM-DD). Default: awal bulan ini s.d. hari ini.
func parseRange(r *http.Request) (string, string, bool) {
	now := time.Now()
	from := now.Format("2006-01") + "-01"
	to := now.Format("2006-01-02")
	if v := r.URL.Query().Get("from"); v != "" {
		from = v
	}
	if v := r.URL.Query().Get("to"); v != "" {
		to = v
	}
	f, err1 := time.Parse("2006-01-02", from)
	t, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || t.Before(f) {
		return "", "", false
	}
	return from, to, true
}

func (h *FinanceHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseRange(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "rentang tanggal tidak valid")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	var total int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM cash_transactions WHERE tx_date BETWEEN $1 AND $2`, from, to).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat transaksi")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, tx_date::text, type, category, description, amount,
		       (purchase_order_id IS NOT NULL OR sale_id IS NOT NULL), created_at
		FROM cash_transactions WHERE tx_date BETWEEN $1 AND $2
		ORDER BY tx_date DESC, id DESC LIMIT $3 OFFSET $4`, from, to, limit, (page-1)*limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat transaksi")
		return
	}
	defer rows.Close()

	items := []cashTx{}
	for rows.Next() {
		var t cashTx
		if err := rows.Scan(&t.ID, &t.Date, &t.Type, &t.Category, &t.Description, &t.Amount, &t.Auto, &t.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca transaksi")
			return
		}
		items = append(items, t)
	}
	writeJSON(w, http.StatusOK, Response{
		Success: true, Data: items,
		Meta: map[string]any{
			"page": page, "limit": limit, "total": total,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

type cashTxInput struct {
	Date        string `json:"date"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"`
}

func (h *FinanceHandler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var in cashTxInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Description = strings.TrimSpace(in.Description)

	f := map[string]string{}
	if in.Type != "in" && in.Type != "out" {
		f["type"] = "pilih kas masuk atau kas keluar"
	} else if !validCategory(in.Type, in.Category) {
		f["category"] = "kategori tidak valid"
	}
	if _, err := time.Parse("2006-01-02", in.Date); err != nil {
		f["date"] = "tanggal tidak valid"
	}
	if in.Amount <= 0 || in.Amount > 1_000_000_000_000 {
		f["amount"] = "jumlah harus lebih dari 0"
	}
	if len(in.Description) > 200 {
		f["description"] = "keterangan maksimal 200 karakter"
	}
	if len(f) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, Response{Success: false, Message: "validasi gagal", Errors: f})
		return
	}

	var userID *int64
	if u, ok := middleware.UserFrom(r.Context()); ok {
		userID = &u.ID
	}
	var t cashTx
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO cash_transactions (tx_date, type, category, description, amount, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tx_date::text, type, category, description, amount, FALSE, created_at`,
		in.Date, in.Type, in.Category, in.Description, in.Amount, userID,
	).Scan(&t.ID, &t.Date, &t.Type, &t.Category, &t.Description, &t.Amount, &t.Auto, &t.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan transaksi")
		return
	}
	writeJSON(w, http.StatusCreated, Response{Success: true, Message: "transaksi berhasil dicatat", Data: t})
}

// Transaksi otomatis dari pembelian/penjualan tidak boleh dihapus manual.
func (h *FinanceHandler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM cash_transactions WHERE id = $1 AND purchase_order_id IS NULL AND sale_id IS NULL`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghapus transaksi")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "transaksi tidak ditemukan atau berasal dari pembelian/penjualan otomatis")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "transaksi dihapus"})
}

type catTotal struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

type sumKey struct{ typ, cat string }

func (h *FinanceHandler) sums(r *http.Request, where string, args ...any) (map[sumKey]int64, error) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT type, category, COALESCE(SUM(amount), 0) FROM cash_transactions WHERE `+where+` GROUP BY type, category`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[sumKey]int64{}
	for rows.Next() {
		var k sumKey
		var v int64
		if err := rows.Scan(&k.typ, &k.cat, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}

func lines(m map[sumKey]int64, typ string, cats ...string) ([]catTotal, int64) {
	out := []catTotal{}
	var total int64
	for _, c := range cats {
		v := m[sumKey{typ, c}]
		out = append(out, catTotal{c, v})
		total += v
	}
	return out, total
}

// Reports menghitung Arus Kas & Laba Rugi untuk rentang from..to, dan Neraca per tanggal "to".
func (h *FinanceHandler) Reports(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseRange(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "rentang tanggal tidak valid")
		return
	}
	period, err := h.sums(r, `tx_date BETWEEN $1 AND $2`, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung laporan")
		return
	}
	before, err := h.sums(r, `tx_date < $1`, from)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung laporan")
		return
	}
	upto, err := h.sums(r, `tx_date <= $1`, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung laporan")
		return
	}
	var inventory int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(price * stock), 0) FROM products`).Scan(&inventory); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menghitung laporan")
		return
	}

	net := func(m map[sumKey]int64) int64 {
		var n int64
		for k, v := range m {
			if k.typ == "in" {
				n += v
			} else {
				n -= v
			}
		}
		return n
	}

	// --- Arus kas (operasi vs pendanaan)
	opIn, opInTotal := lines(period, "in", catSales, catOtherInc)
	opOut, opOutTotal := lines(period, "out", catPurchase, catSalary, catOperating, catOtherExp)
	finIn, finInTotal := lines(period, "in", catCapital, catLoan)
	finOut, finOutTotal := lines(period, "out", catLoanPaid, catWithdraw)
	opening := net(before)
	operating := opInTotal - opOutTotal
	financing := finInTotal - finOutTotal

	// --- Laba rugi (pembelian barang = aset persediaan, bukan beban)
	revenue, revTotal := lines(period, "in", catSales, catOtherInc)
	expense, expTotal := lines(period, "out", catSalary, catOperating, catOtherExp)

	// --- Neraca per tanggal "to"
	cash := net(upto)
	_, loanIn := lines(upto, "in", catLoan)
	_, loanPaid := lines(upto, "out", catLoanPaid)
	_, capIn := lines(upto, "in", catCapital)
	_, draw := lines(upto, "out", catWithdraw)
	_, revAll := lines(upto, "in", catSales, catOtherInc)
	_, expAll := lines(upto, "out", catSalary, catOperating, catOtherExp)
	_, purchased := lines(upto, "out", catPurchase)

	liabilities := loanIn - loanPaid
	capital := capIn - draw
	retained := revAll - expAll
	revaluation := inventory - purchased
	assets := cash + inventory

	writeJSON(w, http.StatusOK, Response{Success: true, Data: map[string]any{
		"from": from, "to": to,
		"cashflow": map[string]any{
			"opening": opening, "closing": opening + operating + financing,
			"operating_in": opIn, "operating_out": opOut, "operating": operating,
			"financing_in": finIn, "financing_out": finOut, "financing": financing,
			"total_in": opInTotal + finInTotal, "total_out": opOutTotal + finOutTotal,
		},
		"income_statement": map[string]any{
			"revenue": revenue, "revenue_total": revTotal,
			"expense": expense, "expense_total": expTotal,
			"net_income": revTotal - expTotal,
		},
		"balance_sheet": map[string]any{
			"cash": cash, "inventory": inventory, "assets": assets,
			"liabilities": liabilities,
			"capital":     capital, "retained": retained, "revaluation": revaluation,
			"equity":            capital + retained + revaluation,
			"liabilities_equity": liabilities + capital + retained + revaluation,
		},
	}})
}
