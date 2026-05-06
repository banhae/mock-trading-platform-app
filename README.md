# mock-trading-platform-app

코인 거래소 mock 시스템의 애플리케이션 리포지토리.
AWS EKS 위에서 Kubernetes + Helm + GitOps 패턴을 학습하기 위한 mock 서비스들을 관리한다.

이 리포는 3개 리포 중 하나이며, 애플리케이션 코드와 배포 설정을 담당한다.

| 리포 | 책임 |
|------|------|
| **mock-trading-platform-app** (이 리포) | 서비스 코드, Dockerfile, Helm chart, CI |
| mock-trading-platform-infra | Terraform, AWS 인프라 (VPC, EKS, ECR, IAM) |
| mock-trading-platform-gitops | ArgoCD Application, 환경별 values |

## 디렉터리 구조

```
mock-trading-platform-app/
├── services/
│   ├── order-service/          # 주문 생성/조회 API (핵심 서비스)
│   ├── auth-service/           # Mock 로그인, JWT 발급
│   ├── wallet-service/         # 잔고 관리 mock (stub)
│   ├── marketdata-service/     # 시세 데이터 mock (stub)
│   └── frontend/               # Vite+React SPA + nginx static serving
├── charts/
│   ├── order-service/          # order-service Helm chart
│   ├── auth-service/           # auth-service Helm chart
│   ├── wallet-service/         # wallet-service Helm chart
│   ├── marketdata-service/     # marketdata-service Helm chart
│   ├── frontend/               # frontend Helm chart
│   └── mock-trading-platform-ingress/       # 단일 ALB Ingress (path-based routing)
└── .github/
    └── workflows/
        └── ci.yaml             # CI: test, docker build, ECR push, chart lint
```

## Archived plans

다음 문서는 과거 phase 계획 문서로, 현재는 `docs/archive/` 아래에 보관한다.

- [FRONTEND_PLAN.md](docs/archive/FRONTEND_PLAN.md)
- [MATCHER_PLAN.md](docs/archive/MATCHER_PLAN.md)
- [PHASE_EXECUTION_PLAN.md](docs/archive/PHASE_EXECUTION_PLAN.md)
- [TRADING_UI_PLAN.md](docs/archive/TRADING_UI_PLAN.md)

## 서비스

### order-service

주문 생성, 조회, 목록, 취소 API를 제공하는 핵심 서비스. PostgreSQL에 주문을 저장한다.
주문은 `open` / `partially_filled` / `filled` / `cancelled` 의 lifecycle status 를
가진다.

현재 `POST /orders` 는 order-service 내부 matcher runtime 경로로 연결되어,
요청 시점에 즉시 book 반영/매칭/상태 전이(`open`/`partially_filled`/`filled`)와
trade persistence 를 수행한다. `DELETE /orders/{id}` 는 cancellable 상태
(`open`, `partially_filled`)에서만 성공하며 book 상태와 함께 갱신된다.
matcher runtime 특성상 현재는 단일 active replica(수평 확장 미지원) 운영을 전제로 한다.

코드베이스는 인메모리 matcher core + match 결과 영속화(`PersistMatchResult`) +
`trades` 테이블 + 재기동 시 open / partially_filled 주문 기반 book rebuild 유틸을
서비스 시작 경로에서 함께 사용한다.

- 포트: 8080
- API: `POST /orders`, `GET /orders?status=&limit=`, `GET /orders/{id}`,
  `DELETE /orders/{id}`, `GET /health`, `GET /ready`
- 목록과 단건 조회, 취소 모두 현재 로그인 사용자 기준으로 scoping 된다.
- 상세: [services/order-service/README.md](services/order-service/README.md)

### auth-service

Mock 유저 로그인 및 JWT 발급 서비스. DB 없이 하드코딩된 유저로 동작한다.

- 포트: 8081
- API: `POST /login`, `GET /health`, `GET /ready`
- 상세: [services/auth-service/README.md](services/auth-service/README.md)

### wallet-service

잔고 관리를 위한 mock 서비스. health/ready 엔드포인트 외에, Phase 2 에서 NATS
JetStream 기반 **order.* 이벤트 consumer wiring** 이 추가되었다. 현재는 수신한
이벤트를 구조화된 로그로 남기고 in-memory 카운터만 증가시킨다. 실제 잔고
hold/release 로직은 이후 phase 에서 도입된다.

