package handler

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SupplierHandler struct {
	pool *pgxpool.Pool
}

func NewSupplierHandler(pool *pgxpool.Pool) *SupplierHandler {
	return &SupplierHandler{pool: pool}
}

// Hak akses dijaga di router: baca = semua role, tulis = admin/manager, hapus = admin.
func (h *SupplierHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/suppliers", h.List)
	mux.HandleFunc("GET /api/v1/suppliers/{id}", h.Get)
	mux.HandleFunc("POST /api/v1/suppliers", h.Create)
	mux.HandleFunc("PUT /api/v1/suppliers/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/suppliers/{id}", h.Delete)
}

type supplier struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	ContactPerson string    `json:"contact_person"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	City          string    `json:"city"`
	Address       string    `json:"address"`
	Notes         string    `json:"notes"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type supplierInput struct {
	Name          string `json:"name"`
	ContactPerson string `json:"contact_person"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	City          string `json:"city"`
	Address       string `json:"address"`
	Notes         string `json:"notes"`
	IsActive      bool   `json:"is_active"`
}

const supplierCols = `id, name, contact_person, email, phone, city, address, notes, is_active, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanSupplier(r rowScanner) (supplier, error) {
	var s supplier
	err := r.Scan(&s.ID, &s.Name, &s.ContactPerson, &s.Email, &s.Phone, &s.City,
		&s.Address, &s.Notes, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (in *supplierInput) clean() map[string]string {
	in.Name = strings.TrimSpace(in.Name)
	in.ContactPerson = strings.TrimSpace(in.ContactPerson)
	in.Email = strings.TrimSpace(in.Email)
	in.Phone = strings.TrimSpace(in.Phone)
	in.City = strings.TrimSpace(in.City)
	in.Address = strings.TrimSpace(in.Address)
	in.Notes = strings.TrimSpace(in.Notes)

	f := map[string]string{}
	if len(in.Name) < 3 || len(in.Name) > 100 {
		f["name"] = "nama supplier 3-100 karakter"
	}
	if len(in.ContactPerson) > 100 {
		f["contact_person"] = "maksimal 100 karakter"
	}
	if in.Email != "" && (len(in.Email) > 120 || !strings.Contains(in.Email, "@") || strings.ContainsAny(in.Email, " ,;")) {
		f["email"] = "format email tidak valid"
	}
	if len(in.Phone) > 30 {
		f["phone"] = "maksimal 30 karakter"
	} else {
		for _, c := range in.Phone {
			if !(c >= '0' && c <= '9') && !strings.ContainsRune("+-() ", c) {
				f["phone"] = "nomor telepon hanya angka, spasi, + - ( )"
				break
			}
		}
	}
	if len(in.City) > 80 {
		f["city"] = "maksimal 80 karakter"
	}
	if len(in.Address) > 300 {
		f["address"] = "maksimal 300 karakter"
	}
	if len(in.Notes) > 500 {
		f["notes"] = "maksimal 500 karakter"
	}
	return f
}

func supplierID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *SupplierHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	search := "%" + strings.ToLower(strings.TrimSpace(q.Get("search"))) + "%"
	status := q.Get("status") // "", "active", "inactive"

	where := `(LOWER(name) LIKE $1 OR LOWER(contact_person) LIKE $1 OR LOWER(city) LIKE $1 OR LOWER(email) LIKE $1)`
	switch status {
	case "active":
		where += ` AND is_active = TRUE`
	case "inactive":
		where += ` AND is_active = FALSE`
	}

	var total, active int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE is_active) FROM suppliers WHERE `+where, search,
	).Scan(&total, &active); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat supplier")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT `+supplierCols+` FROM suppliers WHERE `+where+` ORDER BY name LIMIT $2 OFFSET $3`,
		search, limit, (page-1)*limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat supplier")
		return
	}
	defer rows.Close()

	items := []supplier{}
	for rows.Next() {
		s, err := scanSupplier(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca supplier")
			return
		}
		items = append(items, s)
	}

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Data:    items,
		Meta: map[string]any{
			"page": page, "limit": limit, "total": total, "active": active,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

func (h *SupplierHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	s, err := scanSupplier(h.pool.QueryRow(r.Context(), `SELECT `+supplierCols+` FROM suppliers WHERE id = $1`, id))
	if err != nil {
		writeError(w, http.StatusNotFound, "supplier tidak ditemukan")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: s})
}

func (h *SupplierHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in supplierInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if f := in.clean(); len(f) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, Response{Success: false, Message: "validasi gagal", Errors: f})
		return
	}
	s, err := scanSupplier(h.pool.QueryRow(r.Context(), `
		INSERT INTO suppliers (name, contact_person, email, phone, city, address, notes, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+supplierCols,
		in.Name, in.ContactPerson, in.Email, in.Phone, in.City, in.Address, in.Notes, in.IsActive))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan supplier")
		return
	}
	writeJSON(w, http.StatusCreated, Response{Success: true, Message: "supplier berhasil ditambahkan", Data: s})
}

func (h *SupplierHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	var in supplierInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if f := in.clean(); len(f) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, Response{Success: false, Message: "validasi gagal", Errors: f})
		return
	}
	s, err := scanSupplier(h.pool.QueryRow(r.Context(), `
		UPDATE suppliers SET name=$2, contact_person=$3, email=$4, phone=$5, city=$6,
		       address=$7, notes=$8, is_active=$9, updated_at=NOW()
		WHERE id=$1 RETURNING `+supplierCols,
		id, in.Name, in.ContactPerson, in.Email, in.Phone, in.City, in.Address, in.Notes, in.IsActive))
	if err != nil {
		writeError(w, http.StatusNotFound, "supplier tidak ditemukan")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "supplier berhasil diperbarui", Data: s})
}

func (h *SupplierHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := supplierID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM suppliers WHERE id = $1`, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			writeError(w, http.StatusConflict, "supplier sudah punya riwayat pembelian; nonaktifkan saja")
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal menghapus supplier")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "supplier tidak ditemukan")
		return
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "supplier berhasil dihapus"})
}
