# AI Advent Challenge — первый проект: чат с LLM

Минимальное, но доведённое до рабочего состояния приложение: **Angular → Go → DeepSeek API → PostgreSQL**.
Пользователь вводит промпт в браузере, бэкенд на Go спрашивает модель через API, ответ возвращается
на экран и сохраняется в базу — под формой видна история запросов.

Закрывает задание первого этапа: *отправить запрос в LLM через API, получить ответ, вывести его в интерфейс.*

---

## Как это работает

```
┌──────────────────┐   POST /api/chat    ┌──────────────────┐   POST /chat/completions  ┌──────────────┐
│   Angular 22     │ ──────────────────► │  Go 1.23 backend │ ────────────────────────► │  DeepSeek    │
│   :4200          │ ◄────────────────── │  :8080           │ ◄──────────────────────── │  API         │
└──────────────────┘   JSON-ответ        └────────┬─────────┘   content + usage         └──────────────┘
       ▲                                          │ INSERT
       │ GET /api/history                         ▼
       │                                 ┌──────────────────┐
       └──────────────────────────────── │  PostgreSQL 17   │  журнал запросов и ответов
                                         │  :5432 (Docker)  │
                                         └──────────────────┘
```

Ключ DeepSeek живёт **только** на бэкенде. Фронтенд обращается к относительному `/api/...`,
в dev-режиме запросы проксирует Angular (`proxy.conf.json`) — поэтому CORS не нужен вообще.

## Стек

| Слой | Технология | Почему так |
|---|---|---|
| LLM | DeepSeek API (`deepseek-chat`, `deepseek-reasoner`) | OpenAI-совместимый формат: смена провайдера = смена `base_url` и модели |
| HTTP-клиент | стандартный `net/http` | в первом проекте важнее видеть протокол, чем прятать его за SDK; ноль лишних зависимостей |
| Бэкенд | Go 1.23, `net/http` с роутингом по методам | в Go 1.22+ `ServeMux` понимает `"POST /api/chat"` — фреймворк не нужен |
| БД | PostgreSQL 17 в Docker + `pgx/v5` (pgxpool) | нативный драйвер, пул соединений из коробки |
| Миграции | SQL-файлы через `embed` + таблица `schema_migrations` | применяются на старте, повторный запуск безопасен |
| Фронтенд | Angular 22: standalone, signals, `HttpClient` + `withFetch` | актуальные практики v22, zoneless по умолчанию |

## Быстрый старт

Нужны Docker, Go 1.23+ и Node.js 20+ с pnpm.

```bash
# 1. настройки: создаст .env из шаблона
make env

# 2. впиши свой ключ с https://platform.deepseek.com/api_keys
#    DEEPSEEK_API_KEY=sk-...

# 3. поднять PostgreSQL и дождаться статуса healthy
make db-up

# 4. установить зависимости фронтенда (один раз)
make fe-install

# 5. запустить бэкенд и фронтенд (в двух терминалах)
make be-run     # http://localhost:8080
make fe-run     # http://localhost:4200
```

Либо всё одной командой: `make dev` (БД + бэкенд + фронтенд, Ctrl+C останавливает всё).
Полный список команд — `make help`.

Проверка:

```bash
$ curl -s localhost:8080/api/health
{"status":"ok","db":"ok","api_key":"set","default_model":"deepseek-chat","time":"..."}

$ curl -s -X POST localhost:8080/api/chat -H 'Content-Type: application/json' \
    -d '{"prompt":"Объясни в двух словах, что такое LLM"}'
{"id":1,"content":"...","model":"deepseek-chat",
 "usage":{"prompt_tokens":12,"completion_tokens":57,"total_tokens":69},
 "latency_ms":1830,"created_at":"..."}
```

Открывай <http://localhost:4200> — это и есть интерфейс проекта.

## API

| Метод | Путь | Назначение |
|---|---|---|
| `GET` | `/api/health` | состояние сервиса, PostgreSQL и наличие ключа |
| `POST` | `/api/chat` | `{"prompt": "...", "model": "deepseek-chat"}` → ответ модели |
| `GET` | `/api/history?limit=20` | последние запросы и ответы из PostgreSQL |
| `GET` | `/` | краткая справка по эндпоинтам |

Ошибки приходят в едином формате `{"error":{"code":"...","message":"..."}}`:

| HTTP | `code` | Когда |
|---|---|---|
| 400 | `empty_prompt`, `invalid_json`, `unknown_model`, `invalid_limit` | некорректный запрос |
| 401 | `missing_api_key`, `invalid_api_key` | ключ не задан или отклонён DeepSeek |
| 402 | `insufficient_balance` | закончились средства на балансе |
| 429 | `rate_limited` | превышен лимит запросов |
| 502 | `provider_error`, `llm_unreachable` | сбой на стороне провайдера |
| 504 | `llm_timeout` | модель не ответила за `LLM_TIMEOUT_SECONDS` |

## База данных