- 포트: 8082
- API: `GET /health`, `GET /ready`
- 상세: [services/wallet-service/README.md](services/wallet-service/README.md)

### marketdata-service

시세 데이터를 위한 mock 서비스. NATS JetStream 기반 `order.created`,
`order.updated`, `trade.executed` 이벤트를 consume 해서 in-memory read model
(ticker / orderbook / candles / recent trades)을 유지하고 REST API를 제공한다.
event bus가 비활성(`EVENT_BUS_ENABLED=false`)이어도 서비스는 부팅되며, 이 경우
marketdata endpoint는 empty state JSON을 정상 응답한다.

- 포트: 8083
- API (서비스 직접 호출용): `GET /health`, `GET /ready`,
  `GET /marketdata/{ticker,orderbook,candles,trades}/{pair}`
- API (ingress 경로 — `mock-trading-platform-ingress` chart의 `/api/market` prefix가 path rewrite 없이
  그대로 전달됨): `GET /api/market/{ticker,orderbook,candles,trades}/{pair}`,
  `GET /api/market/{health,ready}` 도 동일 핸들러로 직접 라우팅
- 상세: [services/marketdata-service/README.md](services/marketdata-service/README.md)

### frontend

Vite + React + TypeScript SPA. frontend nginx는 정적 파일만 서빙한다.
`/api/*` 경로 라우팅은 단일 Kubernetes Ingress가 auth/order/wallet/marketdata로 분기한다.
브라우저는 frontend origin 기반 same-origin(`/api`)으로만 backend API를 호출한다.

- 포트: Service 8084 / 컨테이너 80
- Public API: `POST /api/auth/login`, `POST /api/orders`, `GET /api/orders`,
  `GET /api/orders/{id}`, `DELETE /api/orders/{id}`,
  `GET /api/auth/{health,ready}`, `GET /api/orders/{health,ready}`,
  `GET /api/market/{ticker,orderbook,candles,trades}/{pair}`
- `/orders` 페이지는 "My orders" 화면으로, 현재 사용자의 주문 목록과 취소 기능을
  제공한다. 본격적인 trading surface (`/trade/:pair`) 는 Phase 4.5 에서 도입된다.
- 상세: [services/frontend/README.md](services/frontend/README.md)

### JWT 연동

