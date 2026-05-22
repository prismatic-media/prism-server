import { Component, inject } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive } from '@angular/router';
import { AuthService } from '../core/services/auth.service';

@Component({
  selector: 'app-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  template: `
    <nav class="sidebar">
      <div class="logo">🌌 Galactic</div>
      <ul>
        <li><a routerLink="/" routerLinkActive="active" [routerLinkActiveOptions]="{exact:true}">Browse</a></li>
        <li><a routerLink="/history" routerLinkActive="active">Continue Watching</a></li>
        <li><a routerLink="/admin" routerLinkActive="active">Admin</a></li>
      </ul>
      <button class="logout-btn" (click)="logout()">Sign out</button>
    </nav>
    <main class="content">
      <router-outlet />
    </main>
  `,
  styleUrl: './shell.component.scss',
})
export class ShellComponent {
  private readonly auth = inject(AuthService);
  logout() { this.auth.logout(); }
}
