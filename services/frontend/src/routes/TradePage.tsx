import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import {
  getCandles,
  getOrderBook,
  getTicker,
  getTrades,
  type Candle,
  type OrderBookResponse,
  type TickerResponse,
  type Trade,
} from '../api/marketdata';
import { cancelOrder, createOrder, listOrders, type Order } from '../api/orders';
import { CandleChart, type CandleInterval } from '../components/trade/CandleChart';
import { MarketSummaryBar } from '../components/trade/MarketSummaryBar';
import { OpenOrdersPanel } from '../components/trade/OpenOrdersPanel';
import { OrderBookPanel } from '../components/trade/OrderBookPanel';
import { TradeTape } from '../components/trade/TradeTape';
import { MarketSidebar } from '../components/trade/MarketSidebar';
import { OrderTicketForm } from '../components/orders/OrderTicketForm';
import { useOrderTicketForm } from '../hooks/useOrderTicketForm';
import { useAuthToken } from '../hooks/useAuthToken';
import { loadToken } from '../lib/token';
import { canCancelOrder, displayPair } from '../components/orders/OrdersTable';
import { pushToast } from '../lib/toast';

const SUPPORTED_PAIR = 'BTC-KRW';
const POLLING_MS = 3000;

interface PanelState<T> {
  data: T;
  loading: boolean;
  error: string;
}

export function isSupportedPair(pair?: string): boolean {
  return pair === SUPPORTED_PAIR;
}

export async function runPostOrderMutationRefetch(refetchers: Array<() => Promise<void>>) {
  await Promise.all(refetchers.map((fn) => fn()));
}

