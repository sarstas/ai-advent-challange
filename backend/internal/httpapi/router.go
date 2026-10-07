package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"aiadvent/backend/internal/config"
	"aiadvent/backend/internal/llm"
	"aiadvent/backend/internal/store"
)

// allowedModels — какие модели разрешено просить у бэкенда.
// Список заодно служит подсказкой фронтенду и даёт внятную ошибку на опечатку.
var allowedModels = []string{"deepseek-chat", "deepseek-reasoner"}

// RequestStore — то, что HTTP-слой требует от хранилища.
// Интерфейс (а не конкретный *store.Store) позволяет тестировать хендлеры без PostgreSQL.
type RequestStore interface {
	Ping(ctx context.Context) error
	Save(ctx context.Context, r store.Request) (store.Request, error)
	List(ctx context.Context, limit int) ([]store.Request, error)
	Stats(ctx context.Context) (total int, errors int, err error)
}

// Server держит зависимости хендлеров.
type Server struct {
	cfg   config.Config
	llm   llm.Client
	store RequestStore
	log   *slog.Logger
}

// New собирает роутер со всеми middleware.
func New(cfg config.Config, client llm.Client, st RequestStore, log *slog.Logger) http.Handler {
	s := &Server{cfg: cfg, llm: client, store: st, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("GET /api/history", s.handleHistory)

	return chain(mux, withRecover(log), withLogging(log))
}

func (s *Server) allowedModel(model string) bool {
	for _, m := range allowedModels {
		if m == model {
			return true
		}
	}
	return false
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not_found", "маршрут не найден")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "ai-advent-challange backend",
		"endpoints": map[string]string{
			"GET /api/health":  "состояние сервиса, БД и наличия ключа",
			"POST /api/chat":   `{"prompt": "..."} → ответ LLM`,
			"GET /api/history": "?limit=20 — последние запросы из PostgreSQL",
		},
		"models": allowedModels,
	})
}

type healthResponse struct {
	Status       string    `json:"status"` // ok | degraded
	DB           string    `json:"db"`     // ok | error: ...
	APIKey       string    `json:"api_key"`
	DefaultModel string    `json:"default_model"`
	Time         time.Time `json:"time"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 3*time.Second)
	defer cancel()

	resp := healthResponse{
		Status:       "ok",
		DB:           "ok",
		APIKey:       "missing",
		DefaultModel: s.cfg.DefaultModel,
		Time:         time.Now().UTC(),
	}
	if s.cfg.HasAPIKey() {
		resp.APIKey = "set"
	}
	status := http.StatusOK
	if err := s.store.Ping(ctx); err != nil {
		resp.Status, resp.DB, status = "degraded", "error: "+err.Error(), http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}
