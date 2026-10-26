
package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"first-project/internal/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	pool *pgxpool.Pool
}

func NewAuthHandler(pool *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{pool: pool}
}

func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/logout", h.Logout)
	mux.HandleFunc("GET /api/v1/auth/me", h.Me)
}

func (h *AuthHandler) EnsureAdmin(username, password, fullName string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("ADMIN_USERNAME dan ADMIN_PASSWORD wajib diisi")
	}
	if len(password) < 12 {
		return errors.New("ADMIN_PASSWORD minimal 12 karakter")
	}
	if len(username) > 50 {
		return errors.New("ADMIN_USERNAME maksimal 50 karakter")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = h.pool.Exec(
		contextBackground(),
		`INSERT INTO users (username, password_hash, full_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO NOTHING`,
		username, string(hash), fullName,
	)
	return err
}

func contextBackground() context.Context {
	return context.Background()
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Data login tidak valid")
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || input.Password == "" {
		writeError(w, http.StatusBadRequest, "Username dan password wajib diisi")
		return
	}

	var userID int64
	var username, passwordHash, fullName string
	var active bool

	err := h.pool.QueryRow(r.Context(), `
		SELECT id, username, password_hash, full_name, is_active
		FROM users
		WHERE username = $1
	`, input.Username).Scan(&userID, &username, &passwordHash, &fullName, &active)

	if err != nil || !active ||
		bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "Username atau password salah")
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal membuat sesi")
		return
	}

	token := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(token)
	expires := time.Now().Add(24 * time.Hour)

	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO user_sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Gagal menyimpan sesi")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   86400,
	})

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Login berhasil",
		Data: map[string]any{
			"id":        userID,
			"username":  username,
			"full_name": fullName,
		},
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.SessionCookieName)
	if err == nil && cookie.Value != "" {
		_, _ = h.pool.Exec(r.Context(),
			`DELETE FROM user_sessions WHERE token_hash = $1`,
			hashToken(cookie.Value),
		)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Message: "Logout berhasil",
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Silakan login")
		return
	}

	var id int64
	var username, fullName string

	err = h.pool.QueryRow(r.Context(), `
		SELECT u.id, u.username, u.full_name
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > NOW()
		  AND u.is_active = TRUE
	`, hashToken(cookie.Value)).Scan(&id, &username, &fullName)

	if err != nil {
		writeError(w, http.StatusUnauthorized, "Sesi tidak valid atau sudah berakhir")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Success: true,
		Data: map[string]any{
			"id":        id,
			"username":  username,
			"full_name": fullName,
		},
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Menghindari error lint: fungsi ini tidak digunakan untuk menentukan status login.
var _ = pgx.ErrNoRows
