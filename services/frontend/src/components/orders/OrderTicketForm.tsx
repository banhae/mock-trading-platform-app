import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { CreateOrderRequest } from '../../api/orders';
import { formatKRW } from '../../lib/format';
import { displayPair } from './OrdersTable';

type OrderType = 'limit' | 'market';

const PERCENTS = [10, 25, 50, 100];
const DEFAULT_MAX_QTY = 1;

function computeTotal(price: string, quantity: string): string {
  const p = Number(price);
  const q = Number(quantity);
  if (!Number.isFinite(p) || !Number.isFinite(q)) return '-';
  return formatKRW(Math.round(p * q));
}

export function OrderTicketForm({
  values,
  createError,
  submitting,
  onChange,
  onSubmit,
  isAuthenticated = true,
  hidePair = false,
  compact = false,
}: {
  values: CreateOrderRequest;
  createError: string;
  submitting: boolean;
  onChange: (field: keyof CreateOrderRequest, value: string) => void;
  onSubmit: () => Promise<void>;
  isAuthenticated?: boolean;
  hidePair?: boolean;
  compact?: boolean;
}) {
  const [orderType, setOrderType] = useState<OrderType>('limit');

  const side = values.side === 'sell' ? 'sell' : 'buy';
  const sideLabel = side === 'buy' ? '매수' : '매도';
  const total = computeTotal(values.price, values.quantity);

  const marketPreview = orderType === 'market';
  const canSubmit = isAuthenticated && !submitting && !marketPreview;

  function setPercent(pct: number) {
    const q = (DEFAULT_MAX_QTY * pct) / 100;
    // 소수점 8자리까지만 유지.
    const rounded = Number(q.toFixed(8)).toString();
    onChange('quantity', rounded);
  }

  return (
    <section className={`card ticket${compact ? ' ticket--compact' : ''}`}>
      <div role="tablist" aria-label="Order side" className="tabs tabs--side">
        <button
          type="button"
          role="tab"
          data-side="buy"
          aria-selected={side === 'buy'}
          className="tabs__item"
          onClick={() => onChange('side', 'buy')}
        >
          매수
        </button>
        <button
          type="button"
          role="tab"
          data-side="sell"
          aria-selected={side === 'sell'}
          className="tabs__item"
          onClick={() => onChange('side', 'sell')}
        >
          매도
        </button>
      </div>

      <form
        className="ticket__body"
        onSubmit={(e) => {
          e.preventDefault();
          if (!canSubmit) return;
          void onSubmit();
        }}
      >
        <div className="ticket__meta">
          <div role="tablist" aria-label="Order type" className="tabs">
            <button
              type="button"
              role="tab"
              aria-selected={orderType === 'limit'}
              className="tabs__item"
              onClick={() => setOrderType('limit')}
            >
              지정가
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={orderType === 'market'}
              className="tabs__item"
              onClick={() => setOrderType('market')}
              title="시장가는 준비 중입니다"
            >
              시장가
            </button>
          </div>
          {!hidePair && (
            <span className="text-muted text-mono">
              {displayPair(values.pair)}
            </span>
          )}
        </div>

        {hidePair ? null : (
          <input
            type="hidden"
            value={values.pair}
            onChange={(e) => onChange('pair', e.target.value)}
          />
        )}

        <div className="input-group">
          <label className="input-group__label" htmlFor="ticket-price">
            가격 (KRW)
          </label>
          <div className="input-addon">
            <input
              id="ticket-price"
              className="input"
              value={values.price}
              onChange={(e) => onChange('price', e.target.value)}
              inputMode="decimal"
              disabled={marketPreview}
              aria-describedby="ticket-price-note"
            />
            <span className="input-addon__suffix">KRW</span>
          </div>
          {marketPreview && (
            <span id="ticket-price-note" className="text-muted text-xs">
              시장가는 준비 중입니다. 지금은 지정가만 지원합니다.
            </span>
          )}
        </div>

        <div className="input-group">
          <label className="input-group__label" htmlFor="ticket-qty">
            수량 (BTC)
          </label>
          <div className="input-addon">
            <input
              id="ticket-qty"
              className="input"
              value={values.quantity}
              onChange={(e) => onChange('quantity', e.target.value)}
              inputMode="decimal"
            />
            <span className="input-addon__suffix">BTC</span>
          </div>
          <div className="ticket__pct-row" role="group" aria-label="Quantity presets">
            {PERCENTS.map((pct) => (
              <button
                key={pct}
                type="button"
                className="ticket__pct"
                onClick={() => setPercent(pct)}
              >
                {pct}%
              </button>
            ))}
          </div>
        </div>

        <div className="ticket__total">
          <span className="text-muted">총액</span>
          <span className="ticket__total-value">{total} KRW</span>
        </div>

        {!isAuthenticated ? (
          <div className="ticket__login-cta">
            주문을 내려면 <Link to="/login">로그인</Link>이 필요합니다.
          </div>
        ) : (
          <button
            type="submit"
            className={`btn btn--block ${side === 'buy' ? 'btn--buy' : 'btn--sell'}`}
            disabled={!canSubmit}
          >
            {submitting ? '주문 전송 중…' : marketPreview ? '시장가는 준비 중입니다' : `${sideLabel} 주문`}
          </button>
        )}

        {createError && <p className="alert" role="alert">{createError}</p>}
      </form>
    </section>
  );
}
