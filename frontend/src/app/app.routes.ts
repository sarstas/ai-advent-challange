import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    title: 'AI Advent Challenge — чат с DeepSeek',
    // Ленивая загрузка: страница чата попадает в бандл только при первом переходе.
    loadComponent: () =>
      import('./features/chat/chat-page').then((m) => m.ChatPage),
  },
  { path: '**', redirectTo: '' },
];
