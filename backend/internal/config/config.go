// Package config собирает настройки приложения из переменных окружения.
//
// Значения подставляются Makefile'ом из .env (см. `make be-run`), поэтому
// отдельная библиотека для чтения .env не нужна.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// placeholderKey — значение из .env.example; считаем его отсутствием ключа,
// чтобы ошибка была внятной, а не «401 от провайдера».
const placeholderKey = "sk-REPLACE_ME"

// Config — все настройки сервиса в одном месте.
type Config struct {
	Port            string
	DatabaseURL     string
	DeepSeekAPIKey  string
	DeepSeekBaseURL string
	DefaultModel    string
	LLMTimeout      time.Duration
}

// Load читает окружение и подставляет безопасные дефолты.
// Специально не падает без API-ключа: сервис должен подняться,
// отдать /api/health и объяснить проблему через /api/chat.
func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://aiadvent:aiadvent@localhost:5432/aiadvent?sslmode=disable"),
		DeepSeekAPIKey:  strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
		DeepSeekBaseURL: strings.TrimRight(getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"), "/"),
		DefaultModel:    getEnv("DEEPSEEK_MODEL", "deepseek-chat"),
		LLMTimeout:      time.Duration(getEnvInt("LLM_TIMEOUT_SECONDS", 120)) * time.Second,
	}
}

// HasAPIKey сообщает, можно ли идти в DeepSeek.
func (c Config) HasAPIKey() bool {
	return c.DeepSeekAPIKey != "" && c.DeepSeekAPIKey != placeholderKey
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return fallback
}
