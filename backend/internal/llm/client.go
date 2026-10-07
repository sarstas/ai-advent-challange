// Package llm описывает провайдер-независимый контракт общения с LLM
// и реализации этого контракта.
package llm

import (
	"context"
	"errors"
	"fmt"
)

// Message — одно сообщение в формате чата (роль + текст).
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest — запрос к модели.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Usage — сколько токенов потратили (приходит от провайдера).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse — нормализованный ответ модели.
type ChatResponse struct {
	Content string
	Model   string
	Usage   Usage
}

// Client — то, что умеет любой LLM-провайдер.
// Интерфейс позволяет подменить DeepSeek моком в тестах или Ollama локально.
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// Error — ошибка с HTTP-статусом и машинным кодом, который мы отдаём фронтенду.
type Error struct {
	Status  int    // статус, который вернём клиенту нашего API
	Code    string // машинный код: invalid_api_key, rate_limited, llm_timeout...
	Message string // человекочитаемое объяснение
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// AsError приводит любую ошибку к *Error, чтобы хендлер не разбирал типы.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Status: 500, Code: "internal_error", Message: err.Error()}
}
