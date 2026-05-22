import { Injectable, inject } from '@angular/core';
import { HttpInterceptor, HttpRequest, HttpHandler, HttpErrorResponse } from '@angular/common/http';
import { throwError, EMPTY } from 'rxjs';
import { catchError, switchMap } from 'rxjs/operators';
import { AuthService } from '../services/auth.service';
import { Router } from '@angular/router';

@Injectable()
export class AuthInterceptor implements HttpInterceptor {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);

  intercept(req: HttpRequest<unknown>, next: HttpHandler) {
    const token = this.auth.accessToken();
    const authed = token ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } }) : req;

    return next.handle(authed).pipe(
      catchError((err: HttpErrorResponse) => {
        if (err.status === 401 && !req.url.includes('/auth/')) {
          // Try to refresh then retry once.
          return this.auth.refreshAccessToken().pipe(
            switchMap((tokens) => {
              const retried = req.clone({
                setHeaders: { Authorization: `Bearer ${(tokens as any).access_token}` },
              });
              return next.handle(retried);
            }),
            catchError(() => {
              this.router.navigate(['/login']);
              return EMPTY;
            }),
          );
        }
        return throwError(() => err);
      }),
    );
  }
}
