import { ChangeDetectionStrategy, Component } from '@angular/core';
import { RouterOutlet } from '@angular/router';

/**
 * Корневой компонент — только оболочка страницы.
 * Вся работа с LLM живёт в лениво загружаемом ChatPage.
 */
@Component({
  selector: 'app-root',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterOutlet],
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App {}
