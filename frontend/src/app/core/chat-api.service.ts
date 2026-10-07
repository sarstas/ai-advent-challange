import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';

import {
  ApiErrorBody,
  ChatAnswer,
  HealthResponse,
  HistoryResponse,
  ModelId,
} from './models';

/**
 * Единственное место, где фронтенд знает про HTTP.
 * Пути относительные: в dev-режиме их проксирует Angular (proxy.conf.json → localhost:8080),
 * поэтому браузеру не нужен CORS, а ключ DeepSeek живёт только на бэкенде.
 */
@Injectable({ providedIn: 'root' })
export class ChatApiService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api';

  /** Отправляет промпт в LLM через бэкенд. */
  send(prompt: string, model: ModelId): Promise<ChatAnswer> {
    return firstValueFrom(
      this.http.post<ChatAnswer>(`${this.base}/chat`, { prompt, model }),
    );
  }

  /** Последние запросы из PostgreSQL. */
  loadHistory(limit = 20): Promise<HistoryResponse> {
    return firstValueFrom(
      this.http.get<HistoryResponse>(`${this.base}/history`, {
        params: { limit },
      }),
    );
  }

  /** Состояние сервиса: живо ли API, БД и задан ли ключ. */
  loadHealth(): Promise<HealthResponse> {
    return firstValueFrom(this.http.get<HealthResponse>(`${this.base}/health`));
  }
}

/** Превращает любую ошибку в текст, который не стыдно показать пользователю. */
export function apiErrorMessage(error: unknown): string {
  if (error instanceof HttpErrorResponse) {
    if (error.status === 0) {
      return 'Бэкенд недоступен. Запусти его командой `make be-run` и обнови страницу.';
    }
    const body = error.error as ApiErrorBody | null;
    if (body?.error?.message) {
      return body.error.message;
    }
    return `Ошибка ${error.status}: ${error.statusText}`;
  }
  return error instanceof Error ? error.message : 'Неизвестная ошибка';
}
