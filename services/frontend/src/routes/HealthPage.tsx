import { useCallback, useEffect, useState } from 'react';
import { authHealth, authReady } from '../api/auth';
import { ordersHealth, ordersReady } from '../api/orders';

type Status = 'idle' | 'ok' | 'error';

interface ProbeState {
  status: Status;
  detail: string;
  checkedAt: number | null;
}

const initial: ProbeState = { status: 'idle', detail: '', checkedAt: null };

interface ProbeMeta {
  name: string;
  endpoint: string;
  fn: () => Promise<unknown>;
}

const PROBES: ProbeMeta[] = [
  { name: 'auth-service', endpoint: '/api/auth/health', fn: authHealth },
  { name: 'auth-service', endpoint: '/api/auth/ready', fn: authReady },
  { name: 'order-service', endpoint: '/api/orders/health', fn: ordersHealth },
  { name: 'order-service', endpoint: '/api/orders/ready', fn: ordersReady },
];

function timeAgo(epochMs: number | null): string {
  if (!epochMs) return '—';
  const secs = Math.max(0, Math.round((Date.now() - epochMs) / 1000));
  if (secs < 60) return `${secs}s 전`;
  return `${Math.round(secs / 60)}m 전`;
}

export default function HealthPage() {
  const [frontend] = useState<ProbeState>({
    status: 'ok',
    detail: 'SPA loaded',
    checkedAt: Date.now(),
  });
  const [states, setStates] = useState<ProbeState[]>(() => PROBES.map(() => initial));
  const [refreshing, setRefreshing] = useState(false);

  const runAll = useCallback(async () => {
    setRefreshing(true);
    const results = await Promise.all(
      PROBES.map(async (probe) => {
        try {
          const res = await probe.fn();
          return {
            status: 'ok' as Status,
            detail: JSON.stringify(res),
            checkedAt: Date.now(),
          };
        } catch (err) {
          return {
            status: 'error' as Status,
            detail: (err as Error).message,
            checkedAt: Date.now(),
          };
        }
      }),
    );
    setStates(results);
    setRefreshing(false);
  }, []);

  useEffect(() => {
    void runAll();
  }, [runAll]);

  const overallStatus: Status = states.every((s) => s.status === 'ok') && frontend.status === 'ok'
    ? 'ok'
    : states.some((s) => s.status === 'error')
      ? 'error'
      : 'idle';

  const cards = [
    {
      name: 'frontend',
      endpoint: 'SPA bundle',
      state: frontend,
    },
    ...PROBES.map((probe, idx) => ({
      name: probe.name,
      endpoint: probe.endpoint,
      state: states[idx]!,
    })),
  ];

  return (
    <section className="health-page">
      <header className="health-page__header">
        <div>
          <h1 className="health-page__title">서비스 상태</h1>
          <p className="health-page__subtitle">
            각 백엔드 서비스의 health / readiness probe 결과입니다. 운영 메트릭은
            내부 Grafana에서 확인하세요.
          </p>
        </div>
        <div className="health-page__meta">
          <OverallPill status={overallStatus} />
          <button
            type="button"
            className="btn btn--sm"
            onClick={() => void runAll()}
            disabled={refreshing}
          >
            {refreshing ? '확인 중…' : '다시 확인'}
          </button>
        </div>
      </header>
      <div className="health">
        {cards.map((card, idx) => (
          <div key={idx} className="card health__card">
            <div className="health__card-row">
              <strong>{card.name}</strong>
              <StatusPill status={card.state.status} />
            </div>
            <code className="health__endpoint">{card.endpoint}</code>
            <pre className="health__detail">{card.state.detail || '—'}</pre>
            <span className="health__timestamp">마지막 확인 {timeAgo(card.state.checkedAt)}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

function StatusPill({ status }: { status: Status }) {
  if (status === 'ok') {
    return <span className="badge badge--filled badge--dot">정상</span>;
  }
  if (status === 'error') {
    return <span className="badge badge--sell badge--dot">장애</span>;
  }
  return <span className="badge badge--cancelled badge--dot">대기</span>;
}

function OverallPill({ status }: { status: Status }) {
  if (status === 'ok') {
    return <span className="badge badge--filled badge--dot">모든 서비스 정상</span>;
  }
  if (status === 'error') {
    return <span className="badge badge--sell badge--dot">일부 서비스 장애</span>;
  }
  return <span className="badge badge--cancelled badge--dot">상태 확인 중</span>;
}
