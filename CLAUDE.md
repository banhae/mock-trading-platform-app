# CLAUDE.md

## 리포 목적

이 리포는 exchange mock system 의 애플리케이션 코드와 Helm chart 를 관리한다.

책임 범위:

- 서비스 코드
- frontend 코드
- Dockerfile
- Helm charts
- GitHub Actions workflow
- 서비스 README
- phase 계획 문서 (`*_PLAN.md`)

책임 범위 아님:

- Terraform
- AWS infra
- ArgoCD root wiring
- 클러스터 bootstrap
- ALB / ingress 의 최종 프로비저닝 책임
- cross-repo 환경 wiring 전반

---

## 서비스 범위

현재 구현 서비스:

- frontend
- auth-service
- order-service
- wallet-service
- marketdata-service

핵심 서비스:

- order-service

참고:

- frontend nginx 는 정적 파일만 서빙한다
- `/api/*` 라우팅은 Kubernetes ALB Ingress 가 담당한다

---

## 현재 기준선 (Phase 4.5 진행 전)

현재 기준으로 가정해야 하는 현실:

- `auth-service`
  - `POST /login`
  - `GET /health`
  - `GET /ready`
- `order-service`
  - `POST /orders`
  - `GET /orders?status=&limit=`
  - `GET /orders/{id}`
  - `DELETE /orders/{id}`
  - matcher runtime 경로 기반 matching/trade persistence 존재
  - 상태 전이: `open` / `partially_filled` / `filled` / `cancelled`
- `wallet-service`
  - health/ready + NATS JetStream consumer wiring
- `marketdata-service`
  - health/ready + NATS JetStream consumer wiring
  - in-memory read model + REST 조회 API
- `frontend`
  - `/login`
  - `/orders`
  - `/trade/:pair`
  - `/health`
  - 브라우저는 frontend origin same-origin(`/api`)만 호출
- 이벤트 버스: NATS JetStream 사용
- live marketdata: REST read model 제공, WebSocket 은 미구현

명시적으로 확장 phase 작업을 시작하기 전에는,
이 기준선을 깨는 기능이 이미 존재한다고 가정하지 않는다.

---

## 현재 단계의 목표

현재는 작동하는 최소 앱을 유지하면서,
이후 `TRADING_UI_PLAN.md` 에 따라 **이벤트 기반 mock 거래 터미널** 로
확장 가능한 구조를 만드는 것이 목표다.

핵심 요구:

- order-service 는 주문 생성/조회의 기본 API 를 제공해야 한다
- auth-service 는 mock 인증을 제공해야 한다
- frontend 는 동일 origin 기반으로 backend API 를 호출할 수 있어야 한다
- wallet-service / marketdata-service 는 초기에는 stub 또는 최소 consumer 로 시작 가능하다
- 모든 서비스는 health/readiness endpoint 를 기본 제공한다
- Helm chart 로 EKS 배포 가능해야 한다

명시적 승인 전 가정하지 말 것:

- WebSocket streaming 이 이미 있다고 가정하지 말 것
- 멀티 pair 확장이 이미 완료되었다고 가정하지 말 것
- wallet 실제 잔고 hold/release 가 이미 구현되었다고 가정하지 말 것

---

## 트레이딩 확장 방향

이 리포는 "주문 CRUD 백오피스 앱" 에서 끝나지 않는다.

목표는 `TRADING_UI_PLAN.md` 의 phase 를 따라,
**데이터 파이프라인이 먼저 존재하는 상태** 에서
아래 요소를 갖춘 mock 거래 터미널로 점진 확장하는 것이다.

초기 trading UI 범위:

- market summary
- candle chart
- order book
- recent trades
- limit order ticket
- open orders / cancel

원칙:

- UI 는 실제 거래소 전체를 복제하지 않는다
- 차트/호가창 UI 는 실제 내부 marketdata REST 또는 stream 이 준비된 phase 에서만 확장한다
- 프론트엔드 전용 fake 데이터로 화면만 먼저 만들지 않는다
- `/trade/:pair` 는 trading surface
- `/orders` 는 관리 / 디버그 / 보조 화면으로 본다