```sql
CREATE TABLE llm_requests (
    id                BIGSERIAL   PRIMARY KEY,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    model             TEXT        NOT NULL,
    prompt            TEXT        NOT NULL,
    response          TEXT        NOT NULL DEFAULT '',
    prompt_tokens     INTEGER     NOT NULL DEFAULT 0,
    completion_tokens INTEGER     NOT NULL DEFAULT 0,
    total_tokens      INTEGER     NOT NULL DEFAULT 0,
    latency_ms        INTEGER     NOT NULL DEFAULT 0,
    status            TEXT        NOT NULL DEFAULT 'ok',   -- ok | error
    error_message     TEXT        NOT NULL DEFAULT ''
);
```

В журнал попадают **и успешные ответы, и ошибки**: без этого невозможно разобраться
ни с `429`, ни с таймаутами. Посмотреть данные:

```bash
make psql
select id, model, status, total_tokens, latency_ms, left(prompt, 60) from llm_requests order by id desc limit 10;
```

Есть и веб-морда: `make pgadmin` → <http://localhost:5050>.

## Тесты

```bash
make be-test                    # Go: клиент DeepSeek (httptest) + хендлеры с фейковым хранилищем
cd frontend && pnpm test        # Angular: 4 теста страницы чата через HttpTestingController
```

Тесты не требуют ни ключа, ни запущенной БД: клиент DeepSeek проверяется на поддельном
HTTP-сервере, а хранилище подменяется интерфейсом `RequestStore`.

## Разработка без ключа DeepSeek

Чтобы посмотреть UI и проверить всю цепочку, не тратя токены:

```bash
make mock-llm       # в отдельном терминале: заглушка DeepSeek на :9099
make be-run-mock    # бэкенд, направленный на заглушку
make fe-run
```

Или совсем одной командой: `make dev-mock`.
Заглушка (`backend/cmd/fakellm`) отвечает в формате OpenAI-совместимого API,
так что код бэкенда работает без изменений.

## Структура репозитория

```
ai-advent-challange/
├── compose.yaml                 # PostgreSQL 17 (+ pgAdmin в profile tools)
├── Makefile                     # все команды разработки: make help
├── .env.example                 # шаблон настроек, .env в .gitignore
├── backend/
│   ├── cmd/server/main.go       # сборка зависимостей, graceful shutdown
│   ├── cmd/fakellm/main.go      # заглушка DeepSeek для локальной разработки
│   └── internal/
│       ├── config/              # чтение переменных окружения
│       ├── llm/                 # интерфейс Client + реализация DeepSeek + тесты
│       ├── store/               # pgxpool, миграции, Save/List/Stats
│       └── httpapi/             # роутер, хендлеры, middleware, тесты
└── frontend/
    ├── proxy.conf.json          # /api → localhost:8080 (CORS не нужен)
    └── src/app/
        ├── core/                # модели и ChatApiService — единственное место про HTTP
        └── features/chat/       # страница чата (signals + OnPush) и её тесты
```

## Решения, которые стоит объяснить

- **Ключ только на бэкенде.** Фронтенд не знает `DEEPSEEK_API_KEY`; в бандл он попасть не может
  (проверено: значения ключа нет ни в `frontend/dist`, ни в `frontend/src`).
- **`context.WithTimeout` на каждый вызов LLM.** `deepseek-reasoner` думает долго, поэтому таймаут
  настраивается через `LLM_TIMEOUT_SECONDS` и отдельно превращается в понятный `504 llm_timeout`.
- **Запись в БД в отдельном контексте** (`context.WithoutCancel`): даже если пользователь закрыл
  вкладку, факт обращения к модели останется в журнале.
- **БД не ломает основной сценарий.** Если запись в PostgreSQL упала, ответ модели всё равно
  возвращается пользователю, а ошибка уходит в лог.
- **Разрешённый список моделей** на бэкенде: опечатка в названии модели даёт внятную `400`,
  а не загадочную ошибку провайдера.
- **Интерфейсы `llm.Client` и `RequestStore`** — благодаря им хендлеры тестируются без сети и без БД.

## Если что-то не работает

| Симптом | Причина и решение |
|---|---|
| `PostgreSQL недоступен (проверь make db-up)` | контейнер не поднят: `make db-up`, статус — `make db-ps` |
| `{"code":"missing_api_key"}` | не задан `DEEPSEEK_API_KEY` в `.env` или бэкенд не перезапущен после правки |
| `llm_timeout` | модель думает дольше лимита: увеличь `LLM_TIMEOUT_SECONDS` в `.env` |
| `npm error code EPERM` про `~/.npm` | в кэше npm есть root-файлы: `sudo chown -R 501:20 ~/.npm` |
| `png`/`esbuild` не собирается | pnpm заблокировал postinstall: настройка `allowBuilds` уже в `frontend/pnpm-workspace.yaml`, выполни `pnpm install` |
| порт занят | смени `PORT`/`POSTGRES_PORT` в `.env` или останови старый процесс: `lsof -i :8080` |

## Что дальше

- стриминг ответа токенами (SSE) — самый заметный апгрейд для демо;
- несколько сообщений в диалоге (сейчас каждый запрос независим);
- подсчёт стоимости по токенам и лимиты;
- Dockerfile для бэкенда и общий `docker compose up` для всего стека;
- полноценные миграции (golang-migrate) и `GET /api/history/{id}`.
