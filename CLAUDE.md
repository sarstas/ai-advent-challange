# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

LLM chat app: **Angular 22 (`frontend/`) → Go backend (`backend/`) → DeepSeek API**, with every request/response (including failures) logged to **PostgreSQL 17** (Docker). The project language is Russian: code comments, log messages, user-facing error messages, README and commit messages (conventional-commit prefix + Russian text, e.g. `feat(frontend): ...`) are written in Russian — keep that style.

## Commands

All dev commands go through the root `Makefile` (`make help` lists them). The Makefile `-include`s `.env` and exports it, so the Go backend reads config purely from env vars (no dotenv package) — run the backend via `make`, or export the vars yourself.

```bash
make env            # create .env from .env.example (then set DEEPSEEK_API_KEY)
make db-up          # start PostgreSQL and wait for healthy; make db-reset wipes the volume
make psql           # psql inside the container
make be-run         # Go backend on :8080 (applies migrations on start; fails if DB is down)
make fe-install     # pnpm install (once)
make fe-run         # Angular dev server on :4200
make dev            # db + backend + frontend in one terminal
make dev-mock       # same, but backend points at the fake LLM (no key/tokens needed)
make mock-llm / make be-run-mock   # fake DeepSeek on :9099 + backend wired to it, separately

make be-test        # cd backend && go test ./...
make be-fmt         # gofmt -w
make fe-build
cd frontend && pnpm test   # Angular unit tests (Vitest via @angular/build:unit-test, jsdom)
```

Single tests:

```bash
cd backend && go test ./internal/httpapi -run TestChatEmptyPrompt -v
cd frontend && pnpm test -- --include src/app/features/chat/chat-page.spec.ts
```

Neither test suite needs the DB or a DeepSeek key.

## Architecture

Backend (Go module `aiadvent/backend`, stdlib `net/http` with Go 1.22+ method routing — no web framework):

- `cmd/server/main.go` — wiring: `config.Load()` → `store.New` + `Migrate` → `llm.NewDeepSeek` → `httpapi.New`, graceful shutdown. `WriteTimeout` is derived from `LLM_TIMEOUT_SECONDS`.
- `cmd/fakellm` — OpenAI-compatible stub of DeepSeek; the backend uses it unchanged via `DEEPSEEK_BASE_URL`.
- `internal/llm` — provider-agnostic `Client` interface plus DeepSeek implementation over raw `net/http`. Provider/transport failures are mapped to `*llm.Error{Status, Code, Message}`; handlers call `llm.AsError(err)` and pass `Status`/`Code` straight to the API response (e.g. `rate_limited`, `llm_timeout` → 504). New error cases belong in this mapping, not in handlers.
- `internal/store` — pgxpool; migrations are `migrations/*.sql` embedded via `embed`, applied in order on startup and tracked in `schema_migrations` (add a new numbered file; never edit an applied one). Single table `llm_requests`.
- `internal/httpapi` — router, handlers, middleware (recover, logging). Handlers depend on the `RequestStore` interface (defined in `router.go`) and `llm.Client`, so tests use fakes instead of PostgreSQL/network. Endpoints: `GET /api/health`, `POST /api/chat`, `GET /api/history?limit=`.
  - Errors are JSON `{code, message}` via `writeError`; the frontend relies on `code`.
  - Model whitelist `allowedModels` in `router.go` (`deepseek-chat`, `deepseek-reasoner`); unknown model → 400 `unknown_model`.
  - `/api/chat` intentionally: wraps the LLM call in `context.WithTimeout(LLM_TIMEOUT)`; writes to the DB in a context detached from the request (`context.WithoutCancel`) so the log survives a client disconnect; logs errors as `status='error'` rows too; and still returns the model's answer if the DB write fails.
  - Each chat request is a single user message — there is no multi-turn conversation state yet.

Frontend (`frontend/src/app`, standalone components, signals, zoneless, OnPush, `HttpClient` with `withFetch`):

- `core/chat-api.service.ts` is the only place that does HTTP; `core/models.ts` mirrors the backend JSON DTOs — keep them in sync when changing backend responses.
- `features/chat/` — the chat page; tests use `HttpTestingController`.
- Calls relative `/api/...`; in dev `proxy.conf.json` forwards to `localhost:8080`, so the backend has no CORS handling. The DeepSeek key lives only in the backend env and must never reach the frontend.

## Gotchas

- pnpm blocks postinstall scripts; `allowBuilds` in `frontend/pnpm-workspace.yaml` permits the needed ones (esbuild etc.).
- After editing `.env`, restart the backend — config is read once at startup.
