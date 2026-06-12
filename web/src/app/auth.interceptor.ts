import { HttpInterceptorFn, HttpErrorResponse } from '@angular/common/http';
import { inject, Injector } from '@angular/core';
import { Router } from '@angular/router';
import { AuthService } from './auth.service';
import { catchError, switchMap, throwError } from 'rxjs';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const injector = inject(Injector);
  const authService = injector.get(AuthService);
  const token = authService.getToken();

  // Append Bearer token if present and the request targets local API routes
  let authReq = req;
  if (token && req.url.includes('/api/')) {
    authReq = req.clone({
      setHeaders: {
        Authorization: `Bearer ${token}`
      }
    });
  }

  return next(authReq).pipe(
    catchError((error) => {
      if (
        error instanceof HttpErrorResponse &&
        error.status === 503 &&
        error.error &&
        (error.error.redirect === '/setup' || (typeof error.error === 'string' && error.error.includes('setup')))
      ) {
        const router = injector.get(Router);
        router.navigate(['/setup']);
        return throwError(() => error);
      }
      // Intercept 401 Unauthorized errors for general API requests (excluding auth endpoints)
      if (
        error instanceof HttpErrorResponse &&
        error.status === 401 &&
        req.url.includes('/api/') &&
        !req.url.includes('/auth/login') &&
        !req.url.includes('/auth/refresh') &&
        !req.url.includes('/auth/logout')
      ) {
        return authService.refreshToken().pipe(
          switchMap((res: any) => {
            // Re-clone original request with the new active token and retry the API call
            const retriedReq = req.clone({
              setHeaders: {
                Authorization: `Bearer ${res.access_token}`
              }
            });
            return next(retriedReq);
          }),
          catchError((refreshError) => {
            // Force logout and redirect if refresh endpoint fails (session expired)
            authService.logout();
            return throwError(() => refreshError);
          })
        );
      }
      return throwError(() => error);
    })
  );
};
