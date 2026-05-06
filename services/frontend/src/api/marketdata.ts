import { HttpError, request } from './client';

export interface TickerResponse {
  pair: string;
  last_price: string;
  change_rate_24h: string;
  high_24h: string;
  low_24h: string;
  volume_24h: string;
  as_of: string;
}

export interface OrderBookLevel {
  price: string;
  quantity: string;
}

export interface OrderBookResponse {
  pair: string;
  depth: number;
  bids: OrderBookLevel[];
  asks: OrderBookLevel[];
  as_of: string;
}

export interface Candle {
  timestamp: string;
  open: string;
  high: string;
  low: string;
  close: string;
  volume: string;
}

export interface CandlesResponse {
  pair: string;
  interval: '1m' | '5m' | '1h' | string;
  candles: Candle[];
}

export interface Trade {
  trade_id: string;
  pair: string;
  price: string;
  quantity: string;
  maker_order_id: string;
  taker_order_id: string;
  executed_at: string;
}

export interface TradesResponse {
  pair: string;
  trades: Trade[];
}

function toFriendlyError(err: unknown, domain: string): Error {
  if (err instanceof HttpError) {
    if (err.status === 400) {
      return new Error(`invalid ${domain} request. please verify pair and query parameters.`);
    }
    if (err.status >= 500) {
      return new Error(`${domain} service is temporarily unavailable.`);
    }
    return new Error(`failed to load ${domain}: ${err.message || `http ${err.status}`}`);
  }
  return new Error(`failed to load ${domain}: ${(err as Error).message}`);
}

export async function getTicker(pair: string): Promise<TickerResponse> {
  try {
    return await request<TickerResponse>(`/api/market/ticker/${encodeURIComponent(pair)}`);
  } catch (err) {
    throw toFriendlyError(err, 'ticker');
  }
}

export async function getOrderBook(pair: string, depth = 20): Promise<OrderBookResponse> {
  try {
    return await request<OrderBookResponse>(
      `/api/market/orderbook/${encodeURIComponent(pair)}?depth=${depth}`,
    );
  } catch (err) {
    throw toFriendlyError(err, 'orderbook');
  }
}

export async function getCandles(
  pair: string,
  interval: '1m' | '5m' | '1h' = '1m',
  limit = 200,
): Promise<CandlesResponse> {
  try {
    return await request<CandlesResponse>(
      `/api/market/candles/${encodeURIComponent(pair)}?interval=${interval}&limit=${limit}`,
    );
  } catch (err) {
    throw toFriendlyError(err, 'candles');
  }
}

export async function getTrades(pair: string, limit = 50): Promise<TradesResponse> {
  try {
    return await request<TradesResponse>(
      `/api/market/trades/${encodeURIComponent(pair)}?limit=${limit}`,
    );
  } catch (err) {
    throw toFriendlyError(err, 'trades');
  }
}
