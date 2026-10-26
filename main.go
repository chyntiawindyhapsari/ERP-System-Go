
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
	"first-project/internal/handler"
	"first-project/internal/repository"
	"first-project/internal/router"
	"first-project/internal/service"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("gagal konek database: %v", err)
	}
	defer pool.Close()

	log.Println("terhubung ke PostgreSQL")

	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatalf("gagal migrasi: %v", err)
	}

	authHandler := handler.NewAuthHandler(pool)
	if err := authHandler.EnsureAdmin(
		cfg.AdminUsername,
		cfg.AdminPassword,
		cfg.AdminFullName,
	); err != nil {
		log.Fatalf("gagal menyiapkan admin: %v", err)
	}

	productRepo := repository.NewProductRepository(pool)
	productSvc := service.NewProductService(productRepo)
	appHandler := router.New(pool, productSvc)

	srv := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      appHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server berjalan di http://localhost:%s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Println("mematikan server...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	log.Println("server berhenti")
}
