package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DeepSeek реализует Client поверх OpenAI-совместимого эндпоинта
// POST {baseURL}/chat/completions.
type DeepSeek struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewDeepSeek создаёт клиент. timeout страхует от бесконечно висящего запроса
// (модель deepseek-reasoner может думать долго).
func NewDeepSeek(baseURL, apiKey, defaultModel string, timeout time.Duration) *DeepSeek {
	return &DeepSeek{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   defaultModel,
		http:    &http.Client{Timeout: timeout},
	}
}

// apiResponse — только те поля ответа, которые нам нужны.
type apiResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Chat отправляет промпт в DeepSeek и возвращает ответ.
func (d *DeepSeek) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if len(req.Messages) == 0 {
		return ChatResponse{}, &Error{Status: http.StatusBadRequest, Code: "empty_messages", Message: "список сообщений пуст"}
	}
	if req.Model == "" {
		req.Model = d.model
	}

	body, err := json.Marshal(map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   false,
	})
	if err != nil {
		return ChatResponse{}, &Error{Status: 500, Code: "encode_request", Message: err.Error()}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, &Error{Status: 500, Code: "build_request", Message: err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.http.Do(httpReq)
	if err != nil {
		return ChatResponse{}, transportError(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ChatResponse{}, &Error{Status: http.StatusBadGateway, Code: "read_response", Message: err.Error()}
	}

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, providerError(resp.StatusCode, raw)
	}

	var parsed apiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ChatResponse{}, &Error{
			Status:  http.StatusBadGateway,
			Code:    "decode_response",
			Message: fmt.Sprintf("не удалось разобрать ответ провайдера: %v", err),
		}
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, &Error{Status: http.StatusBadGateway, Code: "empty_choices", Message: "провайдер вернул ответ без вариантов"}
	}

	model := parsed.Model
	if model == "" {
		model = req.Model
	}

	return ChatResponse{
		Content: parsed.Choices[0].Message.Content,
		Model:   model,
		Usage:   parsed.Usage,
	}, nil
}

// transportError переводит сетевые ошибки в понятные коды.
func transportError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &Error{Status: http.StatusGatewayTimeout, Code: "llm_timeout", Message: "модель не ответила за отведённое время, попробуй ещё раз или выбери deepseek-chat"}
	case errors.Is(err, context.Canceled):
		return &Error{Status: 499, Code: "client_canceled", Message: "запрос отменён клиентом"}
	default:
		return &Error{Status: http.StatusBadGateway, Code: "llm_unreachable", Message: fmt.Sprintf("не удалось связаться с DeepSeek: %v", err)}
	}
}

// providerError разбирает тело ошибки DeepSeek и маппит статус провайдера
// в статус нашего API.
func providerError(status int, raw []byte) error {
	var parsed apiResponse
	msg := strings.TrimSpace(string(raw))
	if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Error != nil && parsed.Error.Message != "" {
		msg = parsed.Error.Message
		if parsed.Error.Code != "" {
			msg = fmt.Sprintf("%s (%s)", msg, parsed.Error.Code)
		}
	}
	if len(msg) > 500 {
		msg = msg[:500] + "…"
	}

	switch status {
	case http.StatusBadRequest:
		return &Error{Status: http.StatusBadRequest, Code: "invalid_request", Message: msg}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &Error{Status: http.StatusUnauthorized, Code: "invalid_api_key", Message: "DeepSeek отклонил ключ: " + msg}
	case http.StatusPaymentRequired:
		return &Error{Status: http.StatusPaymentRequired, Code: "insufficient_balance", Message: "на балансе DeepSeek недостаточно средств: " + msg}
	case http.StatusTooManyRequests:
		return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "слишком много запросов, попробуй чуть позже: " + msg}
	default:
		return &Error{Status: http.StatusBadGateway, Code: "provider_error", Message: fmt.Sprintf("DeepSeek вернул %d: %s", status, msg)}
	}
}
