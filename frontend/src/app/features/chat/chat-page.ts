import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';

import {
  ChatApiService,
  apiErrorMessage,
} from '../../core/chat-api.service';
import {
  ChatAnswer,
  DEFAULT_MODEL,
  HealthResponse,
  HistoryItem,
  MODELS,
  ModelId,
} from '../../core/models';

@Component({
  selector: 'app-chat-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe],
  templateUrl: './chat-page.html',
  styleUrl: './chat-page.scss',
})
export class ChatPage {
  private readonly api = inject(ChatApiService);

  readonly models = MODELS;

  // --- состояние формы ---
  readonly prompt = signal('');
  readonly model = signal<ModelId>(DEFAULT_MODEL);

  // --- состояние запроса ---
  readonly loading = signal(false);
  readonly error = signal<string | null>(null);
  readonly answer = signal<ChatAnswer | null>(null);
  readonly copied = signal(false);

  // --- история и состояние сервиса ---
  readonly history = signal<HistoryItem[]>([]);
  readonly loadingHistory = signal(false);
  readonly total = signal(0);
  readonly failed = signal(0);
  readonly health = signal<HealthResponse | null>(null);
  readonly healthError = signal(false);

  /** Кнопка активна, когда есть что отправлять и запрос ещё не летит. */
  readonly canSend = computed(
    () => this.prompt().trim().length > 0 && !this.loading(),
  );

  readonly statusText = computed(() => {
    const health = this.health();
    if (!health) {
      return this.healthError() ? 'бэкенд недоступен' : 'проверяю…';
    }
    if (health.db !== 'ok') {
      return 'PostgreSQL недоступен';
    }
    if (health.api_key === 'missing') {
      return 'нет ключа DeepSeek';
    }
    return `готов · ${health.default_model}`;
  });

  readonly statusOk = computed(() => {
    const health = this.health();
    return !!health && health.status === 'ok' && health.api_key === 'set';
  });

  constructor() {
    void this.reloadHistory();
    void this.refreshHealth();
  }

  // --- обработчики формы ---

  onPromptInput(event: Event): void {
    this.prompt.set((event.target as HTMLTextAreaElement).value);
  }

  onModelChange(event: Event): void {
    this.model.set((event.target as HTMLSelectElement).value as ModelId);
  }

  onSubmit(event: Event): void {
    event.preventDefault();
    void this.send();
  }

  /** Основной сценарий: промпт → бэкенд → DeepSeek → PostgreSQL → экран. */
  async send(): Promise<void> {
    if (!this.canSend()) {
      return;
    }
    const prompt = this.prompt().trim();

    this.loading.set(true);
    this.error.set(null);
    this.copied.set(false);

    try {
      const answer = await this.api.send(prompt, this.model());
      this.answer.set(answer);
      await this.reloadHistory();
    } catch (error) {
      // Текст ошибки приходит с бэкенда уже человекочитаемым.
      this.error.set(apiErrorMessage(error));
    } finally {
      this.loading.set(false);
    }
  }

  // --- история ---

  async reloadHistory(): Promise<void> {
    this.loadingHistory.set(true);
    try {
      const response = await this.api.loadHistory(20);
      this.history.set(response.items);
      this.total.set(response.total);
      this.failed.set(response.errors);
    } catch (error) {
      this.error.set(apiErrorMessage(error));
    } finally {
      this.loadingHistory.set(false);
    }
  }

  reuse(item: HistoryItem): void {
    this.prompt.set(item.prompt);
    this.model.set(
      MODELS.some((m) => m.id === item.model)
        ? (item.model as ModelId)
        : DEFAULT_MODEL,
    );
  }

  async refreshHealth(): Promise<void> {
    try {
      this.health.set(await this.api.loadHealth());
      this.healthError.set(false);
    } catch {
      this.health.set(null);
      this.healthError.set(true);
    }
  }

  async copy(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      this.error.set('Браузер запретил доступ к буферу обмена');
    }
  }
}
