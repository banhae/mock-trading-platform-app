import type { Trade } from '../../api/marketdata';
import { formatPrice, formatQuantity, formatTime } from '../../lib/format';

// 체결 side 판정: order-service 가 taker_order_id 를 채워주고 있고,
// maker 의 반대가 taker 이므로, 이 간단한 UI 에서는 taker 기준으로 방향을 표현하기 어렵다.
// 대신 직전 체결 대비 가격 변화로 up/down 을 결정한다. 정확한 trade side
// 이벤트가 들어오면 그때 갈아낀다.
function tradeDirection(trades: Trade[]): ('up' | 'down' | 'flat')[] {
  return trades.map((trade, idx) => {
    if (idx === trades.length - 1) return 'flat';
    const current = Number(trade.price);
    const older = Number(trades[idx + 1]!.price);
    if (!Number.isFinite(current) || !Number.isFinite(older)) return 'flat';
    if (current > older) return 'up';
    if (current < older) return 'down';
    return 'flat';
  });
}

export function TradeTape({
  trades,
  loading,
  error,
}: {
  trades: Trade[];
  loading: boolean;
  error: string;
}) {
  const directions = tradeDirection(trades);

  return (
    <section className="card trade-tape" aria-label="최근 체결">
      <div className="card__header">
        <h3 className="card__title">최근 체결</h3>
        {loading && trades.length > 0 && (
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

      {!error && trades.length === 0 && !loading && (
        <div className="card__body empty-state empty-state--compact">
          <p className="text-muted">아직 체결된 거래가 없습니다.</p>
        </div>
      )}

      {!error && trades.length > 0 && (
        <div className="panel-scroll">
          <table className="table table--compact">
            <thead>
              <tr>
                <th>시간</th>
                <th className="num">가격</th>
                <th className="num">수량</th>
              </tr>
            </thead>
            <tbody>
              {trades.map((trade, idx) => {
                const dir = directions[idx];
                const tone =
                  dir === 'up' ? 'text-buy' : dir === 'down' ? 'text-sell' : 'text-muted';
                return (
                  <tr key={trade.trade_id}>
                    <td className="text-muted num">{formatTime(trade.executed_at)}</td>
                    <td className={`num ${tone}`}>{formatPrice(trade.price)}</td>
                    <td className="num">{formatQuantity(trade.quantity)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
