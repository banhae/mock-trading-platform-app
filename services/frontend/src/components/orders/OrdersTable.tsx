import type { Order, OrderStatus } from '../../api/orders';
import { formatDateTime, formatPrice, formatQuantity } from '../../lib/format';

export function displayPair(pair: string): string {
  return pair.replace('-', '/');
}

export function getOrderStatusLabel(status: OrderStatus): string {
  switch (status) {
    case 'open':
      return '대기';
    case 'partially_filled':
      return '부분체결';
    case 'filled':
      return '체결완료';
    case 'cancelled':
      return '취소';
    default:
      return status;
  }
}

export function canCancelOrder(status: OrderStatus): boolean {
  return status === 'open' || status === 'partially_filled';
}

function statusBadgeModifier(status: OrderStatus): string {
  switch (status) {
    case 'open':
      return 'badge--open';
    case 'partially_filled':
      return 'badge--partial';
    case 'filled':
      return 'badge--filled';
    case 'cancelled':
      return 'badge--cancelled';
    default:
      return '';
  }
}

export function OrderStatusBadge({ status }: { status: OrderStatus }) {
  return (
    <span className={`badge badge--dot ${statusBadgeModifier(status)}`}>
      {getOrderStatusLabel(status)}
    </span>
  );
}

export function SideBadge({ side }: { side: string }) {
  const klass = side === 'sell' ? 'badge--sell' : 'badge--buy';
  const label = side === 'sell' ? '매도' : '매수';
  return <span className={`badge ${klass}`}>{label}</span>;
}

function OrderRow({
  order,
  onCancel,
  cancellingId,
}: {
  order: Order;
  onCancel: (id: string) => void;
  cancellingId: string | null;
}) {
  const cancellable = canCancelOrder(order.status);
  return (
    <tr data-testid="order-row">
      <td>{displayPair(order.pair)}</td>
      <td>
        <SideBadge side={order.side} />
      </td>
      <td className="num">{formatPrice(order.price)}</td>
      <td className="num">{formatQuantity(order.quantity)}</td>
      <td className="num">{order.remaining_quantity ? formatQuantity(order.remaining_quantity) : '-'}</td>
      <td>
        <OrderStatusBadge status={order.status} />
      </td>
      <td className="text-muted text-sm">{formatDateTime(order.created_at)}</td>
      <td className="text-muted text-sm">{formatDateTime(order.updated_at)}</td>
      <td>
        {cancellable ? (
          <button
            type="button"
            className="btn btn--sm"
            onClick={() => onCancel(order.id)}
            disabled={cancellingId === order.id}
          >
            {cancellingId === order.id ? '취소 중…' : '취소'}
          </button>
        ) : (
          <span className="text-muted">—</span>
        )}
      </td>
    </tr>
  );
}

export function OrdersTable({
  orders,
  loading,
  error,
  onCancel,
  cancellingId,
}: {
  orders: Order[];
  loading: boolean;
  error: string;
  onCancel: (id: string) => void;
  cancellingId: string | null;
}) {
  if (loading) {
    return (
      <div className="card card__body">
        <p className="text-muted">주문 목록을 불러오는 중…</p>
      </div>
    );
  }
  if (error) {
    return (
      <div className="card card__body">
        <p role="alert" className="alert">
          주문 목록을 불러오지 못했습니다. {error}
        </p>
      </div>
    );
  }
  if (orders.length === 0) {
    return (
      <div className="card card__body empty-state">
        <h3 className="empty-state__title">아직 주문이 없습니다</h3>
        <p className="text-muted">
          <a href="/trade/BTC-KRW" className="link-primary">
            거래 화면
          </a>
          에서 첫 주문을 내보세요.
        </p>
      </div>
    );
  }

  return (
    <div className="card">
      <table className="table">
        <thead>
          <tr>
            <th>마켓</th>
            <th>구분</th>
            <th className="num">가격</th>
            <th className="num">수량</th>
            <th className="num">잔량</th>
            <th>상태</th>
            <th>주문 시각</th>
            <th>업데이트</th>
            <th>액션</th>
          </tr>
        </thead>
        <tbody>
          {orders.map((order) => (
            <OrderRow
              key={order.id}
              order={order}
              onCancel={onCancel}
              cancellingId={cancellingId}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

// OpenOrdersPanel 은 /orders 페이지 상단의 요약만 보여준다.
// /trade 에서 쓰는 대형 테이블은 components/trade/OpenOrdersPanel.tsx 에 있다.
export function OpenOrdersPanel({ orders }: { orders: Order[] }) {
  const openOrders = orders.filter((order) => canCancelOrder(order.status));
  return (
    <p className="text-muted orders-summary-line">
      미체결 주문 (대기 + 부분체결): {openOrders.length} / {orders.length}
    </p>
  );
}
