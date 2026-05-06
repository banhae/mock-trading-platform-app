import { request } from './client';

export interface LoginResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
}

export function login(username: string, password: string): Promise<LoginResponse> {
  return request<LoginResponse>('/api/auth/login', {
    method: 'POST',
    body: { username, password },
  });
}

export function authHealth(): Promise<unknown> {
  return request('/api/auth/health');
}

export function authReady(): Promise<unknown> {
  return request('/api/auth/ready');
}
