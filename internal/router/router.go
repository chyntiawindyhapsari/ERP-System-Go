package router

import (
	"context"
	"net/http"
	"time"

	"first-project/internal/handler"
	"first-project/internal/middleware"
	"first-project/internal/service"
	"first-project/web"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New merakit semua route dan middleware menjadi satu http.Handler.
func New(pool *pgxpool.Pool, productSvc *service.ProductService) http.Handler {
	mux := http.NewServeMux()

	// Health check: cek server + koneksi database
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

	// Halaman web (frontend). "{$}" artinya hanya alamat "/" persis,
	// sehingga URL lain yang tidak dikenal tetap 404.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(web.IndexHTML)
	})

	handler.NewProductHandler(productSvc).Register(mux)

	// Urutan middleware: Recover (luar) -> Logger -> CORS -> mux (dalam)
	return middleware.Recover(middleware.Logger(middleware.CORS(mux)))
}
