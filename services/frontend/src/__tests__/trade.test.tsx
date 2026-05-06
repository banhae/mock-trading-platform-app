import { describe, expect, it, vi } from 'vitest';
import { renderToString } from 'react-dom/server';
import { MarketSummaryBar } from '../components/trade/MarketSummaryBar';
import { OrderBookPanel } from '../components/trade/OrderBookPanel';
import { TradeTape } from '../components/trade/TradeTape';
import { OpenOrdersPanel } from '../components/trade/OpenOrdersPanel';
import { mapCandlesToSeriesData } from '../components/trade/CandleChart';
import type { Order } from '../api/orders';
import { isSupportedPair, runPostOrderMutationRefetch } from '../routes/TradePage';

vi.mock('lightweight-charts', () => ({
  CandlestickSeries: {},
  ColorType: { Solid: 'solid' },
  createChart: () => ({
    addSeries: () => ({ setData: () => undefined }),
    remove: () => undefined,
  }),
}));

function makeOrder(overrides: Partial<Order> = {}): Order {
  return {
    id: 'o1',
    user_id: 'alice',
    pair: 'BTC-KRW',
    side: 'buy',
    quantity: '1.0',
    remaining_quantity: '1.0',
    price: '50000',
    status: 'open',
    created_at: '2026-04-14T00:00:00Z',
    updated_at: '2026-04-14T00:00:00Z',
    ...overrides,
  };
}

describe('trade route pair support', () => {
  it('accepts BTC-KRW only', () => {
    expect(isSupportedPair('BTC-KRW')).toBe(true);
    expect(isSupportedPair('ETH-KRW')).toBe(false);
    expect(isSupportedPair(undefined)).toBe(false);
  });
});

describe('trade panels render states', () => {
  it('renders market summary values', () => {
    const html = renderToString(
      <MarketSummaryBar
        pair="BTC-KRW"
        loading={false}
        error=""
        ticker={{
          pair: 'BTC-KRW',
          last_price: '51000',
          change_rate_24h: '1.5',
          high_24h: '52000',
          low_24h: '49000',
          volume_24h: '10.1',
          as_of: '2026-04-14T00:00:00Z',
        }}
      />,
    );
    expect(html).toContain('BTC/KRW');
    // ko-KR 포맷으로 천단위 콤마가 붙어서 렌더된다.
    expect(html).toContain('51,000');
    // 등락률은 소수점 2자리 고정 포맷.
    expect(html).toContain('+1.50%');
  });

  it('renders empty states without crashing', () => {
    const bookHtml = renderToString(
      <OrderBookPanel orderbook={null} loading={false} error="" />,
    );
    expect(bookHtml).toContain('호가 데이터를 기다리는 중입니다.');

    const tradesHtml = renderToString(
      <TradeTape trades={[]} loading={false} error="" />,
    );
    expect(tradesHtml).toContain('아직 체결된 거래가 없습니다.');
  });

  it('reuses cancel gating for open orders panel', () => {
    const html = renderToString(
      <OpenOrdersPanel
        orders={[makeOrder({ status: 'open' }), makeOrder({ id: 'o2', status: 'filled' })]}
        loading={false}
        error=""
        onCancel={() => undefined}
        cancellingId={null}
      />,
    );
    const cancelCount = (html.match(/>취소<\/button>/g) || []).length;
    expect(cancelCount).toBe(1);
  });
});

describe('post-order mutation refetch', () => {
  it('triggers all refetchers after create/cancel flows', async () => {
    const a = vi.fn(async () => undefined);
    const b = vi.fn(async () => undefined);

    await runPostOrderMutationRefetch([a, b]);

    expect(a).toHaveBeenCalledTimes(1);
    expect(b).toHaveBeenCalledTimes(1);
  });
});

describe('candle chart boundary mapper', () => {
  it('maps candle strings to chart data and skips bad timestamps', () => {
    const mapped = mapCandlesToSeriesData([
      {
        timestamp: '2026-04-14T00:01:00Z',
        open: '100',
        high: '110',
        low: '90',
        close: '105',
        volume: '1.0',
      },
      {
        timestamp: 'not-a-date',
        open: '1',
        high: '1',
        low: '1',
        close: '1',
        volume: '1',
      },
    ]);

    expect(mapped).toHaveLength(1);
    expect(mapped[0].open).toBe(100);
    expect(mapped[0].close).toBe(105);
  });
});
