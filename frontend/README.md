# Frontend — Angular 22

Клиент проекта AI Advent Challenge: одна страница, на которой можно отправить промпт
в LLM через Go-бэкенд и посмотреть историю запросов из PostgreSQL.

Как запускать весь проект целиком — см. [README в корне репозитория](../README.md).

## Полезные команды

```bash
pnpm install     # установить зависимости
pnpm start       # dev-server на http://localhost:4200 (API проксируется на :8080)
pnpm build       # production-сборка в dist/
pnpm test        # unit-тесты (vitest)
```

Бэкенд должен быть запущен (`make be-run` в корне), иначе кнопка отправки вернёт
«Бэкенд недоступен». Если ключа DeepSeek нет — поднимите заглушку: `make dev-mock`.

## Как устроено

| Файл | Роль |
|---|---|
| `src/app/core/models.ts` | типы ответов бэкенда и список разрешённых моделей |
| `src/app/core/chat-api.service.ts` | единственное место, где фронтенд знает про HTTP |
| `src/app/features/chat/chat-page.ts` | состояние страницы на signals (`prompt`, `loading`, `answer`, `history`) |
| `src/app/features/chat/chat-page.html` | шаблон: форма, карточка ответа, история |
| `proxy.conf.json` | `/api` → `http://localhost:8080`, поэтому CORS не нужен |

Страница загружается лениво (`loadComponent` в `app.routes.ts`), компоненты — standalone,
с `ChangeDetectionStrategy.OnPush` и без `zone.js`.

Ключ DeepSeek во фронтенд не попадает: запрос уходит на свой бэкенд, ключ подставляет он.
