import { NavLink, Link } from 'react-router-dom';
import { clearToken } from '../../lib/token';
import { useAuthToken } from '../../hooks/useAuthToken';
import { pushToast } from '../../lib/toast';

// Header 는 상단 네비게이션과 로그인/유틸 영역을 담는다.
// Health 는 primary nav 에서 빼고 diagnostics utility 로 둔다.
export function Header() {
  const { isAuthenticated } = useAuthToken();

  function onSignOut() {
    clearToken();
    pushToast({ tone: 'info', message: '로그아웃되었습니다.' });
  }

  return (
    <header className="app-header">
      <Link to="/trade/BTC-KRW" className="app-header__brand" aria-label="Mock Trading Platform 홈">
        <span className="app-header__brand-mark">MT</span>
        <span className="app-header__brand-text">
          Mock Trading<span className="app-header__brand-accent">Platform</span>
        </span>
      </Link>
      <nav className="app-header__nav" aria-label="Primary">
        <NavLink
          to="/trade/BTC-KRW"
          className={({ isActive }) =>
            `app-header__nav-link${isActive ? ' app-header__nav-link--active' : ''}`
          }
        >
          거래
        </NavLink>
        <NavLink
          to="/orders"
          className={({ isActive }) =>
            `app-header__nav-link${isActive ? ' app-header__nav-link--active' : ''}`
          }
        >
          내 주문
        </NavLink>
      </nav>
      <div className="app-header__utility">
        <NavLink
          to="/health"
          className={({ isActive }) =>
            `app-header__diag${isActive ? ' app-header__diag--active' : ''}`
          }
          title="서비스 진단"
        >
          서비스 상태
        </NavLink>
        {isAuthenticated ? (
          <button type="button" className="btn btn--sm" onClick={onSignOut}>
            로그아웃
          </button>
        ) : (
          <NavLink to="/login" className="btn btn--sm btn--primary">
            로그인
          </NavLink>
        )}
      </div>
    </header>
  );
}
