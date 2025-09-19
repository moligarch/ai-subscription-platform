import { getApiKey } from './session';

export type ApiError = {
  status: number;
  message: string;
  detail?: any;
};

async function req<T>(input: RequestInfo, init: RequestInit = {}): Promise<T> {
  const headers = init.headers ? new Headers(init.headers as any) : new Headers();
  const token = getApiKey();
  if (token) headers.set('Authorization', `Bearer ${token}`);
  headers.set('Content-Type', 'application/json');

  const res = await fetch(input, { ...init, headers });
  const text = await res.text();
  let body: any = null;
  try { body = text ? JSON.parse(text) : null; } catch { body = text; }

  if (!res.ok) {
    const err: ApiError = { status: res.status, message: body?.error || body?.message || res.statusText, detail: body };
    throw err;
  }
  return body as T;
}

export async function get<T>(path: string) { return req<T>(path, { method: 'GET' }); }
export async function post<T>(path: string, payload?: any) { return req<T>(path, { method: 'POST', body: payload ? JSON.stringify(payload) : undefined }); }
export async function put<T>(path: string, payload?: any) { return req<T>(path, { method: 'PUT', body: payload ? JSON.stringify(payload) : undefined }); }
export async function del<T>(path: string) { return req<T>(path, { method: 'DELETE' }); }
