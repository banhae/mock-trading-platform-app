import { useEffect, useState } from 'react';
import { AUTH_CHANGE_EVENT, loadToken } from '../lib/token';

// 현재 탭의 access token 상태를 구독한다.
// - same-tab: saveToken/clearToken 이 dispatch 하는 AUTH_CHANGE_EVENT
// - cross-tab: browser 의 storage 이벤트
// - 다른 이유(예: 탭 복귀) 로 변경 가능성이 있을 때: focus 이벤트
export function useAuthToken(): { token: string | null; isAuthenticated: boolean } {
  const [token, setToken] = useState<string | null>(() => loadToken());

  useEffect(() => {
    function sync() {
      setToken(loadToken());
    }
    window.addEventListener(AUTH_CHANGE_EVENT, sync);
    window.addEventListener('storage', sync);
    window.addEventListener('focus', sync);
    return () => {
      window.removeEventListener(AUTH_CHANGE_EVENT, sync);
      window.removeEventListener('storage', sync);
      window.removeEventListener('focus', sync);
    };
  }, []);

  return { token, isAuthenticated: token !== null };
}