두 서비스는 동일한 `JWT_SECRET` 환경변수를 공유한다. auth-service가 발급한 JWT를 order-service가 검증한다.
연동 방법은 [auth-service README](services/auth-service/README.md#order-service와의-jwt-연동)를 참고.

## Helm Charts

각 서비스의 Helm chart는 `charts/` 아래에 위치한다.

지원 항목: image (repository, tag), replicaCount, env, resources, service, probes.

```bash
# 배포 예시
helm install order-service ./charts/order-service \
  --set image.repository=<ECR_URL>/mock-trading-platform/order-service \
  --set image.tag=<TAG> \
  --set env.DATABASE_URL="postgres://..." \
  --set env.DEV_MODE="true"

helm install auth-service ./charts/auth-service \
  --set image.repository=<ECR_URL>/mock-trading-platform/auth-service \
  --set image.tag=<TAG> \
  --set env.DEV_MODE="true"

helm install wallet-service ./charts/wallet-service \
  --set image.repository=<ECR_URL>/mock-trading-platform/wallet-service \
  --set image.tag=<TAG>

helm install marketdata-service ./charts/marketdata-service \
  --set image.repository=<ECR_URL>/mock-trading-platform/marketdata-service \
  --set image.tag=<TAG>

helm install frontend ./charts/frontend \
  --set image.repository=<ECR_URL>/mock-trading-platform/frontend \
  --set image.tag=<TAG> \
  --set env.VITE_API_BASE="/api"
```

Ingress는 `charts/mock-trading-platform-ingress`에서 단일 리소스로 path-based routing을 제공한다. Secret 관리는 별도 환경 values에서 주입한다.

## CI

GitHub Actions 워크플로우 (`ci.yaml`) 가 서비스 빌드와 chart 검증을 수행한다.

### 서비스 CI

| 이벤트 | 테스트 | Docker 빌드 | ECR 푸시 |
|--------|--------|-------------|----------|
| PR | O (서비스 matrix 전체) | O (검증용) | X |
| Push to main | O (서비스 matrix 전체) | O | O (AWS 설정 시) |
| Tag push `v*` | O (전체) | O | O (AWS 설정 시) |

AWS 변수(`AWS_ACCOUNT_ID`, `AWS_REGION`, `AWS_ROLE_ARN`)가 설정되지 않으면 ECR push는 스킵되고 나머지는 정상 실행된다.

### Helm Chart Lint

`charts/**` 또는 `services/**` 변경 시 `helm lint`로 chart 템플릿을 검증한다.

`helm lint`는 chart 구조 및 템플릿 렌더링 문법의 1차 검증 단계이다.
실제 클러스터 배포 가능 여부나 values의 의미적 정합성까지 보장하지는 않는다.

## 빌드 및 테스트

```bash
# order-service
cd services/order-service && go test -v ./... && cd -

# auth-service
cd services/auth-service && go test -v ./... && cd -

# wallet-service
cd services/wallet-service && go test -v ./... && cd -

# marketdata-service
cd services/marketdata-service && go test -v ./... && cd -

# frontend
cd services/frontend && npm ci && npm test -- --run && npm run build && cd -

# Docker
docker build -t order-service services/order-service
docker build -t auth-service services/auth-service
docker build -t wallet-service services/wallet-service
docker build -t marketdata-service services/marketdata-service
docker build -t frontend services/frontend

# Helm lint
helm lint charts/order-service
helm lint charts/auth-service
helm lint charts/wallet-service
helm lint charts/marketdata-service
helm lint charts/frontend
```

## 현재 제약 및 다음 단계

구현 완료:
- order-service (주문 생성/조회/목록/취소, PostgreSQL, JWT 검증)
- order-service 내부 Phase 3 준비 코드
  - 인메모리 matcher core (price-time priority, single pair `BTC-KRW` 전제)
  - match 결과 DB 반영 유틸(`PersistMatchResult`) 및 trade persistence 스키마
  - DB open / partially_filled 주문 기반 order book rebuild 유틸
- auth-service (Mock 로그인, JWT 발급)
- wallet-service (health/ready + Phase 2 이벤트 consumer wiring)
- marketdata-service (health/ready + Phase 2 이벤트 consumer wiring)
- frontend (Vite+React SPA, nginx static serving, same-origin `/api` 호출 + ingress path routing)
- Helm chart (5개 서비스)
- CI (test, docker build, ECR push, chart lint)
- ECR 이미지 경로 `mock-trading-platform/<service>` 규칙 적용
- 이벤트 버스: NATS JetStream
  - order-service matcher runtime 경로에서 `order.created` / `order.updated` /
    `trade.executed` 를 best-effort 로 발행
  - wallet-service / marketdata-service 는 `order.*` 를 구독하여 구조화 로그 +
    카운터를 증가시키는 consumer wiring 보유
  - 두 consumer 모두 stream subject 는 `order.>` + `trade.>` 를 허용하지만,
    실제 subscribe 는 `order.*` 로 동작
  - `EVENT_BUS_ENABLED=false` 일 때는 세 서비스 모두 no-op 모드로 기동 가능

아직 미구현:
- wallet-service 실제 잔고 로직 (Phase 7)
- marketdata-service WebSocket streaming (Phase 5)
- frontend trading terminal 라우트(`/trade/:pair`) 및 관련 UI (Phase 4.5)
- Kubernetes Secret 관리
- DB 마이그레이션 도구

### Phase 2 cross-repo 필요 사항

이 리포는 애플리케이션 코드와 Helm chart 만 관리한다. 실제 NATS JetStream 클러스터
배포, ingress 연결, namespace 연결 등은 아래 리포에서 함께 움직여야 Phase 2 가 실제
환경에서 동작한다.

- `mock-trading-platform-infra`: NATS 배포 전용 인프라(필요 시), IAM / 네트워크 경로
- `mock-trading-platform-gitops`: NATS Helm chart (예: `nats-io/nats`) 배포, 각 서비스 values
  override (`env.EVENT_BUS_ENABLED`, `env.NATS_URL`, `env.NATS_STREAM`,
  `env.NATS_DURABLE`)

이 리포 단독으로는 `EVENT_BUS_ENABLED=false` 기본값을 유지한다. NATS 가 준비된
환경에서 gitops 쪽이 values override 로 활성화한다.
