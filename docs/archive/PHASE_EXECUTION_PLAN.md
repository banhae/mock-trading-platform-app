# PHASE_EXECUTION_PLAN

`mock-trading-platform-app`의 현재 상태를 기준으로, **Phase 4.5 구현 전에 반드시 정리해야 하는 작업을 구현 단위로 쪼개고 실행 순서를 부여한 문서**다.

핵심 원칙:

- **P0를 끝내기 전에는 Phase 4.5를 시작하지 않는다.**
- **실제 체결(`trade.executed`)이 먼저 살아야 marketdata REST가 의미를 가진다.**
- **marketdata REST가 먼저 살아야 polling 기반 trading terminal UI가 fake 데이터 없이 구현 가능하다.**
- `/orders`는 기존 관리/디버그 화면으로 유지하되, `/trade/:pair` 도입을 방해하지 않도록 사전 정리한다.

---

## 1. 실행 순서 요약

## P0 — 지금 바로 구현해야 하는 순서

| 순서 | 우선순위 | Phase | 구현 단위 | 목표 |
|---|---|---|---|---|
| 1 | P0 | Phase 3 | 주문 생성 런타임에 matcher 연결 | `POST /orders`가 더 이상 단순 CRUD가 아니라 실제 체결 가능 경로가 되게 한다 |
| 2 | P0 | Phase 3 | 취소/상태 전이/이벤트 일관성 정리 | matcher 도입 후 주문 lifecycle이 깨지지 않도록 한다 |
| 3 | P0 | Phase 1 후속 | `/orders` 화면을 새 상태 모델에 맞게 정리 | `partially_filled` / `filled` / `cancelled`를 기존 화면이 정상 표현하게 만든다 |
| 4 | P0 | Phase 4 | marketdata consumer를 실제 read model로 확장 | `trade.executed`와 `order.*`를 바탕으로 orderbook/trades/candles/ticker를 집계한다 |
| 5 | P0 | Phase 4 | marketdata REST API 구현 | frontend가 실제 marketdata를 HTTP로 읽을 수 있게 한다 |
| 6 | P0 | Phase 4 | frontend proxy / env wiring 추가 | 브라우저가 same-origin으로 `/api/marketdata/*`를 호출할 수 있게 한다 |
| 7 | P0 | 검증 게이트 | P0 통합 검증 | 이 게이트를 통과해야만 Phase 4.5 작업을 시작한다 |

## P1 — P0 직후 Phase 4.5 진입 준비

| 순서 | 우선순위 | Phase | 구현 단위 | 목표 |
|---|---|---|---|---|
| 8 | P1 | Phase 4.5 | frontend 데이터 접근 계층 정리 | ticker/orderbook/trades/candles/my orders polling 훅 구성 |
| 9 | P1 | Phase 4.5 | `/trade/:pair` 라우트와 레이아웃 골격 도입 | trading surface 진입 경로 생성 |
| 10 | P1 | Phase 4.5 | `MarketSummaryBar`, `OrderBook`, `TradeTape` 구현 | marketdata 읽기 전용 영역부터 붙인다 |
| 11 | P1 | Phase 4.5 | `CandleChart` 구현 | 초기 200개 캔들 로드 + polling 갱신 |
| 12 | P1 | Phase 4.5 | `OrderTicket`, `OpenOrdersPanel` 구현 | 기존 `/orders`의 주문 생성/취소 기능을 trading surface에 이관 |
| 13 | P1 | Phase 4.5 | 주문 직후 refetch / empty/loading/error 처리 | fake 데이터 없이 usable한 화면 완성 |

## P2 — 구조 정리 / 재작업 비용 감소

| 순서 | 우선순위 | Phase | 구현 단위 | 목표 |
|---|---|---|---|---|
| 14 | P2 | 공통 | api-gateway 이관 전제 정리 | 현재 frontend nginx의 임시 proxy 책임을 분리 가능한 형태로 정리 |
| 15 | P2 | 공통 | DB migration 체계 도입 | matcher/trade/marketdata schema 변경을 수동 적용에서 분리 |
| 16 | P2 | 공통 | DEV_MODE seed 전략 문서화/구현 | fake UI 데이터 대신 실제 API/이벤트 경로로 seed 주입 |
| 17 | P2 | 공통 | 운영 계약 정리 | 이벤트 payload, pair 규칙, interval/depth/limit validation 고정 |

