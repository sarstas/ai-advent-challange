import { provideHttpClient } from '@angular/common/http';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { ChatPage } from './chat-page';

const health = {
  status: 'ok',
  db: 'ok',
  api_key: 'set',
  default_model: 'deepseek-chat',
  time: '2026-01-01T00:00:00Z',
};

const emptyHistory = { items: [], count: 0, total: 0, errors: 0 };

describe('ChatPage', () => {
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [ChatPage],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();

    http = TestBed.inject(HttpTestingController);
  });

  /** Компонент при старте сам просит health и историю — отвечаем на них. */
  function createPage() {
    const fixture = TestBed.createComponent(ChatPage);
    http.expectOne('/api/health').flush(health);
    http.expectOne((req) => req.url === '/api/history').flush(emptyHistory);
    return fixture;
  }

  it('создаётся и показывает состояние бэкенда', async () => {
    const fixture = createPage();
    await fixture.whenStable();
    fixture.detectChanges();

    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('AI Advent Challenge');
    expect(text).toContain('готов');
    http.verify();
  });

  it('кнопка отправки заблокирована, пока промпт пустой', async () => {
    const fixture = createPage();
    await fixture.whenStable();
    fixture.detectChanges();

    const button = (fixture.nativeElement as HTMLElement).querySelector(
      'button.primary',
    ) as HTMLButtonElement;
    expect(button.disabled).toBe(true);

    fixture.componentInstance.prompt.set('Привет');
    fixture.detectChanges();
    expect(button.disabled).toBe(false);
    http.verify();
  });

  it('отправляет промпт и показывает ответ модели', async () => {
    const fixture = createPage();
    const page = fixture.componentInstance;
    await fixture.whenStable();

    page.prompt.set('Что такое LLM?');
    page.send();

    const post = http.expectOne('/api/chat');
    expect(post.request.body).toEqual({
      prompt: 'Что такое LLM?',
      model: 'deepseek-chat',
    });
    post.flush({
      id: 7,
      content: 'LLM — это большая языковая модель.',
      model: 'deepseek-chat',
      usage: { prompt_tokens: 5, completion_tokens: 9, total_tokens: 14 },
      latency_ms: 1200,
      created_at: '2026-01-01T00:00:05Z',
    });

    // после ответа компонент перечитывает историю — даём промисам разрешиться
    await fixture.whenStable();
    http.expectOne((req) => req.url === '/api/history').flush({
      items: [
        {
          id: 7,
          prompt: 'Что такое LLM?',
          response: 'LLM — это большая языковая модель.',
          model: 'deepseek-chat',
          usage: { prompt_tokens: 5, completion_tokens: 9, total_tokens: 14 },
          latency_ms: 1200,
          status: 'ok',
          created_at: '2026-01-01T00:00:05Z',
        },
      ],
      count: 1,
      total: 1,
      errors: 0,
    });

    await fixture.whenStable();
    fixture.detectChanges();

    const element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('LLM — это большая языковая модель.');
    expect(element.textContent).toContain('14 токенов');
    expect(page.history().length).toBe(1);
    http.verify();
  });

  it('показывает понятную ошибку, если бэкенд вернул 401', async () => {
    const fixture = createPage();
    const page = fixture.componentInstance;
    await fixture.whenStable();

    page.prompt.set('привет');
    page.send();

    http.expectOne('/api/chat').flush(
      { error: { code: 'missing_api_key', message: 'DEEPSEEK_API_KEY не задан' } },
      { status: 401, statusText: 'Unauthorized' },
    );

    await fixture.whenStable();
    fixture.detectChanges();

    expect(page.error()).toBe('DEEPSEEK_API_KEY не задан');
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'DEEPSEEK_API_KEY не задан',
    );
    http.verify();
  });
});
