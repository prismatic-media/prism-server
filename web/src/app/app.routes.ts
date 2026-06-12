import { Routes } from '@angular/router';
import { authGuard, loginGuard } from './auth.guard';

export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () => import('./login/login.component').then(m => m.LoginComponent),
    canActivate: [loginGuard]
  },
  {
    path: 'setup',
    loadComponent: () => import('./setup/setup.component').then(m => m.SetupComponent)
  },
  {
    path: 'watch/:id',
    loadComponent: () => import('./player/player.component').then(m => m.PlayerComponent),
    canActivate: [authGuard]
  },
  {
    path: '',
    loadComponent: () => import('./layout/layout.component').then(m => m.LayoutComponent),
    canActivate: [authGuard],
    children: [
      {
        path: '',
        loadComponent: () => import('./home/home.component').then(m => m.HomeComponent)
      },
      {
        path: 'movies',
        loadComponent: () => import('./movies/movies.component').then(m => m.MoviesComponent)
      },
      {
        path: 'movies/:id',
        loadComponent: () => import('./media-details/media-details.component').then(m => m.MediaDetailsComponent)
      },
      {
        path: 'tv-shows',
        loadComponent: () => import('./tv-shows/tv-shows.component').then(m => m.TVShowsComponent)
      },
      {
        path: 'tv-shows/:id',
        loadComponent: () => import('./media-details/media-details.component').then(m => m.MediaDetailsComponent)
      },
      {
        path: 'admin/library',
        loadComponent: () => import('./admin/library/library-admin.component').then(m => m.LibraryAdminComponent)
      },
      {
        path: 'admin/storage',
        loadComponent: () => import('./admin/storage/storage-admin.component').then(m => m.StorageAdminComponent)
      },
      {
        path: 'admin/transcoding',
        loadComponent: () => import('./admin/transcoding/transcoding-admin.component').then(m => m.TranscodingAdminComponent)
      },
      {
        path: 'admin/settings',
        loadComponent: () => import('./admin/settings/settings-admin.component').then(m => m.SettingsAdminComponent)
      }
    ]
  },
  {
    path: '**',
    redirectTo: ''
  }
];
