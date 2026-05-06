import { useEffect, useMemo, useRef } from 'react';
import {
  CandlestickSeries,
  ColorType,
  createChart,
  type Time,
  type IChartApi,
  type CandlestickData,
} from 'lightweight-charts';
import type { Candle } from '../../api/marketdata';

export type CandleInterval = '1m' | '5m' | '1h';

const INTERVALS: { value: CandleInterval; label: string }[] = [
  { value: '1m', label: '1분' },
  { value: '5m', label: '5분' },
  { value: '1h', label: '1시간' },
];

export function mapCandlesToSeriesData(candles: Candle[]): CandlestickData<Time>[] {
  return candles.reduce<CandlestickData<Time>[]>((rows, candle) => {
    const ms = Date.parse(candle.timestamp);
    if (Number.isNaN(ms)) {
      return rows;
    }

    rows.push({
      time: Math.floor(ms / 1000) as Time,
      open: Number(candle.open),
      high: Number(candle.high),
      low: Number(candle.low),
      close: Number(candle.close),
    });
    return rows;
  }, []);
}

export function CandleChart({
  candles,
  loading,
  error,
  interval,
  onIntervalChange,
}: {
  candles: Candle[];
  loading: boolean;
  error: string;
  interval: CandleInterval;
  onIntervalChange: (next: CandleInterval) => void;
}) {
  const chartContainerRef = useRef<HTMLDivElement | null>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const seriesRef = useRef<{ setData(data: CandlestickData<Time>[]): void } | null>(null);
  const seriesData = useMemo(() => mapCandlesToSeriesData(candles), [candles]);

  useEffect(() => {
    if (!chartContainerRef.current || chartRef.current) {
      return;
    }

    const chart = createChart(chartContainerRef.current, {
      layout: {
        background: { type: ColorType.Solid, color: '#ffffff' },
        textColor: '#4b5563',
      },
      width: chartContainerRef.current.clientWidth || 600,
      height: chartContainerRef.current.clientHeight || 420,
      grid: {
        vertLines: { color: '#eef0f4' },
        horzLines: { color: '#eef0f4' },
      },
      rightPriceScale: {
        borderColor: '#e5e7ec',
      },
      timeScale: {
        borderColor: '#e5e7ec',
        timeVisible: true,
      },
      autoSize: true,
    });

    const series = chart.addSeries(CandlestickSeries, {
      upColor: '#0a8a6a',
      downColor: '#d64545',
      borderVisible: false,
      wickUpColor: '#0a8a6a',
      wickDownColor: '#d64545',
    });

    chartRef.current = chart;
    seriesRef.current = series;

    return () => {
      chartRef.current?.remove();
      chartRef.current = null;
      seriesRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (seriesRef.current) {
      seriesRef.current.setData(seriesData);
    }
  }, [seriesData]);

  const hasData = candles.length > 0;

  return (
    <section className="card chart-card" aria-label="캔들 차트">
      <div className="chart-card__toolbar">
        <div className="toolbar">
          <h3 className="card__title">차트</h3>
          <span className="text-muted text-xs chart-card__interval-hint">
            · 캔들
          </span>
          {loading && !hasData && (
            <span className="text-muted text-xs">불러오는 중…</span>
          )}
        </div>
        <div role="tablist" aria-label="차트 주기" className="tabs">
          {INTERVALS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              role="tab"
              aria-selected={opt.value === interval}
              className="tabs__item"
              onClick={() => onIntervalChange(opt.value)}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>
      <div className="chart-card__body">
        {error && (
          <p role="alert" className="chart-card__placeholder text-sell">
            {error}
          </p>
        )}
        {!error && !hasData && !loading && (
          <p className="chart-card__placeholder">표시할 캔들 데이터가 없습니다.</p>
        )}
        {!error && (
          <div
            ref={chartContainerRef}
            data-testid="candle-chart"
            className="chart-card__canvas"
          />
        )}
      </div>
    </section>
  );
}
