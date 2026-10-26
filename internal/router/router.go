
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

	mux.Handle("/api/v1/", middleware.RequireSession(pool, protectedAPI))

	return middleware.Recover(
		middleware.Logger(
			middleware.CORS(mux),
		),
	)
}