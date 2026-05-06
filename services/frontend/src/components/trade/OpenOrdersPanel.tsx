import type { Order } from '../../api/orders';
import { canCancelOrder, displayPair, OrderStatusBadge, SideBadge } from '../orders/OrdersTable';
import { formatPrice, formatQuantity } from '../../lib/format';

export function OpenOrdersPanel({
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
  const openOrders = orders.filter((order) => canCancelOrder(order.status));

  return (
    <section className="card open-orders" aria-label="미체결 주문">
      <div className="card__header">
        <h3 className="card__title">
          미체결 주문{' '}
          <span className="text-muted card__title-count">
            ({openOrders.length}/{orders.length})
          </span>
        </h3>
        {loading && orders.length > 0 && (
          <span className="text-muted text-xs">갱신 중…</span>
        )}
      </div>

      {error && (
        <div className="card__body">
          <p role="alert" className="alert">
            {error}
          </p>
        </div>
      )}

      {!error && openOrders.length === 0 && !loading && (
        <div className="card__body empty-state empty-state--compact">
          <p className="text-muted">미체결 주문이 없습니다.</p>
        </div>
      )}

      {!error && openOrders.length > 0 && (
        <div className="panel-scroll">
          <table className="table table--compact">
            <thead>
              <tr>
                <th>마켓</th>
                <th>구분</th>
                <th className="num">가격</th>
                <th className="num">잔량</th>
                <th>상태</th>
                <th aria-label="액션"></th>
              </tr>
            </thead>
            <tbody>
              {openOrders.map((order) => (
                <tr key={order.id}>
                  <td>{displayPair(order.pair)}</td>
                  <td>
                    <SideBadge side={order.side} />
                  </td>
                  <td className="num">{formatPrice(order.price)}</td>
                  <td className="num">{formatQuantity(order.remaining_quantity)}</td>
                  <td>
                    <OrderStatusBadge status={order.status} />
                  </td>
                  <td>
                    <button
                      type="button"
                      className="btn btn--sm"
                      onClick={() => onCancel(order.id)}
                      disabled={cancellingId === order.id}
                    >
                      {cancellingId === order.id ? '취소 중…' : '취소'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
