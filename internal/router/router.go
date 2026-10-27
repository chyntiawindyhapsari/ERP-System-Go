
package router

import (
	"context"
	"net/http"
	"strings"
	"time"

	"first-project/internal/handler"
	"first-project/internal/middleware"
	"first-project/internal/service"
	"first-project/web"

	"github.com/jackc/pgx/v5/pgxpool"
)

func New(pool *pgxpool.Pool, productSvc *service.ProductService) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","database":"down"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","database":"up"}`))
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(web.IndexHTML)
	})

	authHandler := handler.NewAuthHandler(pool)
	authHandler.Register(mux)

	protectedAPI := http.NewServeMux()
	handler.NewProductHandler(productSvc).Register(protectedAPI)

	handler.NewUserHandler(pool).Register(protectedAPI)
	handler.NewSupplierHandler(pool).Register(protectedAPI)
	handler.NewPurchaseHandler(pool).Register(protectedAPI)
	handler.NewFinanceHandler(pool).Register(protectedAPI)
	handler.NewSaleHandler(pool).Register(protectedAPI)

	// Hak akses per role (admin selalu boleh semuanya):
	//  - produk (& persediaan): gudang kelola penuh; kasir hanya lihat (untuk form pembelian)
	//  - supplier: role supplier kelola penuh; kasir hanya lihat (untuk form pembelian)
	//  - pembelian & penjualan: kasir kelola penuh
	//  - keuangan (arus kas, laba rugi, neraca): role keuangan
	//  - pengguna: hanya admin
	type rule struct {
		prefix string
		read   []string // boleh GET
		write  []string // boleh POST/PUT/PATCH/DELETE
	}
	rules := []rule{
		{"/api/v1/users", []string{"admin"}, []string{"admin"}},
		{"/api/v1/products", []string{"admin", "gudang", "kasir"}, []string{"admin", "gudang"}},
		{"/api/v1/suppliers", []string{"admin", "supplier", "kasir"}, []string{"admin", "supplier"}},
		{"/api/v1/purchases", []string{"admin", "kasir"}, []string{"admin", "kasir"}},
		{"/api/v1/sales", []string{"admin", "kasir"}, []string{"admin", "kasir"}},
		{"/api/v1/finance", []string{"admin", "keuangan"}, []string{"admin", "keuangan"}},
	}
	authorized := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var h http.Handler = protectedAPI
		for _, ru := range rules {
			if !strings.HasPrefix(r.URL.Path, ru.prefix) {
				continue
			}
			roles := ru.write
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				roles = ru.read
			}
			h = middleware.RequireRole(roles...)(h)
			break
		}
		h.ServeHTTP(w, r)
	})

	mux.Handle("/api/v1/", middleware.RequireSession(pool, authorized))

	return middleware.Recover(
		middleware.Logger(
			middleware.CORS(mux),
		),
	)
}