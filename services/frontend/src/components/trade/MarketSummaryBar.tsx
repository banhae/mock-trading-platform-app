import type { TickerResponse } from '../../api/marketdata';
import { displayPair } from '../orders/OrdersTable';
import {
  changeDirection,
  formatChangeRate,
  formatKRW,
  formatQuantity,
  multiplyAsKRW,
} from '../../lib/format';

export function MarketSummaryBar({
  ticker,
  loading,
  error,
  pair,
}: {
  ticker: TickerResponse | null;
  loading: boolean;
  error: string;
  pair: string;
}) {
  const direction = changeDirection(ticker?.change_rate_24h);
  const changeClass =
    direction === 'up' ? 'text-buy' : direction === 'down' ? 'text-sell' : 'text-muted';
  const changeArrow = direction === 'up' ? '▲' : direction === 'down' ? '▼' : '–';

  const lastPrice = ticker?.last_price;
  const volume = ticker?.volume_24h;
  const quoteVolume = lastPrice && volume ? multiplyAsKRW(lastPrice, volume) : '-';

  return (
    <section className="summary-bar" aria-label="시세 요약">
      <div className="summary-bar__pair">
        <span className="summary-bar__pair-symbol">{displayPair(pair)}</span>
        <span className="summary-bar__pair-sub">원화 마켓 · KRW</span>
      </div>
      <div className="summary-bar__price">
        <span className={`summary-bar__price-value ${changeClass}`}>
          {ticker ? formatKRW(lastPrice) : '—'}
        </span>
        <span className={`summary-bar__price-change ${changeClass}`}>
          <span aria-hidden="true">{changeArrow}</span>{' '}
          {formatChangeRate(ticker?.change_rate_24h)}
          <span className="summary-bar__price-change-label"> (24h)</span>
        </span>
      </div>
      <div className="summary-bar__stats">
        <Stat label="24h 고가" value={formatKRW(ticker?.high_24h)} tone="up" />
        <Stat label="24h 저가" value={formatKRW(ticker?.low_24h)} tone="down" />
        <Stat label="24h 거래량" value={`${formatQuantity(ticker?.volume_24h)} BTC`} />
        <Stat label="24h 거래대금" value={`${quoteVolume} KRW`} />
      </div>
      <div className="summary-bar__status" aria-live="polite">
        {error ? (
          <span className="text-sell">데이터 로드 실패</span>
        ) : loading && !ticker ? (
          '불러오는 중…'
        ) : (
          <span className="summary-bar__pulse" aria-hidden="true">
            <span className="summary-bar__pulse-dot" /> 3초마다 갱신
          </span>
        )}
      </div>
    </section>
  );
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: 'up' | 'down';
}) {
  const toneClass = tone === 'up' ? 'text-buy' : tone === 'down' ? 'text-sell' : '';
  return (
    <div className="summary-bar__stat">
      <span className="summary-bar__stat-label">{label}</span>
      <span className={`summary-bar__stat-value ${toneClass}`.trim()}>{value}</span>
    </div>
  );
}
