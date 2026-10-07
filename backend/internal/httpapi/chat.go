package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"aiadvent/backend/internal/llm"
	"aiadvent/backend/internal/store"
)

const (
	maxPromptBytes = 32 << 10 // 32 KiB — с запасом на длинные промпты
	maxPromptRunes = 8000
)

// usageDTO — токены в ответе API.
type usageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatResponse — успешный ответ POST /api/chat.
type chatResponse struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	Model     string    `json:"model"`
	Usage     usageDTO  `json:"usage"`
	LatencyMS int       `json:"latency_ms"`
	CreatedAt time.Time `json:"created_at"`
}

type chatRequestBody struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPromptBytes)

	var body chatRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", `тело запроса — JSON вида {"prompt": "привет"}`)
		return
	}

	prompt := strings.TrimSpace(body.Prompt)
	switch {
	case prompt == "":
		writeError(w, http.StatusBadRequest, "empty_prompt", "промпт не может быть пустым")
		return
	case len([]rune(prompt)) > maxPromptRunes:
		writeError(w, http.StatusBadRequest, "prompt_too_long", "промпт длиннее 8000 символов")
		return
	}

	model := strings.TrimSpace(body.Model)
	if model == "" {
		model = s.cfg.DefaultModel
	}
	if !s.allowedModel(model) {
		writeError(w, http.StatusBadRequest, "unknown_model",
			"неизвестная модель "+model+"; доступны: "+strings.Join(allowedModels, ", "))
		return
	}

	if !s.cfg.HasAPIKey() {
		writeError(w, http.StatusUnauthorized, "missing_api_key",
			"DEEPSEEK_API_KEY не задан. Впиши ключ в .env и перезапусти бэкенд")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.LLMTimeout)
	defer cancel()

	start := time.Now()
	answer, err := s.llm.Chat(ctx, llm.ChatRequest{
		Model:    model,
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	})
	latencyMS := int(time.Since(start).Milliseconds())

	// Журналируем в отдельном контексте: даже если клиент отвалился,
	// запись в БД должна состояться.
	dbCtx, dbCancel := contextWithTimeout(r, 5*time.Second)
	defer dbCancel()

	if err != nil {
		apiErr := llm.AsError(err)
		if _, saveErr := s.store.Save(dbCtx, store.Request{
			Model:        model,
			Prompt:       prompt,
			LatencyMS:    latencyMS,
			Status:       "error",
			ErrorMessage: apiErr.Message,
		}); saveErr != nil {
			s.log.Error("не удалось сохранить ошибку в БД", "error", saveErr)
		}
		s.log.Error("запрос к LLM не удался", "code", apiErr.Code, "model", model, "latency_ms", latencyMS)
		writeError(w, apiErr.Status, apiErr.Code, apiErr.Message)
		return
	}

	record, err := s.store.Save(dbCtx, store.Request{
		Model:            answer.Model,
		Prompt:           prompt,
		Response:         answer.Content,
		PromptTokens:     answer.Usage.PromptTokens,
		CompletionTokens: answer.Usage.CompletionTokens,
		TotalTokens:      answer.Usage.TotalTokens,
		LatencyMS:        latencyMS,
		Status:           "ok",
	})
	if err != nil {
		// БД не должна ломать основной сценарий — ответ всё равно отдаём.
		s.log.Error("не удалось сохранить запрос в БД", "error", err)
	}

	s.log.Info("ответ LLM получен",
		"model", answer.Model,
		"tokens", answer.Usage.TotalTokens,
		"latency_ms", latencyMS,
	)

	writeJSON(w, http.StatusOK, chatResponse{
		ID:        record.ID,
		Content:   answer.Content,
		Model:     answer.Model,
		Usage:     usageDTO{answer.Usage.PromptTokens, answer.Usage.CompletionTokens, answer.Usage.TotalTokens},
		LatencyMS: latencyMS,
		CreatedAt: record.CreatedAt,
	})
}
