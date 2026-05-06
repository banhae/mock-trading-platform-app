import { afterEach, describe, expect, it, vi } from 'vitest';
import { request, HttpError } from '../api/client';

describe('api client', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('calls relative path and parses json', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const res = await request<{ ok: boolean }>('/api/auth/health');
    expect(res).toEqual({ ok: true });
    const call = fetchMock.mock.calls[0];
    expect(call[0]).toBe('/api/auth/health');
    expect(call[1].method).toBe('GET');
  });

  it('rejects non-slash path', async () => {
    await expect(request('api/orders')).rejects.toThrow(/same-origin/);
  });


  it('rejects non-api base path', async () => {
    await expect(request('/healthz')).rejects.toThrow(/must start with \/api/);
  });

  it('rejects scheme-relative path (//host/...)', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    await expect(request('//evil.example/steal')).rejects.toThrow(/same-origin/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('rejects backslash-normalized scheme-relative path (/\\host/...)', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    await expect(request('/\\evil.example/steal')).rejects.toThrow(/same-origin/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('sends body with json content-type and bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: 'abc' }), {
        status: 201,
        headers: { 'content-type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await request('/api/orders', {
      method: 'POST',
      body: { pair: 'BTC-USDT' },
      token: 't0k3n',
    });
    const init = fetchMock.mock.calls[0][1];
    expect(init.method).toBe('POST');
    expect(init.headers['Content-Type']).toBe('application/json');
    expect(init.headers['Authorization']).toBe('Bearer t0k3n');
    expect(JSON.parse(init.body)).toEqual({ pair: 'BTC-USDT' });
  });

  it('throws HttpError on non-2xx', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('nope', { status: 401, statusText: 'Unauthorized' }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(request('/api/auth/login', { method: 'POST', body: {} })).rejects.toBeInstanceOf(
      HttpError,
    );
  });
});
