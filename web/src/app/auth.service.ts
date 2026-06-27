import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, tap, BehaviorSubject, map, shareReplay, catchError, throwError } from 'rxjs';

export interface User {
  id: number;
  username: string;
  is_admin: boolean;
  created_at: string;
  updated_at: string;
}

interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
  user: User;
}

@Injectable({
  providedIn: 'root',
})
export class AuthService {
  private http = inject(HttpClient);
  private router = inject(Router);

  private tokenKey = 'prism_access_token';
  private refreshTokenKey = 'prism_refresh_token';
  private userKey = 'prism_user';

  private refreshObservable: Observable<any> | null = null;

  private currentUserSubject = new BehaviorSubject<User | null>(null);
  public currentUser$ = this.currentUserSubject.asObservable();

  constructor() {
    // Reload user from localStorage if present
    const cachedUser = localStorage.getItem(this.userKey);
    if (cachedUser) {
      try {
        this.currentUserSubject.next(JSON.parse(cachedUser));
      } catch (e) {
        this.clearLocalStorage();
      }
    }
  }

  public get currentUser(): User | null {
    return this.currentUserSubject.value;
  }

  public get isAdmin(): boolean {
    return this.currentUser?.is_admin || false;
  }

  public isLoggedIn(): boolean {
    return !!this.getToken();
  }

  public getToken(): string | null {
    return localStorage.getItem(this.tokenKey);
  }

  public isTokenExpired(token: string): boolean {
    try {
      const parts = token.split('.');
      if (parts.length !== 3) return true;
      const payload = JSON.parse(atob(parts[1]));
      if (!payload.exp) return false;
      return payload.exp * 1000 < Date.now();
    } catch (e) {
      return true;
    }
  }

  public login(username: string, password: string): Observable<LoginResponse> {
    return this.http.post<LoginResponse>('/api/v1/auth/login', { username, password }).pipe(
      tap((response) => {
        localStorage.setItem(this.tokenKey, response.access_token);
        localStorage.setItem(this.refreshTokenKey, response.refresh_token);
        localStorage.setItem(this.userKey, JSON.stringify(response.user));
        this.currentUserSubject.next(response.user);
      }),
    );
  }

  public refreshToken(): Observable<any> {
    if (this.refreshObservable) {
      return this.refreshObservable;
    }

    const refreshToken = localStorage.getItem(this.refreshTokenKey);
    if (!refreshToken) {
      return throwError(() => new Error('No refresh token available'));
    }

    this.refreshObservable = this.http.post<any>('/api/v1/auth/refresh', { refresh_token: refreshToken }).pipe(
      tap((response) => {
        localStorage.setItem(this.tokenKey, response.access_token);
        localStorage.setItem(this.refreshTokenKey, response.refresh_token);
        this.refreshObservable = null;
      }),
      catchError((err) => {
        this.refreshObservable = null;
        return throwError(() => err);
      }),
      shareReplay(1),
    );

    return this.refreshObservable;
  }

  public logout(): void {
    const refreshToken = localStorage.getItem(this.refreshTokenKey);
    // Fire-and-forget logout call to the server
    this.http.post('/api/v1/auth/logout', { refresh_token: refreshToken || '' }).subscribe({
      next: () => this.finalizeLogout(),
      error: () => this.finalizeLogout(), // Logout anyway on failure
    });
  }

  private finalizeLogout(): void {
    this.clearLocalStorage();
    this.currentUserSubject.next(null);
    this.router.navigate(['/login']);
  }

  private clearLocalStorage(): void {
    localStorage.removeItem(this.tokenKey);
    localStorage.removeItem(this.refreshTokenKey);
    localStorage.removeItem(this.userKey);
  }
}
