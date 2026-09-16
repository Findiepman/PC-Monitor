import type { Action, AuditEntry, Meta } from './types';

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

export const SIGNED_OUT = 'dashd:signedout';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError("Can't reach dashd. Check your connection or the tunnel.", 0);
  }
  if (res.status === 204) return undefined as T;

  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/login' && path !== '/api/session') {
      window.dispatchEvent(new Event(SIGNED_OUT));
    }
    throw new ApiError(data.error ?? `Request failed with ${res.status}.`, res.status);
  }
  return data as T;
}

export const api = {
  session: () => request<{ user: string; expires: string }>('GET', '/api/session'),
  login: (username: string, password: string, code: string) =>
    request<{ user: string }>('POST', '/api/login', { username, password, code }),
  logout: () => request<void>('POST', '/api/logout'),
  meta: () => request<Meta>('GET', '/api/meta'),
  audit: () => request<AuditEntry[]>('GET', '/api/audit'),
  action: (unitId: string, action: Action) =>
    request<{ ok: true }>(
      'POST',
      `/api/units/${encodeURIComponent(unitId)}/${action}`,
    ),
};
