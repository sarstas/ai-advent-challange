// Заглушка DeepSeek API: отвечает в формате OpenAI-совместимого
// /chat/completions, чтобы проверить всю цепочку (Angular → Go → БД)
// без реального ключа и без трат на токены.
//
// Запуск:  make mock-llm
// Бэкенд против неё: make be-run-mock
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

func main() {
	addr := ":9099"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	log.SetFlags(log.Ltime)
	http.HandleFunc("POST /chat/completions", handleChat)

	log.Printf("заглушка DeepSeek слушает http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	var req chatRequest
	_ = json.Unmarshal(raw, &req)

	prompt := ""
	if len(req.Messages) > 0 {
		prompt = req.Messages[len(req.Messages)-1].Content
	}
	model := req.Model
	if model == "" {
		model = "deepseek-chat"
	}

	log.Printf("<- model=%s auth=%q prompt=%q", model, r.Header.Get("Authorization"), prompt)

	// Небольшая задержка, чтобы спиннер в UI было видно.
	time.Sleep(300 * time.Millisecond)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":     "chatcmpl-mock",
		"object": "chat.completion",
		"model":  model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message{Role: "assistant", Content: mockAnswer(prompt)},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     len(strings.Fields(prompt)) + 3,
			"completion_tokens": 12,
			"total_tokens":      len(strings.Fields(prompt)) + 15,
		},
	})
}

func mockAnswer(prompt string) string {
	if strings.TrimSpace(prompt) == "" {
		return "Пустой промпт — но это заглушка, поэтому я всё равно отвечаю."
	}
	return "[заглушка DeepSeek] Ты написал: «" + prompt + "».\n\n" +
		"Реальный ответ придёт, когда в .env будет настоящий DEEPSEEK_API_KEY."
}
