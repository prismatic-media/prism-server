import { Component, inject, effect } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { AuthService } from './core/services/auth.service';
import { RealtimeService } from './core/services/realtime.service';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [RouterOutlet],
  template: `<router-outlet />`,
  styles: [':host { display: block; height: 100vh; }'],
})
export class App {
  constructor() {
    const auth = inject(AuthService);
    const realtime = inject(RealtimeService);
    effect(() => {
      if (auth.isLoggedIn()) {
        realtime.connect();
      } else {
        realtime.disconnect();
      }
    });
  }
}
