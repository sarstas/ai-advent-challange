// Сервис AI Advent Challenge: принимает промпт по HTTP, спрашивает DeepSeek,
// складывает пару «промпт → ответ» в PostgreSQL и отдаёт ответ клиенту.
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

	"aiadvent/backend/internal/config"
	"aiadvent/backend/internal/httpapi"
	"aiadvent/backend/internal/llm"
	"aiadvent/backend/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("сервис остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg := config.Load()

	if !cfg.HasAPIKey() {
		log.Warn("DEEPSEEK_API_KEY не задан — /api/chat будет отвечать 401. Впиши ключ в .env")
	}

	// --- PostgreSQL ---
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		return err
	}
	log.Info("подключение к PostgreSQL установлено, миграции применены")

	// --- LLM ---
	client := llm.NewDeepSeek(cfg.DeepSeekBaseURL, cfg.DeepSeekAPIKey, cfg.DefaultModel, cfg.LLMTimeout)

	// --- HTTP ---
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.New(cfg, client, st, log),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Ответ модели может идти долго, поэтому write-таймаут заметно больше LLM_TIMEOUT.
		WriteTimeout: cfg.LLMTimeout + 30*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("backend слушает", "addr", "http://localhost:"+cfg.Port, "model", cfg.DefaultModel)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// --- graceful shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		log.Info("получен сигнал, останавливаемся", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("сервис остановлен корректно")
	return nil
}
