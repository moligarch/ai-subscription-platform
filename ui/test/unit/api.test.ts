import { describe, it, expect, beforeEach, vi } from 'vitest';
import * as api from '../../src/lib/api';
import * as session from '../../src/lib/session';

describe('api wrapper', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.fetch = vi.fn();
    session.setApiKey('test-key');
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    session.clearApiKey();
  });

  it('sets Authorization header', async () => {
    (globalThis.fetch as any).mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ message: 'ok' })
    });

    // @ts-ignore
    const res = await api.get('/api/v1/stats');
    expect((globalThis.fetch as any).mock.calls.length).toBe(1);
    const fetchArgs = (globalThis.fetch as any).mock.calls[0];
    const headers = fetchArgs[1].headers;
    expect(headers.get('Authorization')).toBe('Bearer test-key');
  });

  it('throws ApiError on non-2xx', async () => {
    (globalThis.fetch as any).mockResolvedValue({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      text: async () => JSON.stringify({ error: 'bad token' })
    });

    await expect(api.get('/api/v1/users')).rejects.toMatchObject({ status: 401 });
  });
});