---

## 아직 하지 말 것

- 완전한 matching engine
- 고빈도 저지연 최적화
- 복잡한 CQRS 프레임워크
- 지나친 DDD layering
- 과도한 interface 분리
- production-grade auth system
- 실제 결제 / 지갑 서명 / 온체인 출금 처리
- 마진 / 숏 / 청산 / 리스크 엔진
- stop / IOC / FOK / post-only / iceberg 같은 고급 주문 유형
- 프론트엔드 전용 fake market data
- 외부 거래소 연동을 core feature 전제로 설계하는 것

예외:

- 명시적으로 `TRADING_UI_PLAN.md` 의 **Phase 3** 을 수행하는 작업만,
  아래 제약을 지키는 **최소 matcher** 를 허용한다.
  - single process
  - single goroutine
  - single pair
  - price-time priority only
  - limit order only
  - matcher 는 `order-service` 내부 컴포넌트
  - 별도 matching-service 분리 금지

---

## 구현 원칙

1. order-service 중심
2. 단순한 구조
3. 작은 파일 수
4. 불필요한 보일러플레이트 제거
5. Kubernetes 배포 우선 설계
6. **데이터 우선, UI 후행**
7. 구체적인 운영상 이유 없는 서비스 분리 금지

설명:

- UI 는 실제 API / 이벤트 / stream 계약 위에 올라가야 한다
- "예뻐 보이는 화면" 보다 "실제 내부 데이터 흐름" 이 먼저다
- matcher, marketdata, gateway 를 쉽게 서비스 분리하지 않는다
- shared 패키지나 공통 레이어는 정말 필요할 때만 만든다

---

## 트레이딩 도메인 규칙

### Pair 규칙

- canonical pair id: `BTC-KRW`
- display symbol: `BTC/KRW`

규칙:

- API path, DB, 이벤트, WebSocket channel 은 canonical pair id 사용
- 화면 표시 문자열만 display symbol 사용
- 초기 범위는 **단일 pair**
- 멀티 pair 는 별도 승인 없이는 도입하지 않는다

### 숫자 규칙

가격, 수량, 금액은 아래 규칙을 따른다.

- 핵심 거래 로직에 `float` 사용 금지
- decimal string 또는 fixed-point integer 기반으로 유지
- matching / persistence / aggregation / candle 계산에서 부동소수 오차 허용 금지

### 사용자 범위 규칙

- "내 주문" 은 JWT 기반 현재 사용자 컨텍스트로 결정한다
- public API 에 user query param 을 노출하는 방식은 피한다

### 기본 이벤트 이름

- `order.created`
- `order.updated`
- `trade.executed`

### 기본 WebSocket channel naming

- `orderbook:<PAIR>`
- `trades:<PAIR>`
- `candles:<PAIR>:<INTERVAL>`

예:

- `orderbook:BTC-KRW`
- `trades:BTC-KRW`
- `candles:BTC-KRW:1m`

---

## 디렉터리 원칙

- `services/` 아래에 서비스별 폴더를 둔다
- `charts/` 아래에 서비스별 chart 를 둔다
- 공통 코드가 정말 필요할 때만 shared 패키지를 만든다
- 초기에 공통 라이브러리를 과도하게 만들지 않는다
- phase 진행을 위한 루트 문서 (`*_PLAN.md`) 는 허용한다

---

## 코드 스타일

- 읽기 쉬운 구현을 우선한다
- mocking 가능한 구조는 좋지만 추상화를 남발하지 않는다
- config 는 환경변수 기반으로 단순하게 간다
- 로그는 구조화된 형태를 선호한다
- health/readiness endpoint 를 기본 제공한다
- trading state transition 은 숨기지 말고 명확히 드러나게 구현한다
- matching / aggregation 코어는 가능하면 결정적이고 테스트하기 쉽게 만든다
- frontend 컴포넌트는 실제 API contract 를 기준으로 작성한다

---

## frontend 규칙

