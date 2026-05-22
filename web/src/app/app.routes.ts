import { Routes } from '@angular/router';
import { authGuard, adminGuard } from './core/guards/auth.guard';
import { ShellComponent } from './shell/shell.component';

export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () =>
      import('./features/login/login.component').then((m) => m.LoginComponent),
  },
  {
    path: '',
    component: ShellComponent,
    canActivate: [authGuard],
    children: [
      {
        path: '',
        loadComponent: () =>
          import('./features/library/library.component').then((m) => m.LibraryComponent),
      },
      {
        path: 'media/:id',
        loadComponent: () =>
          import('./features/media-detail/media-detail.component').then(
            (m) => m.MediaDetailComponent,
          ),
      },
      {
        path: 'player/:id',
        loadComponent: () =>
          import('./features/player/player.component').then((m) => m.PlayerComponent),
      },
      {
        path: 'history',
        loadComponent: () =>
          import('./features/history/history.component').then((m) => m.HistoryComponent),
      },
      {
        path: 'admin',
        loadComponent: () =>
          import('./features/admin/admin.component').then((m) => m.AdminComponent),
        canActivate: [adminGuard],
      },
    ],
  },
  { path: '**', redirectTo: '' },
];