## P3 — 후속 확장

| 순서 | 우선순위 | Phase | 구현 단위 | 목표 |
|---|---|---|---|---|
| 18 | P3 | Phase 5 | WebSocket stream | polling UI를 유지한 채 transport만 교체 |
| 19 | P3 | Phase 6 | UI polish | stale/reconnecting/depth visualization/validation 강화 |
| 20 | P3 | Phase 7 | wallet 반영 | `trade.executed` 기반 balance / hold / release |

---

## 2. P0 상세 계획

### P0-1. 주문 생성 런타임에 matcher 연결

### 목표

현재 존재하는 matcher 코어/재구성 유틸/체결 persistence 유틸을 **실제 `POST /orders` 경로에 연결**한다.

### 작업 단위

1. **order-service 부팅 시 matcher 초기화 경로 고정**
   - 앱 시작 시 in-memory order book 생성
   - DB의 `open`, `partially_filled` 주문으로 book rebuild 수행
   - 초기 pair는 `BTC-KRW` 한 개만 허용

2. **주문 생성 application service 도입**
   - HTTP handler가 직접 `store.Create()`를 호출하지 않게 변경
   - `SubmitOrder()` 같은 application layer로 위임
   - 이 레이어가 아래 순서를 책임짐
     - 입력 검증
     - canonical pair 검증 (`BTC-KRW`)
     - matcher submit
     - match result persistence
     - `order.created` / `order.updated` / `trade.executed` 발행

3. **무체결 / 부분체결 / 완전체결 분기 고정**
   - 무체결: `open`
   - 일부만 체결: `partially_filled`
   - 전량 체결: `filled`
   - 응답 contract는 기존 `/orders` 소비자가 깨지지 않게 유지

4. **가격-시간 우선순위 테스트 추가**
   - 같은 가격에서 먼저 들어온 주문이 먼저 체결되는지 검증
   - sell book / buy book 교차 모두 검증

### 완료 기준

- `POST /orders`가 실제로 matcher를 타고 체결을 발생시킬 수 있다.
- 체결 발생 시 DB에 trade가 저장된다.
- `order.updated`, `trade.executed` 이벤트가 런타임에서 실제 발행된다.

### 선행 조건

- 없음

### 주요 영향 파일(예상)

- `order-service/internal/http/handler.go`
- `order-service/internal/matcher/*`
- `order-service/main.go` 또는 service bootstrap

---

### P0-2. 취소/상태 전이/이벤트 일관성 정리

### 목표

matcher를 붙인 뒤에도 취소, 남은 수량, 상태 전이가 서로 충돌하지 않게 만든다.

### 작업 단위

1. **`DELETE /orders/{id}`가 in-memory book과 DB를 함께 갱신**
   - open / partially_filled 주문만 취소 가능
   - filled / cancelled 주문은 재취소 불가

2. **remaining quantity / updated_at 일관성 검증**
   - 부분체결 후 취소 시 최종 상태가 `cancelled`로 정리되는지 확인
   - 음수 잔량, 이중 체결, 이중 취소 방지

3. **이벤트 발행 규칙 고정**
   - 상태 변경 시 `order.updated`
   - 체결 발생 시 `trade.executed`
   - consumer가 순서를 가정하지 않도록 payload를 충분히 self-contained 하게 유지

4. **회귀 테스트 추가**
   - open 주문 취소
   - partially_filled 주문 취소
   - 이미 filled/cancelled 주문 취소 시도

### 완료 기준

- cancel 이후 book snapshot과 DB 상태가 어긋나지 않는다.
- order lifecycle이 `open → partially_filled → filled/cancelled` 범위 안에서만 움직인다.

### 선행 조건

- P0-1

---

### P0-3. `/orders` 화면을 새 상태 모델에 맞게 정리

### 목표

기존 관리 화면이 matcher 도입 후에도 깨지지 않게 한다.

### 작업 단위

1. **상태 렌더링 확장**
   - `open`, `partially_filled`, `filled`, `cancelled` 표시
   - 남은 수량 / 생성시각 / 수정시각 표기 정리

2. **취소 버튼 조건 정리**
   - `open`, `partially_filled`일 때만 노출 또는 활성화