- 브라우저는 항상 frontend origin 만 호출한다
- backend URL 을 브라우저에 직접 노출하지 않는다
- frontend nginx 는 정적 파일만 서빙한다
- `/api/*` 라우팅은 Kubernetes Ingress(ALB)가 담당한다
- trading surface 가 생기면 메인 라우트는 `/trade/:pair` 다
- `/orders` 는 내 주문 관리 / 디버그 / 보조 화면이다
- 거래소 전체 정보구조를 복제하지 않는다
- 차트가 필요할 때는 기본 선택지로 `lightweight-charts` 를 우선 검토한다
- UI 는 실제 데이터가 빈 경우도 정상 처리해야 한다
- hardcoded fixture 로만 돌아가는 trading UI 를 "완성" 으로 간주하지 않는다

---

## API / 이벤트 규칙

- 브라우저 공개 경로는 상대경로, same-origin 으로 유지한다
- public path 는 Ingress 에서 backend 서비스로 라우팅된다
- marketdata public API 는 `/api/market/*` 기준으로 확장한다
- order list 는 현재 사용자 기준이어야 한다
- 취소는 `DELETE /orders/{id}` 를 기본으로 본다
- 이벤트 이름 / pair naming / 숫자 표현 규칙은 쉽게 바꾸지 않는다
- 한 번 정한 이벤트 이름과 pair 규칙을 깨는 변경은 별도 합의 없이 넣지 않는다

---

## Helm 규칙

각 chart 는 최소한 아래를 지원해야 한다.

- image repository
- image tag
- replica count
- env
- resources
- service
- ingress
- probes

추가 원칙:

- library chart 나 overly generic helper 는 초기 단계에서 만들지 않는다
- 실제 앱 구조보다 chart 를 더 추상화하지 않는다
- WebSocket / ingress / timeout 관련 변화는 exchange-infra / exchange-gitops 와
  함께 조정될 가능성이 높다는 점을 전제로 한다

---

## 테스트 규칙

각 서비스는 최소 1개 이상의 테스트를 둔다.

기본 테스트 우선순위:

- health
- handler
- basic business path

phase 가 확장되면 최소한 아래를 추가 검토한다.

- order lifecycle
  - 상태 전이
  - cancel
- matcher
  - price-time priority
  - partial fill
- marketdata
  - candle aggregation
  - orderbook snapshot 계산
  - recent trades 정렬
- frontend
  - public API path 사용
  - 주요 컴포넌트 렌더링
  - 오류 / 빈 상태 처리

원칙:

- 무거운 통합테스트보다 결정적이고 빠른 테스트를 선호한다
- trading core 의 correctness 는 적은 수의 통합테스트보다 핵심 단위테스트로 먼저 지킨다

---

## 파일 작업 규칙

새 코드를 쓰기 전 반드시 아래를 먼저 제시한다.

1. 생성/수정 파일 목록
2. 각 파일 역할
3. 구현 순서

추가 원칙:

- 한 번에 모든 서비스를 완성하려 하지 말고 작은 단위로 나눈다
- phase 작업은 해당 phase 범위 안에서만 움직인다
- 범위를 벗어나는 기능은 별도 phase 또는 별도 이슈로 분리한다

---

## GitHub Actions 원칙

초기 workflow 는 단순해야 한다.

- build
- test
- docker build
- push to ECR

아직 하지 말 것:

- 복잡한 matrix build
- semantic release
- multi-arch 고도화
- environment promotion pipeline

원칙:

- CI 는 서비스별 변경 감지와 기본 검증에 집중한다
- phase 확장 때문에 CI 를 과하게 복잡하게 만들지 않는다

---

## 금지사항

- 필요 없는 마이크로서비스 추가 금지
- boilerplate generator output 그대로 대량 반입 금지
- local docker-compose 중심 설계 금지
- Helm chart 를 실제 앱 구조보다 과하게 일반화하지 말 것
- frontend 전용 fixture 데이터로만 돌아가는 trading UI 를 merge 하지 말 것
- matcher / marketdata / gateway 를 이유 없이 더 잘게 쪼개지 말 것
- "진짜 거래소처럼 보이게 하자" 를 명분으로 범위가 끝없이 커지는 변경을 넣지 말 것
