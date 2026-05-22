import { Injectable, signal, computed, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { tap, catchError, EMPTY } from 'rxjs';
import { environment } from '../../../environments/environment';
import type { LoginRequest, TokenResponse, User } from '../models';

const ACCESS_TOKEN_KEY = 'galactic_access_token';
const REFRESH_TOKEN_KEY = 'galactic_refresh_token';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);

  private _accessToken = signal<string | null>(localStorage.getItem(ACCESS_TOKEN_KEY));
  private _refreshToken = signal<string | null>(localStorage.getItem(REFRESH_TOKEN_KEY));

  readonly isLoggedIn = computed(() => !!this._accessToken());
  readonly accessToken = this._accessToken.asReadonly();

  login(credentials: LoginRequest) {
    return this.http
      .post<TokenResponse>(`${environment.apiBase}/auth/login`, credentials)
      .pipe(
        tap((tokens) => this.storeTokens(tokens)),
      );
  }

  logout() {
    const refreshToken = this._refreshToken();
    if (refreshToken) {
      this.http
        .post(`${environment.apiBase}/auth/logout`, { refresh_token: refreshToken })
        .pipe(catchError(() => EMPTY))
        .subscribe();
    }
    this.clearTokens();
    this.router.navigate(['/login']);
  }

  refreshAccessToken() {
    const refreshToken = this._refreshToken();
    if (!refreshToken) return EMPTY;
    return this.http
      .post<TokenResponse>(`${environment.apiBase}/auth/refresh`, { refresh_token: refreshToken })
      .pipe(tap((tokens) => this.storeTokens(tokens)));
  }

  getMe() {
    return this.http.get<User>(`${environment.apiBase}/me`);
  }

  private storeTokens(tokens: TokenResponse) {
    localStorage.setItem(ACCESS_TOKEN_KEY, tokens.access_token);
    localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refresh_token);
    this._accessToken.set(tokens.access_token);
    this._refreshToken.set(tokens.refresh_token);
  }

  private clearTokens() {
    localStorage.removeItem(ACCESS_TOKEN_KEY);
    localStorage.removeItem(REFRESH_TOKEN_KEY);
    this._accessToken.set(null);
    this._refreshToken.set(null);
  }
}
