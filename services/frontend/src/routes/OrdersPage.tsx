import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  cancelOrder,
  createOrder,
  getOrder,
  listOrders,
  type Order,
  type OrderStatus,
} from '../api/orders';
import { OrderTicketForm } from '../components/orders/OrderTicketForm';
import {
  canCancelOrder,
  displayPair,
  OrdersTable,
} from '../components/orders/OrdersTable';
import { useOrderTicketForm } from '../hooks/useOrderTicketForm';
import { useAuthToken } from '../hooks/useAuthToken';
import { loadToken } from '../lib/token';
import { pushToast } from '../lib/toast';

// /orders 는 "내 주문" 관리 화면이다.
// - 메인 테이블: 내 주문 목록
// - 주문 생성 / 단건 조회: 보조 도구 섹션으로 유지

const STATUS_FILTERS: { value: '' | OrderStatus; label: string }[] = [
  { value: '', label: '전체' },
  { value: 'open', label: '대기' },
  { value: 'partially_filled', label: '부분체결' },
  { value: 'filled', label: '체결완료' },
  { value: 'cancelled', label: '취소' },
];

export default function OrdersPage() {
  const orderForm = useOrderTicketForm();
  const { isAuthenticated } = useAuthToken();

  const [orders, setOrders] = useState<Order[]>([]);
  const [statusFilter, setStatusFilter] = useState<'' | OrderStatus>('');
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState('');
  const [cancellingId, setCancellingId] = useState<string | null>(null);

  const [lookupId, setLookupId] = useState('');
  const [lookupResult, setLookupResult] = useState<Order | null>(null);
  const [lookupError, setLookupError] = useState('');

  const refresh = useCallback(async () => {
    const token = loadToken();
    if (!token) {
      setListError('주문을 보려면 로그인해 주세요.');
      setOrders([]);
      return;
    }
    setLoading(true);
    setListError('');
    try {
      const res = await listOrders(token, {
        status: statusFilter || undefined,
      });
      setOrders(res.orders ?? []);
    } catch (err) {
      setListError((err as Error).message);
    } finally {
      setLoading(false);
    }
    // isAuthenticated 가 바뀌면 (login / signout) 자동으로 refresh 를 재실행한다.
  }, [statusFilter, isAuthenticated]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function onCreate() {
    await orderForm.submit(async (payload) => {
      const token = loadToken();
      if (!token) {
        throw new Error('로그인이 필요합니다.');
      }
      const created = await createOrder(token, payload);
      await refresh();
      const sideLabel = created.side === 'sell' ? '매도' : '매수';
      pushToast({
        tone: 'success',
        message: `${sideLabel} 주문이 접수되었습니다. (${displayPair(created.pair)})`,
      });
    });
  }

  async function onCancel(id: string) {
    const order = orders.find((candidate) => candidate.id === id);
    if (!order || !canCancelOrder(order.status)) {
      return;
    }

    const token = loadToken();
    if (!token) {
      setListError('로그인이 필요합니다.');
      return;
    }
    setCancellingId(id);
    setListError('');
    try {
      await cancelOrder(token, id);
      await refresh();
      pushToast({ tone: 'info', message: '주문이 취소되었습니다.' });
    } catch (err) {
      const message = (err as Error).message;
      setListError(message);
      pushToast({ tone: 'error', message: `주문 취소에 실패했습니다. ${message}` });
    } finally {
      setCancellingId(null);
    }
  }

  async function onLookup(e: React.FormEvent) {
    e.preventDefault();
    setLookupError('');
    setLookupResult(null);
    const token = loadToken();
    if (!token) {
      setLookupError('로그인이 필요합니다.');
      return;
    }
    if (!lookupId) {
      setLookupError('주문 ID를 입력해주세요.');
      return;
    }
    try {
      const order = await getOrder(token, lookupId);
      setLookupResult(order);
    } catch (err) {
      setLookupError((err as Error).message);
    }
  }

  const sortedOrders = useMemo(
    () => [...orders].sort((a, b) => b.created_at.localeCompare(a.created_at)),
    [orders],
  );

  const openCount = orders.filter((o) => canCancelOrder(o.status)).length;
  const filledCount = orders.filter((o) => o.status === 'filled').length;

  return (
    <section className="orders-page">
      <header className="orders-page__header">
        <div>
          <h1 className="orders-page__title">내 주문</h1>
          <p className="text-muted text-sm orders-page__subtitle">
            내가 낸 주문의 상태를 확인하고 관리합니다. 새 주문은{' '}
            <a href="/trade/BTC-KRW" className="link-primary">
              거래 화면
            </a>
            에서 바로 낼 수 있습니다.
          </p>
        </div>
        <dl className="orders-page__metrics" aria-label="요약">
          <div className="orders-page__metric">
            <dt>미체결</dt>
            <dd>{openCount}</dd>
          </div>
          <div className="orders-page__metric">
            <dt>체결</dt>
            <dd>{filledCount}</dd>
          </div>
          <div className="orders-page__metric">
            <dt>전체</dt>
            <dd>{orders.length}</dd>
          </div>
        </dl>
      </header>

      <div className="orders-page__filters">
        <div role="tablist" aria-label="주문 상태" className="filter-tabs">
          {STATUS_FILTERS.map((opt) => (
            <button
              key={opt.value || 'all'}
              type="button"
              role="tab"
              aria-selected={statusFilter === opt.value}
              className="filter-tabs__item"
              onClick={() => setStatusFilter(opt.value)}
            >
              {opt.label}
            </button>
          ))}
        </div>
        <button
          type="button"
          className="btn btn--sm"
          onClick={() => void refresh()}
          disabled={loading}
        >
          {loading ? '새로고침 중…' : '새로고침'}
        </button>
      </div>

      <OrdersTable
        orders={sortedOrders}
        loading={loading}
        error={listError}
        onCancel={onCancel}
        cancellingId={cancellingId}
      />

      <details className="orders-page__dev">
        <summary className="orders-page__dev-summary">
          도구 · 새 주문 / ID로 조회
        </summary>

        <div className="orders-page__dev-grid">
          <OrderTicketForm
            values={orderForm.values}
            createError={orderForm.createError}
            submitting={orderForm.submitting}
            onChange={orderForm.setField}
            onSubmit={onCreate}
            isAuthenticated={isAuthenticated}
          />
          <div className="card">
            <div className="card__header">
              <h3 className="card__title">주문 ID로 조회</h3>
              <span className="card__subtitle">진단용</span>
            </div>
            <div className="card__body">
              <form onSubmit={onLookup} className="lookup-form">
                <div className="input-group lookup-form__field">
                  <label className="input-group__label" htmlFor="lookup-id">
                    주문 ID
                  </label>
                  <input
                    id="lookup-id"
                    className="input"
                    value={lookupId}
                    onChange={(e) => setLookupId(e.target.value)}
                    placeholder="예: ord-01HXYZ…"
                  />
                </div>
                <button type="submit" className="btn btn--primary">
                  조회
                </button>
              </form>
              {lookupError && (
                <p role="alert" className="alert lookup-form__error">
                  {lookupError}
                </p>
              )}
              {lookupResult && (
                <pre className="lookup-result">
                  {JSON.stringify(lookupResult, null, 2)}
                </pre>
              )}
            </div>
          </div>
        </div>
      </details>
    </section>
  );
}
