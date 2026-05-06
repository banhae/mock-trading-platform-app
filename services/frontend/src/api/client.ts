// same-origin + ingress 경로 기반 fetch wrapper.
// 프론트는 항상 '/api/...' 상대 경로만 호출하고, ingress 가 각 서비스로 라우팅한다.
// (auth/orders/wallet/market)

export type HttpMethod = 'GET' | 'POST' | 'DELETE';

export interface RequestOptions {
  method?: HttpMethod;
  body?: unknown;
  token?: string;
}

export class HttpError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export const API_BASE = '/api';

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  // 반드시 same-origin 절대 경로 + ingress API base 여야 한다.
  // - scheme-relative URL(`//host/...`) 은 브라우저가 cross-origin 으로 해석해서
  //   Authorization 헤더가 타 origin 으로 유출될 수 있다.
  // - `/\host/...` 도 일부 브라우저 URL 파서에서 scheme-relative 로 정규화된다.
  if (
    !path.startsWith('/') ||
    path.startsWith('//') ||
    path.startsWith('/\\')
  ) {
    throw new Error(`path must be same-origin absolute path: ${path}`);
  }
  if (!path.startsWith(`${API_BASE}/`)) {
    throw new Error(`path must start with ${API_BASE}/: ${path}`);
  }
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  if (opts.token) {
    headers['Authorization'] = `Bearer ${opts.token}`;
  }
  const res = await fetch(path, {
    method: opts.method ?? 'GET',
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new HttpError(res.status, text || res.statusText);
  }
  // 일부 health 엔드포인트는 바디가 비어있을 수 있다.
  const ct = res.headers.get('content-type') ?? '';
  if (ct.includes('application/json')) {
    return (await res.json()) as T;
  }
  return (await res.text()) as unknown as T;
}
