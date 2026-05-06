import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  cancelOrder,
  createOrder,
  listOrders,
  type Order,
} from '../api/orders';
import { HttpError } from '../api/client';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

const sampleOrder: Order = {
  id: 'abc',
  user_id: 'alice',
  pair: 'BTC-KRW',
  side: 'buy',
  quantity: '0.5',
  remaining_quantity: '0.5',
  price: '50000',
  status: 'open',
  created_at: '2026-04-13T00:00:00Z',
  updated_at: '2026-04-13T00:00:00Z',
};

describe('orders api', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('listOrders calls /api/orders with bearer token and no query by default', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ orders: [sampleOrder] }),
    );
    vi.stubGlobal('fetch', fetchMock);

    const res = await listOrders('t0k3n');
    expect(res.orders).toHaveLength(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/orders');
    expect(init.method).toBe('GET');
    expect(init.headers.Authorization).toBe('Bearer t0k3n');
  });

  it('listOrders passes status and limit as query string', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ orders: [] }));
    vi.stubGlobal('fetch', fetchMock);

    await listOrders('t0k3n', { status: 'open', limit: 10 });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/orders?status=open&limit=10');
  });

  it('listOrders does NOT send any user query param', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ orders: [] }));
    vi.stubGlobal('fetch', fetchMock);

    await listOrders('t0k3n', { status: 'open' });
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).not.toContain('user');
  });

  it('listOrders surfaces HttpError on non-2xx', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response('nope', { status: 500 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(listOrders('t0k3n')).rejects.toBeInstanceOf(HttpError);
  });

  it('cancelOrder issues DELETE to /api/orders/{id}', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ ...sampleOrder, status: 'cancelled' }));
    vi.stubGlobal('fetch', fetchMock);

    const res = await cancelOrder('t0k3n', 'abc');
    expect(res.status).toBe('cancelled');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/orders/abc');
    expect(init.method).toBe('DELETE');
    expect(init.headers.Authorization).toBe('Bearer t0k3n');
    // DELETE 는 body 를 보내지 않는다.
    expect(init.body).toBeUndefined();
  });

  it('cancelOrder url-encodes the id', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleOrder));
    vi.stubGlobal('fetch', fetchMock);
    await cancelOrder('t0k3n', 'weird id/with slash');
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/orders/weird%20id%2Fwith%20slash',
    );
  });

  it('cancelOrder surfaces HttpError on 409 conflict (already cancelled)', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response('conflict', { status: 409 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(cancelOrder('t0k3n', 'abc')).rejects.toBeInstanceOf(HttpError);
  });

  it('createOrder posts json body to /api/orders', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(sampleOrder, 201));
    vi.stubGlobal('fetch', fetchMock);

    await createOrder('t0k3n', {
      pair: 'BTC-KRW',
      side: 'buy',
      quantity: '0.5',
      price: '50000',
    });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/orders');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      pair: 'BTC-KRW',
      side: 'buy',
      quantity: '0.5',
      price: '50000',
    });
  });
});
