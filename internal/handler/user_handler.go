package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"first-project/internal/middleware"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var validRoles = map[string]bool{"admin": true, "gudang": true, "supplier": true, "kasir": true, "keuangan": true}

type UserHandler struct {
	pool *pgxpool.Pool
}

func NewUserHandler(pool *pgxpool.Pool) *UserHandler {
	return &UserHandler{pool: pool}
}

// Register: semua route ini hanya boleh diakses admin (dijaga di router).
func (h *UserHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/users", h.List)
	mux.HandleFunc("POST /api/v1/users", h.Create)
	mux.HandleFunc("PUT /api/v1/users/{id}", h.Update)
}

type userView struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	FullName  string    `json:"full_name"`
	Role      string    `json:"role"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, username, full_name, role, is_active, created_at
		FROM users ORDER BY id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memuat pengguna")
		return
	}
	defer rows.Close()

	users := []userView{}
	for rows.Next() {
		var u userView
		if err := rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.IsActive, &u.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Gagal membaca pengguna")
			return
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Data: users})
}

type createUserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in createUserInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.FullName = strings.TrimSpace(in.FullName)

	fields := map[string]string{}
	if len(in.Username) < 3 || len(in.Username) > 50 {
		fields["username"] = "username 3-50 karakter"
	}
	if len(in.Password) < 8 {
		fields["password"] = "password minimal 8 karakter"
	}
	if len(in.FullName) > 100 {
		fields["full_name"] = "nama maksimal 100 karakter"
	}
	if !validRoles[in.Role] {
		fields["role"] = "role harus admin, gudang, supplier, kasir, atau keuangan"
	}
	if len(fields) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, Response{
			Success: false, Message: "validasi gagal", Errors: fields,
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal memproses password")
		return
	}

	var u userView
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO users (username, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, full_name, role, is_active, created_at`,
		in.Username, string(hash), in.FullName, in.Role,
	).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.IsActive, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusUnprocessableEntity, Response{
				Success: false, Message: "validasi gagal",
				Errors: map[string]string{"username": "username sudah dipakai"},
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "Gagal membuat pengguna")
		return
	}
	writeJSON(w, http.StatusCreated, Response{Success: true, Message: "pengguna berhasil dibuat", Data: u})
}

type updateUserInput struct {
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
}

// Update mengubah role & status aktif. Admin tidak bisa menurunkan/menonaktifkan dirinya sendiri.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id tidak valid")
		return
	}
	var in updateUserInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validRoles[in.Role] {
		writeError(w, http.StatusUnprocessableEntity, "role harus admin, gudang, supplier, kasir, atau keuangan")
		return
	}
	if me, ok := middleware.UserFrom(r.Context()); ok && me.ID == id && (in.Role != "admin" || !in.IsActive) {
		writeError(w, http.StatusUnprocessableEntity, "Anda tidak bisa menurunkan role atau menonaktifkan akun sendiri")
		return
	}

	var u userView
	err = h.pool.QueryRow(r.Context(), `
		UPDATE users SET role = $2, is_active = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING id, username, full_name, role, is_active, created_at`,
		id, in.Role, in.IsActive,
	).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &u.IsActive, &u.CreatedAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "pengguna tidak ditemukan")
		return
	}
	if !in.IsActive {
		_, _ = h.pool.Exec(r.Context(), `DELETE FROM user_sessions WHERE user_id = $1`, id)
	}
	writeJSON(w, http.StatusOK, Response{Success: true, Message: "pengguna diperbarui", Data: u})
}