3. **현재 주문 생성 폼을 향후 이관 가능하게 분리**
   - form 로직을 페이지 내부에 묶어두지 않고 컴포넌트/훅으로 추출
   - 이후 `OrderTicket`으로 옮기기 쉽게 준비

4. **debug lookup 성격 축소**
   - `/orders`는 “내 주문 관리/보조 화면” 역할을 우선
   - direct id lookup은 꼭 필요할 때만 하단 보조 섹션으로 유지

### 완료 기준

- matcher가 붙어 실제 상태가 바뀌어도 `/orders` 페이지가 정상 동작한다.
- 이후 Phase 4.5에서 `OrderTicket`, `OpenOrdersPanel` 추출이 쉬워진다.

### 선행 조건

- P0-1, P0-2

### 주요 영향 파일(예상)

- `frontend/src/pages/OrdersPage.tsx`
- `frontend/src/components/*` (신규 추출 시)

---

### P0-4. marketdata consumer를 실제 read model로 확장

### 목표

Phase 3에서 살아난 이벤트를 받아서 **실제 marketdata read model**을 만든다.

### 작업 단위

1. **consumer subscribe 범위 확장**
   - `trade.executed`
   - `order.created`
   - `order.updated`

2. **in-memory orderbook read model 추가**
   - pair별 bid/ask ladder
   - price level aggregate quantity
   - depth slice 생성 함수

3. **recent trades read model 추가**
   - 고정 길이 ring buffer 또는 bounded slice
   - 최신순 반환 준비

4. **OHLCV bucket 추가**
   - 초기 interval: `1m`, `5m`, `1h`
   - bucket start 정규화
   - trade 수신 시 open/high/low/close/volume 갱신

5. **ticker summary 추가**
   - last price
   - 24h high/low
   - 24h volume
   - 24h change rate

6. **empty state 우선 전략 적용**
   - 백필/영속화는 지금 하지 않음
   - 재기동 후 비어 있어도 API가 정상 응답

### 완료 기준

- 실제 체결이 발생하면 marketdata-service 메모리에 orderbook/trades/candles/ticker가 갱신된다.
- 재기동 후에도 “에러”가 아니라 “empty/stale” 상태로 정상 응답한다.

### 선행 조건

- P0-1, P0-2

### 주요 영향 파일(예상)

- `marketdata-service/internal/consumer.go`
- `marketdata-service/internal/*` (read model 신규)
- `marketdata-service/main.go`

---

### P0-5. marketdata REST API 구현

### 목표

frontend가 polling으로 소비할 수 있는 실제 HTTP endpoint를 만든다.

### 작업 단위

1. **ticker endpoint**
   - `GET /marketdata/ticker/{pair}`

2. **orderbook endpoint**
   - `GET /marketdata/orderbook/{pair}?depth=20`
   - `depth` validation

3. **candles endpoint**
   - `GET /marketdata/candles/{pair}?interval=1m&limit=200`
   - `interval`, `limit` validation

4. **trades endpoint**
   - `GET /marketdata/trades/{pair}?limit=50`
   - 최신순/표시용 정렬 규칙 고정

5. **응답 contract 명세 고정**
   - 숫자는 decimal string 또는 현재 backend contract 원칙과 일치
   - pair 표기는 canonical pair id 사용

6. **핸들러 테스트 / smoke test 추가**
   - empty response
   - 정상 응답
   - invalid pair/interval/depth/limit

### 완료 기준

- frontend 없이 curl만으로도 ticker/orderbook/candles/trades를 읽을 수 있다.
- empty 데이터 상황에서도 500 없이 일관된 응답을 준다.

### 선행 조건

- P0-4

---

### P0-6. frontend proxy / env wiring 추가

### 목표

브라우저가 same-origin 기준으로 marketdata API를 호출할 수 있게 한다.

### 작업 단위

1. **frontend nginx에 `/api/marketdata/*` reverse proxy 추가**
2. **chart / values / env에 `MARKETDATA_UPSTREAM` 추가**
3. **local dev / cluster 값 정리**
4. **기존 `/api/auth/*`, `/api/orders*`와 충돌 없는지 확인**

### 완료 기준

