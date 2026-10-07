package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"aiadvent/backend/internal/store"
)

// contextWithTimeout берёт таймаут, но отвязывается от отмены исходного запроса.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), d)
}

// historyItem — одна запись журнала для фронтенда.
type historyItem struct {
	ID        int64     `json:"id"`
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	Model     string    `json:"model"`
	Usage     usageDTO  `json:"usage"`
	LatencyMS int       `json:"latency_ms"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type historyResponse struct {
	Items  []historyItem `json:"items"`
	Count  int           `json:"count"`
	Total  int           `json:"total"`
	Errors int           `json:"errors"`
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit должен быть целым числом больше нуля")
			return
		}
		limit = min(parsed, 100)
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	records, err := s.store.List(ctx, limit)
	if err != nil {
		s.log.Error("не удалось прочитать историю", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "не удалось прочитать историю из PostgreSQL")
		return
	}
	total, errors, err := s.store.Stats(ctx)
	if err != nil {
		s.log.Error("не удалось посчитать статистику", "error", err)
	}

	items := make([]historyItem, 0, len(records))
	for _, rec := range records {
		items = append(items, toHistoryItem(rec))
	}

	writeJSON(w, http.StatusOK, historyResponse{
		Items:  items,
		Count:  len(items),
		Total:  total,
		Errors: errors,
	})
}

func toHistoryItem(rec store.Request) historyItem {
	return historyItem{
		ID:        rec.ID,
		Prompt:    rec.Prompt,
		Response:  rec.Response,
		Model:     rec.Model,
		Usage:     usageDTO{rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens},
		LatencyMS: rec.LatencyMS,
		Status:    rec.Status,
		Error:     rec.ErrorMessage,
		CreatedAt: rec.CreatedAt,
	}
}
