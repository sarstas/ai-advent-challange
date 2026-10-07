/** Типы, которыми обменивается Angular с Go-бэкендом. */

export interface Usage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

/** Ответ POST /api/chat */
export interface ChatAnswer {
  id: number;
  content: string;
  model: string;
  usage: Usage;
  latency_ms: number;
  created_at: string;
}

/** Одна запись журнала из GET /api/history */
export interface HistoryItem {
  id: number;
  prompt: string;
  response: string;
  model: string;
  usage: Usage;
  latency_ms: number;
  status: 'ok' | 'error';
  error?: string;
  created_at: string;
}

export interface HistoryResponse {
  items: HistoryItem[];
  count: number;
  total: number;
  errors: number;
}

/** Ответ GET /api/health */
export interface HealthResponse {
  status: 'ok' | 'degraded';
  db: string;
  api_key: 'set' | 'missing';
  default_model: string;
  time: string;
}

/** Единый формат ошибки бэкенда */
export interface ApiErrorBody {
  error: { code: string; message: string };
}

/** Модели DeepSeek, которые разрешены на бэкенде (см. allowedModels в Go). */
export const MODELS = [
  { id: 'deepseek-chat', label: 'deepseek-chat — быстрая (V3)' },
  { id: 'deepseek-reasoner', label: 'deepseek-reasoner — с рассуждениями (R1)' },
] as const;

export type ModelId = (typeof MODELS)[number]['id'];

export const DEFAULT_MODEL: ModelId = 'deepseek-chat';
