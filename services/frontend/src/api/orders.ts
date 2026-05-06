import { request } from './client';

export interface CreateOrderRequest {
  pair: string;
  side: 'buy' | 'sell';
  quantity: string;
  price: string;
}

// 주문 status enum. backend 의 order-service/model.go 와 반드시 동기화한다.
export type OrderStatus =
  | 'open'
  | 'partially_filled'
  | 'filled'
  | 'cancelled';

export interface Order {
  id: string;
  user_id: string;
  pair: string;
  side: string;
  quantity: string;
  remaining_quantity: string;
  price: string;
  status: OrderStatus;
  created_at: string;
  updated_at: string;
}

export interface ListOrdersResponse {
  orders: Order[];
}

export interface ListOrdersOptions {
  status?: OrderStatus;
  limit?: number;
}

export function createOrder(token: string, body: CreateOrderRequest): Promise<Order> {
  return request<Order>('/api/orders', { method: 'POST', body, token });
}

export function getOrder(token: string, id: string): Promise<Order> {
  return request<Order>(`/api/orders/${encodeURIComponent(id)}`, { token });
}

// listOrders fetches the current user's orders. 서버가 JWT 에서 user 를
// 꺼내기 때문에 user query param 은 절대 붙이지 않는다.
export function listOrders(
  token: string,
  opts: ListOrdersOptions = {},
): Promise<ListOrdersResponse> {
  const params = new URLSearchParams();
  if (opts.status) params.set('status', opts.status);
  if (opts.limit !== undefined) params.set('limit', String(opts.limit));
  const qs = params.toString();
  const path = qs ? `/api/orders?${qs}` : '/api/orders';
  return request<ListOrdersResponse>(path, { token });
}

export function cancelOrder(token: string, id: string): Promise<Order> {
  return request<Order>(`/api/orders/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    token,
  });
}

export function ordersHealth(): Promise<unknown> {
  return request('/api/orders/health');
}

export function ordersReady(): Promise<unknown> {
  return request('/api/orders/ready');
}
