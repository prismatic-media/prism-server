import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

export const authGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  if (auth.isLoggedIn()) return true;
  return router.createUrlTree(['/login']);
};

export const adminGuard: CanActivateFn = () => {
  // We derive admin status from the JWT payload.
  // If not logged in, redirect to login.
  const auth = inject(AuthService);
  const router = inject(Router);
  if (!auth.isLoggedIn()) return router.createUrlTree(['/login']);

  const token = auth.accessToken();
  if (!token) return router.createUrlTree(['/login']);

  try {
    const payload = JSON.parse(atob(token.split('.')[1]));
    if (payload.adm === true) return true;
  } catch {
    // ignore malformed token
  }
  return router.createUrlTree(['/']);
};
