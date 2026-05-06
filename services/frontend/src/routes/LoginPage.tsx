import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { login } from '../api/auth';
import { saveToken, clearToken } from '../lib/token';
import { useAuthToken } from '../hooks/useAuthToken';
import { pushToast } from '../lib/toast';

// mock 거래 계정 안내. 실제 백엔드가 mock 인증을 내려주는 값과 일치한다.
const DEMO_USERNAME = 'dev';
const DEMO_PASSWORD = 'dev';

export default function LoginPage() {
  const [username, setUsername] = useState(DEMO_USERNAME);
  const [password, setPassword] = useState(DEMO_PASSWORD);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const { isAuthenticated: hasToken } = useAuthToken();
  const navigate = useNavigate();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const res = await login(username, password);
      saveToken(res.access_token);
      pushToast({
        tone: 'success',
        message: `환영합니다, ${username}님. 거래 화면으로 이동합니다.`,
      });
      setTimeout(() => navigate('/trade/BTC-KRW'), 350);
    } catch (err) {
      const message = (err as Error).message;
      setError(`로그인에 실패했습니다. ${message}`);
      pushToast({ tone: 'error', message: '로그인에 실패했습니다. 계정을 확인해주세요.' });
    } finally {
      setSubmitting(false);
    }
  }

  function onLogout() {
    clearToken();
    pushToast({ tone: 'info', message: '로그아웃되었습니다.' });
  }

  return (
    <section className="login" aria-label="login">
      <header className="login__header">
        <h1 className="login__title">거래소에 로그인</h1>
        <p className="login__subtitle">
          mock 계정으로 주문 API에 접속합니다. 실제 자산은 거래되지 않습니다.
        </p>
      </header>

      <form className="login__form" onSubmit={onSubmit}>
        <div className="input-group">
          <label className="input-group__label" htmlFor="login-username">
            아이디
          </label>
          <input
            id="login-username"
            className="input input--plain"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            required
          />
        </div>
        <div className="input-group">
          <label className="input-group__label" htmlFor="login-password">
            비밀번호
          </label>
          <input
            id="login-password"
            className="input input--plain"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
        </div>
        <button
          type="submit"
          className="btn btn--primary btn--block"
          disabled={submitting}
        >
          {submitting ? '로그인 중…' : '로그인'}
        </button>
      </form>

      <div className="login__hint" aria-label="데모 계정 안내">
        <span className="login__hint-label">데모 계정</span>
        <code className="login__hint-value">
          {DEMO_USERNAME} / {DEMO_PASSWORD}
        </code>
      </div>

      <div className="login__status">
        <span>
          세션{' '}
          <span className={`badge ${hasToken ? 'badge--filled badge--dot' : 'badge--cancelled'}`}>
            {hasToken ? '연결됨' : '없음'}
          </span>
        </span>
        {hasToken ? (
          <button type="button" className="btn btn--sm btn--ghost" onClick={onLogout}>
            로그아웃
          </button>
        ) : (
          <Link to="/trade/BTC-KRW" className="link-primary text-sm">
            둘러보기 →
          </Link>
        )}
      </div>

      {error && (
        <p role="alert" className="alert login__message">
          {error}
        </p>
      )}
    </section>
  );
}