export default function TradePage() {
  const { pair } = useParams();
  const orderForm = useOrderTicketForm();
  const tickerRequestVersionRef = useRef(0);
  const orderbookRequestVersionRef = useRef(0);
  const tradesRequestVersionRef = useRef(0);
  const candlesRequestVersionRef = useRef(0);

  const [interval, setInterval_] = useState<CandleInterval>('1m');
  const { isAuthenticated } = useAuthToken();

  const [tickerState, setTickerState] = useState<PanelState<TickerResponse | null>>({
    data: null,
    loading: true,
    error: '',
  });
  const [orderbookState, setOrderbookState] = useState<PanelState<OrderBookResponse | null>>({
    data: null,
    loading: true,
    error: '',
  });
  const [tradesState, setTradesState] = useState<PanelState<Trade[]>>({
    data: [],
    loading: true,
    error: '',
  });
  const [candlesState, setCandlesState] = useState<PanelState<Candle[]>>({
    data: [],
    loading: true,
    error: '',
  });
  const [ordersState, setOrdersState] = useState<PanelState<Order[]>>({
    data: [],
    loading: true,
    error: '',
  });
  const [cancellingId, setCancellingId] = useState<string | null>(null);

  const supported = isSupportedPair(pair);

  const refreshTicker = useCallback(async () => {
    if (!pair || !supported) return;
    const version = ++tickerRequestVersionRef.current;
    setTickerState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const data = await getTicker(pair);
      if (version === tickerRequestVersionRef.current) {
        setTickerState({ data, loading: false, error: '' });
      }
    } catch (err) {
      if (version === tickerRequestVersionRef.current) {
        setTickerState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
      }
    }
  }, [pair, supported]);

  const refreshOrderBook = useCallback(async () => {
    if (!pair || !supported) return;
    const version = ++orderbookRequestVersionRef.current;
    setOrderbookState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const data = await getOrderBook(pair, 20);
      if (version === orderbookRequestVersionRef.current) {
        setOrderbookState({ data, loading: false, error: '' });
      }
    } catch (err) {
      if (version === orderbookRequestVersionRef.current) {
        setOrderbookState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
      }
    }
  }, [pair, supported]);

  const refreshTrades = useCallback(async () => {
    if (!pair || !supported) return;
    const version = ++tradesRequestVersionRef.current;
    setTradesState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const data = await getTrades(pair, 50);
      if (version === tradesRequestVersionRef.current) {
        setTradesState({ data: data.trades ?? [], loading: false, error: '' });
      }
    } catch (err) {
      if (version === tradesRequestVersionRef.current) {
        setTradesState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
      }
    }
  }, [pair, supported]);

  const refreshCandles = useCallback(async () => {
    if (!pair || !supported) return;
    const version = ++candlesRequestVersionRef.current;
    setCandlesState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const data = await getCandles(pair, interval, 200);
      if (version === candlesRequestVersionRef.current) {
        setCandlesState({ data: data.candles ?? [], loading: false, error: '' });
      }
    } catch (err) {
      if (version === candlesRequestVersionRef.current) {
        setCandlesState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
      }
    }
  }, [pair, supported, interval]);

  const refreshMyOrders = useCallback(async () => {
    const token = loadToken();
    if (!token) {
      setOrdersState({ data: [], loading: false, error: '' });
      return;
    }
    setOrdersState((prev) => ({ ...prev, loading: true, error: '' }));
    try {
      const data = await listOrders(token);
      setOrdersState({ data: data.orders ?? [], loading: false, error: '' });
    } catch (err) {
      setOrdersState((prev) => ({ ...prev, loading: false, error: (err as Error).message }));
    }
  }, []);

  const refetchMarketPanels = useCallback(async () => {
    await Promise.all([refreshTicker(), refreshOrderBook(), refreshTrades(), refreshCandles()]);
  }, [refreshCandles, refreshOrderBook, refreshTicker, refreshTrades]);

  useEffect(() => {
    if (!supported) return;
    void refetchMarketPanels();
    void refreshMyOrders();

    const timer = window.setInterval(() => {
      void refetchMarketPanels();
      void refreshMyOrders();
    }, POLLING_MS);

    return () => {
      window.clearInterval(timer);
    };
  }, [refetchMarketPanels, refreshMyOrders, supported]);

  async function onCreateOrder() {
    await orderForm.submit(async (payload) => {
      const token = loadToken();
      if (!token) {
        throw new Error('로그인이 필요합니다.');
      }
      const created = await createOrder(token, payload);
      await runPostOrderMutationRefetch([refreshMyOrders, refetchMarketPanels]);
      const sideLabel = created.side === 'sell' ? '매도' : '매수';
      pushToast({
        tone: 'success',
        message: `${sideLabel} 주문이 접수되었습니다. (${displayPair(created.pair)})`,
      });
    });
  }

  async function onCancelOrder(id: string) {
    const order = ordersState.data.find((candidate) => candidate.id === id);
    if (!order || !canCancelOrder(order.status)) {
      return;
    }

    const token = loadToken();
    if (!token) {
      setOrdersState((prev) => ({ ...prev, error: '로그인이 필요합니다.' }));
      return;
    }

    setCancellingId(id);
    try {
      await cancelOrder(token, id);
      await runPostOrderMutationRefetch([refreshMyOrders, refetchMarketPanels]);
      pushToast({ tone: 'info', message: '주문이 취소되었습니다.' });
    } catch (err) {
      const message = (err as Error).message;
      setOrdersState((prev) => ({ ...prev, error: message }));
      pushToast({ tone: 'error', message: `주문 취소에 실패했습니다. ${message}` });
    } finally {
      setCancellingId(null);
    }
  }

  const sortedTrades = useMemo(
    () => [...tradesState.data].sort((a, b) => b.executed_at.localeCompare(a.executed_at)),
    [tradesState.data],
  );

  if (!supported) {
    return (
      <section className="unsupported">
        <div className="card">
          <div className="card__body unsupported__body">
            <h2 className="unsupported__title">지원하지 않는 마켓입니다</h2>
            <p className="text-muted unsupported__desc">
              <code>{pair}</code> 마켓은 아직 열려 있지 않습니다. 현재는{' '}
              <strong>BTC/KRW</strong> 마켓만 거래할 수 있습니다.
            </p>
            <a href="/trade/BTC-KRW" className="btn btn--primary">
              BTC/KRW 거래 화면으로 이동
            </a>
          </div>
        </div>
      </section>
    );
  }

  return (
    <div className="trade-layout">
      <div className="trade-main">
        <MarketSummaryBar
          ticker={tickerState.data}
          loading={tickerState.loading}
          error={tickerState.error}
          pair={SUPPORTED_PAIR}
        />

        <div className="trade-main__row">
          <CandleChart
            candles={candlesState.data}
            loading={candlesState.loading}
            error={candlesState.error}
            interval={interval}
            onIntervalChange={(next) => setInterval_(next)}
          />
          <OrderBookPanel
            orderbook={orderbookState.data}
            loading={orderbookState.loading}
            error={orderbookState.error}
          />
        </div>

        <div className="trade-main__row trade-main__row--bottom">
          <OpenOrdersPanel
            orders={ordersState.data}
            loading={ordersState.loading}
            error={ordersState.error}
            onCancel={onCancelOrder}
            cancellingId={cancellingId}
          />
          <TradeTape
            trades={sortedTrades}
            loading={tradesState.loading}
            error={tradesState.error}
          />
        </div>
      </div>

      <aside className="trade-aside">
        <OrderTicketForm
          values={orderForm.values}
          createError={orderForm.createError}
          submitting={orderForm.submitting}
          onChange={orderForm.setField}
          onSubmit={onCreateOrder}
          isAuthenticated={isAuthenticated}
          hidePair
        />
        <MarketSidebar activePair={SUPPORTED_PAIR} ticker={tickerState.data} />
      </aside>
    </div>
  );
}
