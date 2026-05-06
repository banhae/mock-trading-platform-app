// ko-KR 기준 숫자 / 시간 포맷 유틸. 핵심 거래 로직에는 사용하지 않고,
// 화면 표시 단계에서만 사용한다.

const KRW_FORMAT = new Intl.NumberFormat('ko-KR', {
  maximumFractionDigits: 0,
});

const PRICE_FORMAT = new Intl.NumberFormat('ko-KR', {
  maximumFractionDigits: 4,
});

const QUANTITY_FORMAT = new Intl.NumberFormat('ko-KR', {
  maximumFractionDigits: 8,
});

const CHANGE_FORMAT = new Intl.NumberFormat('ko-KR', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

function toNumber(value: string | number | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : null;
  }
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

export function formatKRW(value: string | number | null | undefined, fallback = '-'): string {
  const n = toNumber(value);
  if (n === null) return fallback;
  return KRW_FORMAT.format(n);
}

export function formatPrice(value: string | number | null | undefined, fallback = '-'): string {
  const n = toNumber(value);
  if (n === null) return fallback;
  // KRW pair 의 경우 가격은 정수가 일반적.
  if (Math.abs(n) >= 100) {
    return KRW_FORMAT.format(Math.round(n));
  }
  return PRICE_FORMAT.format(n);
}

export function formatQuantity(value: string | number | null | undefined, fallback = '-'): string {
  const n = toNumber(value);
  if (n === null) return fallback;
  return QUANTITY_FORMAT.format(n);
}

// signed 는 "+" 를 항상 붙일지 여부.
export function formatChangeRate(value: string | number | null | undefined, signed = true): string {
  const n = toNumber(value);
  if (n === null) return '-';
  const body = CHANGE_FORMAT.format(Math.abs(n));
  if (!signed) return `${body}%`;
  if (n > 0) return `+${body}%`;
  if (n < 0) return `-${body}%`;
  return `${body}%`;
}

export function changeDirection(value: string | number | null | undefined): 'up' | 'down' | 'flat' {
  const n = toNumber(value);
  if (n === null || n === 0) return 'flat';
  return n > 0 ? 'up' : 'down';
}

// 현재가 × 수량 = 총 거래대금 (KRW pair 기준).
export function multiplyAsKRW(price: string, quantity: string): string {
  const p = Number(price);
  const q = Number(quantity);
  if (!Number.isFinite(p) || !Number.isFinite(q)) return '-';
  return KRW_FORMAT.format(Math.round(p * q));
}

// HH:MM:SS (UTC). trade tape 등에서 사용.
export function formatTime(value: string, fallback = '-'): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  return date.toISOString().slice(11, 19);
}

function pad2(n: number): string {
  return String(n).padStart(2, '0');
}

// 사용자 화면용 날짜/시간. 한국 거래소 맥락에 맞춰 KST 기준으로 표시하고,
// 테스트 환경에 관계없이 deterministic 하게 만든다. 형태는 "YYYY-MM-DD HH:MM:SS".
export function formatDateTime(value: string | null | undefined, fallback = '-'): string {
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  const kst = new Date(date.getTime() + 9 * 60 * 60 * 1000);
  const y = kst.getUTCFullYear();
  const m = pad2(kst.getUTCMonth() + 1);
  const d = pad2(kst.getUTCDate());
  const h = pad2(kst.getUTCHours());
  const mi = pad2(kst.getUTCMinutes());
  const s = pad2(kst.getUTCSeconds());
  return `${y}-${m}-${d} ${h}:${mi}:${s}`;
}
