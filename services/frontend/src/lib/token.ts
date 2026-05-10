// Access token은 sessionStorage에 저장한다. 학습용 mock이므로
// 더 정교한 저장 방식은 이후 phase에서 다룬다.

const KEY = 'mock-trading-platform.access_token';

// same-tab 에서 saveToken/clearToken 이 일어났을 때 구독자에게 알린다.
// browser 의 `storage` 이벤트는 값이 바뀐 탭에는 발생하지 않기 때문에,
// 같은 탭 내 다른 컴포넌트의 인증 UI 가 즉시 동기화되지 않는 문제를 피하려면
// 이 커스텀 이벤트를 같이 사용해야 한다.
export const AUTH_CHANGE_EVENT = 'mock-trading-platform:auth-change';

function emitAuthChange(): void {
  if (typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new Event(AUTH_CHANGE_EVENT));
  } catch {
    // non-browser / older runtime 은 무시.
  }
}

export function saveToken(token: string): void {
  try {
    sessionStorage.setItem(KEY, token);
  } catch {
    // sessionStorage 접근 불가(SSR/비브라우저 환경) 시 무시.
  }
  emitAuthChange();
}

export function loadToken(): string | null {
  try {
    return sessionStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function clearToken(): void {
  try {
    sessionStorage.removeItem(KEY);
  } catch {
    // ignore
  }
  emitAuthChange();
}
