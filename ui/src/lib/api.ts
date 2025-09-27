// file src/lib/api.ts
// NOTE: Session-cookie auth edition.
// - No Authorization header is sent anymore.
// - The browser stores an HttpOnly "admin_session" cookie after calling login.
// - All subsequent /api/* calls rely on same-origin cookies.

export type ApiError = {
  status: number;
  message: string;
  detail?: any;
};

async function fetchWithTimeout(url: string, options: RequestInit, timeout = 100000): Promise<Response> {
  const res = await Promise.race([
    fetch(url, options),
    new Promise<Response>((_, reject) => setTimeout(() => reject(new Error('Request timed out')), timeout)),
  ]);
  if (!(res instanceof Response)) {
    throw res; // Ensure only Response is returned
  }
  return res;
}

async function req<T>(input: RequestInfo, init: RequestInit = {}): Promise<T> {
  // IMPORTANT: use same-origin cookies; do NOT set Authorization header
  const headers = new Headers(init.headers || {});
  // Only set Content-Type for requests with a body
  if (init.method && init.method !== 'GET' && !headers.has('content-type')) {
    headers.set('content-type', 'application/json');
  }

  const url = typeof input === 'string' ? input : input.url;

  const res = await fetchWithTimeout(
    url,
    {
      ...init,
      headers,
      credentials: 'same-origin', // <- cookie-based auth
    },
    100000,
  );

  // 204: no content
  if (res.status === 204) {
    // @ts-ignore
    return undefined as T;
  }

  const text = await res.text().catch(() => '');
  let body: any = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }

  if (!res.ok) {
    const err: ApiError = {
      status: res.status,
      message: body?.error || body?.message || res.statusText,
      detail: body,
    };
    throw err;
  }

  // Non-JSON responses: return undefined to avoid crashes
  const ct = res.headers.get('content-type') || '';
  if (!ct.toLowerCase().includes('application/json')) {
    // @ts-ignore
    return undefined as T;
  }

  return body as T;
}

// --------------- Public helpers ---------------

export async function get<T>(path: string) {
  return req<T>(path, { method: 'GET' });
}

export async function post<T>(path: string, payload?: any) {
  return req<T>(path, {
    method: 'POST',
    body: payload !== undefined ? JSON.stringify(payload) : undefined,
  });
}

export async function put<T>(path: string, payload?: any) {
  return req<T>(path, {
    method: 'PUT',
    body: payload !== undefined ? JSON.stringify(payload) : undefined,
  });
}

export async function del<T>(path: string) {
  return req<T>(path, { method: 'DELETE' });
}

// --------------- Auth endpoints (session cookie) ---------------

/**
 * Logs in with the Admin API key and establishes a session via HttpOnly cookie.
 * On success, server returns 204 No Content and sets "admin_session".
 */
export async function loginWithApiKey(key: string): Promise<void> {
  const res = await fetch('/api/v1/admin/auth/login', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ key }),
  });
  if (res.status === 204) return;
  if (res.status === 401) throw new Error('invalid api key');
  const text = await res.text().catch(() => '');
  throw new Error(text || `login failed: ${res.status}`);
}

/**
 * Logs out and clears the session cookie.
 */
export async function logout(): Promise<void> {
  await fetch('/api/v1/admin/auth/logout', {
    method: 'POST',
    credentials: 'same-origin',
  });
}
