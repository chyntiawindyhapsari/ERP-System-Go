package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"first-project/internal/config"
	"first-project/internal/database"
	"first-project/internal/repository"
	"first-project/internal/router"
	"first-project/internal/service"
)

func main() {
	// 1. Baca konfigurasi dari environment / file .env
	cfg := config.Load()

	// Context ini otomatis dibatalkan saat menekan Ctrl+C (graceful shutdown)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Koneksi ke PostgreSQL
	pool, err := database.Connect(ctx, cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("gagal konek database: %v", err)
	}
	defer pool.Close()
	log.Println("terhubung ke PostgreSQL")

	// 3. Jalankan migrasi (membuat tabel jika belum ada)
	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatalf("gagal migrasi: %v", err)
	}

	// 4. Susun layer: repository -> service -> handler (di dalam router)
	productRepo := repository.NewProductRepository(pool)
	productSvc := service.NewProductService(productRepo)
	handler := router.New(pool, productSvc)

	// 5. Jalankan HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server berjalan di http://localhost:%s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// 6. Tunggu Ctrl+C, lalu matikan server dengan rapi
	<-ctx.Done()
	log.Println("mematikan server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("server berhenti")
}