- 브라우저에서 `/api/marketdata/ticker/BTC-KRW` 호출이 frontend origin을 통해 정상 동작한다.

### 선행 조건

- P0-5

### 주요 영향 파일(예상)

- `frontend/nginx.conf.template`
- frontend chart templates / values

---

### P0-7. P0 통합 검증 게이트

### 목표

Phase 4.5 착수 전에 “실제 데이터가 end-to-end로 흐른다”는 것을 확인한다.

### 체크리스트

1. 로그인 후 buy/sell limit order 2개로 실제 체결 발생
2. `/orders`에서 상태가 `filled` 또는 `partially_filled`로 보임
3. marketdata REST에서
   - ticker 응답 존재
   - orderbook depth 응답 존재
   - recent trades 응답 존재
   - candles 응답 존재
4. 체결 직후 polling 없이 수동 refetch만으로도 데이터 변화가 보임
5. 서비스 재기동 후에도 에러가 아니라 empty 또는 재구성 가능한 상태로 복구됨

### 이 게이트를 통과하면

- **그때부터 Phase 4.5 구현 프롬프트를 작성한다.**

---

## 3. P1 상세 계획 (Phase 4.5 진입)

### P1-1. frontend 데이터 접근 계층 정리

- `useTicker(pair)`
- `useOrderBook(pair, depth)`
- `useTrades(pair, limit)`
- `useCandles(pair, interval, limit)`
- `useMyOrders(status, limit)`
- polling 간격과 주문 직후 refetch 유틸 공통화

### P1-2. `/trade/:pair` 라우트와 레이아웃 골격

- 기본 진입: `/trade/BTC-KRW`
- 상단: `MarketSummaryBar`
- 중앙: `CandleChart`
- 우측: `OrderTicket`
- 하단/측면: `OrderBook`, `TradeTape`, `OpenOrdersPanel`

### P1-3. 읽기 전용 위젯부터 구현

1. `MarketSummaryBar`
2. `OrderBook`
3. `TradeTape`
4. `CandleChart`

이 순서로 가는 이유:

- 주문 기능 이관 전에도 marketdata REST만으로 독립 검증 가능
- fake 데이터 없이 화면 뼈대를 빨리 확인 가능

### P1-4. 주문 기능 이관

1. `/orders`의 생성 폼 로직을 `OrderTicket`으로 이동
2. open 주문 패널을 `OpenOrdersPanel`로 분리
3. 주문 생성/취소 직후 관련 쿼리 즉시 refetch

### P1-5. 상태 처리

- loading
- empty
- stale
- error
- polling 중 마지막 성공 시각 표시(선택)

---

## 4. P2 / P3 권장 순서

### P2

1. api-gateway 고려한 proxy 책임 분리
2. DB migration 도입
3. DEV_MODE seed (실제 API/이벤트 경로만 사용)
4. 운영 계약 문서화

### P3

1. WebSocket stream
2. UI polish
3. wallet 반영

---

## 5. Codex 작업 분할 권장 방식

한 번에 “Phase 3~4.5 전부 구현” 식으로 넘기지 말고 아래처럼 쪼개는 것이 안전하다.

### 권장 작업 묶음

- **Task A (P0-1 ~ P0-2)**
  - matcher runtime integration
  - cancel / lifecycle consistency
  - event emission consistency

- **Task B (P0-3)**
  - `/orders` 화면 상태 모델 정리
  - form / open orders 패널 추출 준비

- **Task C (P0-4 ~ P0-5)**
  - marketdata read model
  - marketdata REST

- **Task D (P0-6 ~ P0-7)**
  - frontend proxy wiring
  - E2E smoke / integration gate

- **Task E (P1-1 ~ P1-5)**
  - `/trade/:pair` + polling UI

---

## 6. 지금 당장 시작할 첫 작업

가장 먼저 시작해야 하는 것은 아래 하나다.

> **Task A: `POST /orders` 실시간 matcher 연결 + cancel/lifecycle/event consistency 정리**

이 작업이 끝나기 전까지는:

- 체결이 실제로 발생하지 않고
- `trade.executed`가 안정적으로 흐르지 않으며
- marketdata-service가 집계할 실데이터가 없고
- 따라서 Phase 4.5 UI는 시작해도 fake 데이터 유혹에 빠질 가능성이 높다.

