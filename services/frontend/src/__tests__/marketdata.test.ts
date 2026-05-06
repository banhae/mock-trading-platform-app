import { afterEach, describe, expect, it, vi } from 'vitest';
import { getCandles, getOrderBook, getTicker, getTrades } from '../api/marketdata';

describe('marketdata api client', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('uses /api/market base for ticker', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ pair: 'BTC-KRW' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    await getTicker('BTC-KRW');

    expect(fetchMock.mock.calls[0][0]).toBe('/api/market/ticker/BTC-KRW');
  });

  it('uses /api/market base for orderbook/candles/trades', async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({ pair: 'BTC-KRW', trades: [], candles: [], bids: [], asks: [] }),
          {
            status: 200,
            headers: { 'content-type': 'application/json' },
          },
        ),
      ),
    );
    vi.stubGlobal('fetch', fetchMock);

    await getOrderBook('BTC-KRW', 10);
    await getCandles('BTC-KRW', '1m', 20);
    await getTrades('BTC-KRW', 30);

    expect(fetchMock.mock.calls[0][0]).toBe('/api/market/orderbook/BTC-KRW?depth=10');
    expect(fetchMock.mock.calls[1][0]).toBe('/api/market/candles/BTC-KRW?interval=1m&limit=20');
    expect(fetchMock.mock.calls[2][0]).toBe('/api/market/trades/BTC-KRW?limit=30');
  });
});
