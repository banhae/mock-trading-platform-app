import type { OrderBookLevel, OrderBookResponse } from '../../api/marketdata';
import { formatPrice, formatQuantity } from '../../lib/format';

type Side = 'bid' | 'ask';

function withCumulative(levels: OrderBookLevel[]) {
  let cumulative = 0;
  return levels.map((level) => {
    cumulative += Number(level.quantity || '0');
    return { level, cumulative };
  });
}

function peakCumulative(levels: OrderBookLevel[]): number {
  return levels.reduce((acc, lvl) => acc + Number(lvl.quantity || '0'), 0);
}

function spread(orderbook: OrderBookResponse | null) {
  if (!orderbook) return null;
  const bestBid = orderbook.bids?.[0];
  const bestAsk = orderbook.asks?.[0];
  if (!bestBid || !bestAsk) return null;
  const bid = Number(bestBid.price);
  const ask = Number(bestAsk.price);
  if (!Number.isFinite(bid) || !Number.isFinite(ask) || bid <= 0) return null;
  const diff = ask - bid;
  const pct = (diff / bid) * 100;
  return { diff, pct };
}

export function OrderBookPanel({
  orderbook,
  loading,
  error,
}: {
  orderbook: OrderBookResponse | null;
  loading: boolean;
  error: string;
}) {
  const bids = orderbook?.bids ?? [];
  const asks = orderbook?.asks ?? [];
  const maxTotal = Math.max(peakCumulative(bids), peakCumulative(asks), 1);
  const sp = spread(orderbook);

  const hasData = bids.length > 0 || asks.length > 0;

  return (
    <section className="card orderbook" aria-label="호가창">
      <div className="card__header">
        <h3 className="card__title">호가창</h3>
        {loading && hasData && (
          <span className="text-muted text-xs">갱신 중…</span>
        )}
      </div>

      {error && (
        <p role="alert" className="orderbook__empty text-sell">
          {error}
        </p>
      )}

      {!error && !hasData && !loading && (
        <p className="orderbook__empty">호가 데이터를 기다리는 중입니다.</p>
      )}

      {!error && hasData && (
        <>
          <div className="orderbook__header">
            <span>가격 (KRW)</span>
            <span>수량</span>
            <span>누적</span>
          </div>
          <div className="orderbook__side orderbook__side--asks" data-side="ask">
            {withCumulative(asks).slice(0, 12).map(({ level, cumulative }, idx) => (
              <Row
                key={`ask-${idx}`}
                side="ask"
                level={level}
                cumulative={cumulative}
                maxTotal={maxTotal}
              />
            ))}
          </div>
          {sp && (
            <div className="orderbook__spread">
              <span className="orderbook__spread-label">스프레드</span>
              {formatPrice(sp.diff)} ({sp.pct.toFixed(2)}%)
            </div>
          )}
          <div className="orderbook__side" data-side="bid">
            {withCumulative(bids).slice(0, 12).map(({ level, cumulative }, idx) => (
              <Row
                key={`bid-${idx}`}
                side="bid"
                level={level}
                cumulative={cumulative}
                maxTotal={maxTotal}
              />
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function Row({
  side,
  level,
  cumulative,
  maxTotal,
}: {
  side: Side;
  level: OrderBookLevel;
  cumulative: number;
  maxTotal: number;
}) {
  const width = `${Math.min((cumulative / maxTotal) * 100, 100)}%`;
  return (
    <div className={`orderbook__row orderbook__row--${side}`}>
      <span className="orderbook__depth" style={{ width }} aria-hidden="true" />
      <span className="orderbook__row-price">{formatPrice(level.price)}</span>
      <span>{formatQuantity(level.quantity)}</span>
      <span className="text-muted">{formatQuantity(cumulative)}</span>
    </div>
  );
}
