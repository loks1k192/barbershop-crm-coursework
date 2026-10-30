// Сервис core — REST API CRM сети барбершопов поверх единой БД PostgreSQL.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RomanKovalev007/barber_crm/services/core/internal/api"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/config"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/db"
	"github.com/RomanKovalev007/barber_crm/services/core/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.MaxConns)
	if err != nil {
		log.Error("database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	st := store.New(pool)

	// Механизм обновления материализованного представления KPI.
	if cfg.KPIRefreshInterval > 0 {
		go func() {
			t := time.NewTicker(cfg.KPIRefreshInterval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if ts, err := st.RefreshKPI(ctx); err != nil {
						log.Error("kpi refresh", "error", err)
					} else {
						log.Info("kpi refreshed", "at", ts)
					}
				}
			}
		}()
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(st, log).Router(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	go func() {
		log.Info("core started", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("core stopped")
}
