# frontend

Exchange mock system의 SPA 프론트엔드다. Vite + React + TypeScript로 빌드하며, 컨테이너의 nginx는 정적 파일만 서빙한다.

- 포트: 컨테이너 80, Service 8084 (Helm chart `charts/frontend` 기준)
- 라우트: `/trade/:pair`, `/login`, `/orders`, `/health`
- API 호출: 브라우저는 same-origin `/api/*` 경로만 호출
- `/api/*` 라우팅: Kubernetes Ingress(`charts/mock-trading-platform-ingress`)가 path 기반으로 각 백엔드 서비스에 전달

## Public API path

- `POST   /api/auth/login` → auth-service `/login`
- `GET    /api/auth/health` → auth-service `/health`
- `GET    /api/auth/ready` → auth-service `/ready`
- `POST   /api/orders` → order-service `/orders`
- `GET    /api/orders` → order-service `/orders`
- `GET    /api/orders/{id}` → order-service `/orders/{id}`
- `DELETE /api/orders/{id}` → order-service `/orders/{id}`
- `GET    /api/orders/health` → order-service `/health`
- `GET    /api/orders/ready` → order-service `/ready`
- `GET    /api/market/ticker/{pair}` → marketdata-service `/marketdata/ticker/{pair}`
- `GET    /api/market/orderbook/{pair}` → marketdata-service `/marketdata/orderbook/{pair}`
- `GET    /api/market/candles/{pair}` → marketdata-service `/marketdata/candles/{pair}`
- `GET    /api/market/trades/{pair}` → marketdata-service `/marketdata/trades/{pair}`

## 로컬 개발

```bash
cd services/frontend
npm ci
npm test -- --run
npm run build
```

## Docker 빌드

```bash
docker build -t frontend services/frontend
```

## 헬스체크

nginx는 `/healthz` 엔드포인트를 200으로 응답하며, Helm chart readiness/liveness probe도 동일 경로를 사용한다.
