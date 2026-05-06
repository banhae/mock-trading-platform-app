import { describe, expect, it } from 'vitest';
import { renderToString } from 'react-dom/server';
import {
  canCancelOrder,
  displayPair,
  getOrderStatusLabel,
  OrdersTable,
} from '../components/orders/OrdersTable';
import type { Order } from '../api/orders';

// OrdersTable 는 순수 프레젠테이션 컴포넌트이므로 react-dom/server 의
// renderToString 으로 DOM 없이 테스트한다. jsdom 같은 무거운 환경 의존성을
// 추가하지 않기 위한 선택이다.
function makeOrder(overrides: Partial<Order> = {}): Order {
  return {
    id: 'o1',
    user_id: 'alice',
    pair: 'BTC-KRW',
    side: 'buy',
    quantity: '0.5',
    remaining_quantity: '0.5',
    price: '50000',
    status: 'open',
    created_at: '2026-04-13T00:00:00Z',
    updated_at: '2026-04-13T00:30:00Z',
    ...overrides,
  };
}

describe('order presentation helpers', () => {
  it('converts canonical pair id to display symbol', () => {
    expect(displayPair('BTC-KRW')).toBe('BTC/KRW');
  });

  it('returns readable korean status labels', () => {
    expect(getOrderStatusLabel('open')).toBe('대기');
    expect(getOrderStatusLabel('partially_filled')).toBe('부분체결');
    expect(getOrderStatusLabel('filled')).toBe('체결완료');
    expect(getOrderStatusLabel('cancelled')).toBe('취소');
  });

  it('allows cancel only for open and partially filled', () => {
    expect(canCancelOrder('open')).toBe(true);
    expect(canCancelOrder('partially_filled')).toBe(true);
    expect(canCancelOrder('filled')).toBe(false);
    expect(canCancelOrder('cancelled')).toBe(false);
  });
});

describe('OrdersTable', () => {
  const noop = () => undefined;

  it('renders loading state', () => {
    const html = renderToString(
      <OrdersTable
        orders={[]}
        loading={true}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    expect(html).toContain('주문 목록을 불러오는 중');
  });

  it('renders error state', () => {
    const html = renderToString(
      <OrdersTable
        orders={[]}
        loading={false}
        error="boom"
        onCancel={noop}
        cancellingId={null}
      />,
    );
    expect(html).toContain('주문 목록을 불러오지 못했습니다');
    expect(html).toContain('boom');
  });

  it('renders empty state with onboarding CTA', () => {
    const html = renderToString(
      <OrdersTable
        orders={[]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    expect(html).toContain('아직 주문이 없습니다');
    expect(html).toContain('/trade/BTC-KRW');
  });

  it('renders an order row with status/remaining/created/updated and cancel button for open status', () => {
    const html = renderToString(
      <OrdersTable
        orders={[makeOrder({ status: 'open', pair: 'BTC-KRW' })]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    expect(html).toContain('BTC/KRW');
    expect(html).toContain('data-testid="order-row"');
    // 취소 버튼은 canCancelOrder(open) 일 때만 렌더된다.
    expect(html).toContain('>취소</button>');
    expect(html).toContain('0.5'); // remaining_quantity
    expect(html).toContain('대기');
    // KST 기준 deterministic 포맷. 2026-04-13 00:00 UTC = 2026-04-13 09:00 KST
    expect(html).toContain('2026-04-13 09:00:00');
    expect(html).toContain('2026-04-13 09:30:00');
  });

  it('renders cancel button for partially_filled orders', () => {
    const html = renderToString(
      <OrdersTable
        orders={[makeOrder({ status: 'partially_filled' })]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    expect(html).toContain('>취소</button>');
    expect(html).toContain('부분체결');
  });

  it('does NOT render cancel button for filled orders', () => {
    const html = renderToString(
      <OrdersTable
        orders={[makeOrder({ status: 'filled' })]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    expect(html).toContain('체결완료');
    expect(html).not.toContain('>취소</button>');
  });

  it('does NOT render cancel button for cancelled orders', () => {
    const html = renderToString(
      <OrdersTable
        orders={[makeOrder({ status: 'cancelled' })]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    // 취소 status 라벨 (badge) 은 보이지만 액션 버튼은 없어야 한다.
    expect(html).toContain('badge--cancelled');
    expect(html).not.toContain('>취소</button>');
  });

  it('disables the cancel button of the row currently being cancelled', () => {
    const html = renderToString(
      <OrdersTable
        orders={[makeOrder({ id: 'o1', status: 'open' })]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId="o1"
      />,
    );
    expect(html).toContain('취소 중…');
    expect(html).toContain('disabled');
  });

  it('renders multiple rows', () => {
    const html = renderToString(
      <OrdersTable
        orders={[
          makeOrder({ id: 'a', status: 'open' }),
          makeOrder({ id: 'b', status: 'filled' }),
          makeOrder({ id: 'c', status: 'cancelled' }),
        ]}
        loading={false}
        error=""
        onCancel={noop}
        cancellingId={null}
      />,
    );
    const matches = html.match(/data-testid="order-row"/g);
    expect(matches?.length).toBe(3);
  });
});
