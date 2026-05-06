import type { TickerResponse } from '../../api/marketdata';
import {
  changeDirection,
  formatChangeRate,
  formatKRW,
} from '../../lib/format';
import { displayPair } from '../orders/OrdersTable';

interface PreviewRow {
  pair: string;
  displayName: string;
}

// 백엔드가 아직 BTC-KRW 만 지원하므로, 나머지는 명시적 preview 로 표시한다.
// 실제 API 호출은 하지 않는다.
const PREVIEW_MARKETS: PreviewRow[] = [
  { pair: 'ETH-KRW', displayName: 'Ethereum' },
  { pair: 'XRP-KRW', displayName: 'XRP' },
  { pair: 'SOL-KRW', displayName: 'Solana' },
  { pair: 'ADA-KRW', displayName: 'Cardano' },
  { pair: 'DOGE-KRW', displayName: 'Dogecoin' },
];

function btcName(pair: string): string {
  const base = pair.split('-')[0];
  return base === 'BTC' ? 'Bitcoin' : base ?? pair;
}

export function MarketSidebar({
  activePair,
  ticker,
}: {
  activePair: string;
  ticker: TickerResponse | null;
}) {
  const direction = changeDirection(ticker?.change_rate_24h);
  const changeTone = direction === 'up' ? 'text-buy' : direction === 'down' ? 'text-sell' : 'text-muted';

  return (
    <aside className="card market-sidebar" aria-label="마켓 목록">
      <div className="card__header">
        <h3 className="card__title">원화 마켓</h3>
        <span className="text-muted text-xs">KRW</span>
      </div>
      <div className="market-sidebar__hint">
        현재 <strong>BTC/KRW</strong>만 거래 가능합니다. 나머지 마켓은 준비 중입니다.
      </div>
      <ul className="market-sidebar__list">
        <li
          className={`market-sidebar__item${
            activePair === 'BTC-KRW' ? ' market-sidebar__item--active' : ''
          }`}
          aria-current={activePair === 'BTC-KRW' ? 'page' : undefined}
        >
          <div>
            <div className="market-sidebar__symbol">{displayPair('BTC-KRW')}</div>
            <div className="market-sidebar__sub">{btcName('BTC-KRW')}</div>
          </div>
          <div className="market-sidebar__price">
            {ticker ? formatKRW(ticker.last_price) : '—'}
          </div>
          <div className={`market-sidebar__change ${changeTone}`}>
            {formatChangeRate(ticker?.change_rate_24h)}
          </div>
        </li>
        {PREVIEW_MARKETS.map((row) => (
          <li
            key={row.pair}
            className="market-sidebar__item market-sidebar__item--disabled"
            aria-disabled="true"
            title="준비 중인 마켓입니다"
          >
            <div>
              <div className="market-sidebar__symbol">
                {displayPair(row.pair)}{' '}
                <span className="badge badge--cancelled market-sidebar__preview">
                  준비중
                </span>
              </div>
              <div className="market-sidebar__sub">{row.displayName}</div>
            </div>
            <div className="market-sidebar__price">—</div>
            <div className="market-sidebar__change text-muted">—</div>
          </li>
        ))}
      </ul>
    </aside>
  );
}
