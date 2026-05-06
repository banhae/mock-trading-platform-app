# TRADING_UI_PLAN

차트, 호가창, 최근 체결, 주문 ticket, open orders 같은 **"실제 거래소처럼 보이는"**
트레이딩 화면으로 가기 위한 phase 계획 문서다.

레퍼런스는 국내 현물 거래소형 단일 마켓 화면이지만, **거래소 전체 UI를 복제하는 것**
이 목표는 아니다. 목표는 **핵심 trading surface** 를 갖춘 mock 거래 터미널을
점진적으로 만드는 것이다.

이 문서는 **지금 당장 구현하자는 승인 문서가 아니다.**
현재 설계의 경계선을 명확히 보고, 어떤 순서로 확장해야 하는지 합의해두기 위한
계획 문서다.

실제 작업은 각 phase 별로 별도 승인을 받은 뒤 진행한다.

---

## 비목표

이 문서가 목표로 하지 않는 것:

- 실제 거래소 전체 정보구조 복제
  - 자산
  - 입출금
  - 아카이브
  - 고객지원
  - 전체 글로벌 네비게이션
- production-grade trading system
- 외부 거래소 시세 연동을 전제로 한 설계
- 프론트엔드 전용 fake 데이터로만 만든 데모 UI
- 완전한 matching engine / 저지연 고빈도 시스템

---

## 전제 — 현재 (Phase 0)

현재 구현된 API 를 기준으로 본 현실:

- `auth-service`
  - `POST /login`
  - `GET /health`
  - `GET /ready`
- `order-service`
  - `POST /orders`
  - `GET /orders/{id}`
  - 주문 status 는 사실상 `open` 고정
  - matching 없음
  - 체결 개념 없음
- `marketdata-service`
  - health/ready 만 있는 stub
- `wallet-service`
  - health/ready 만 있는 stub
- 이벤트 버스 없음
- `frontend`
  - 로그인
  - 주문 생성
  - 주문 id 단건 조회
  - health 확인

즉 지금은 **"주문을 기록하는 CRUD 앱"** 이다.
거래소 UI (차트, 호가창, 최근 체결, 실시간 스트리밍) 가 의존할 **데이터 자체가
없다.**

프론트에 차트 라이브러리나 호가창 컴포넌트를 얹어봤자 연결할 소스가 없기 때문에,
이 문제는 프론트 단독으로 해결할 수 없다.

CLAUDE.md 의 "아직 하지 말 것" 항목
(완전한 matching engine, 고빈도 저지연 최적화 등) 과 충돌하지 않는 선에서,
아래 순서로 점진적으로 확장한다.

---

## 공통 규칙

### 1) Pair 표기 규칙

초기 단일 pair 는 아래 규칙으로 통일한다.

- **canonical pair id**: `BTC-KRW`
- **display symbol**: `BTC/KRW`

규칙:

- API path, DB, 이벤트, WebSocket channel 은 **canonical pair id** 사용
- UI 표시 문자열만 **display symbol** 사용

예:

- REST path: `/marketdata/candles/BTC-KRW`
- 이벤트: `trade.executed` with `pair=BTC-KRW`
- WS channel: `candles:BTC-KRW:1m`
- 화면 표기: `BTC/KRW`

### 2) 숫자 타입 규칙

가격, 수량, 금액은 아래 원칙을 따른다.

- `float` 를 핵심 거래 로직에 사용하지 않는다
- REST contract 는 decimal string 또는 고정 소수점 정수 기반으로 유지한다
- matching, persistence, aggregation 에서 부동소수 오차를 허용하지 않는다

### 3) 데이터 우선 원칙

- 차트/호가창/체결 UI 는 **실제 내부 marketdata REST 또는 stream** 이 준비된 뒤에만 붙인다
- 프론트 전용 fixture JSON 으로만 화면을 먼저 만들지 않는다
- 화면이 비어 있어도 괜찮다. 빈 상태를 표현하는 편이 fake 데이터보다 낫다

### 4) 초기 범위 제한

초기 트레이딩 터미널은 아래만 구현한다.

- market summary
- candle chart
- order book
- recent trades
- limit order ticket
- open orders / cancel

아래는 스코프 밖이다.

- 마진
- 숏
- 청산
- stop / IOC / FOK / post-only / iceberg
- drawing tools
- RSI / MACD / MA 같은 지표
- 멀티 차트
- 심볼 검색
- 멀티 마켓 탭

---

## Phase 1 — Order lifecycle 완성

**목표**:
"주문에 상태 변화가 있다" 는 모델을 order-service 내부에 먼저 자리잡게 한다.
Phase 2 이벤트 버스와 Phase 3 matcher 의 전제 조건이다.

추가:

- order-service
  - `status` enum:
    - `open`
    - `partially_filled`
    - `filled`
    - `cancelled`
  - 주문 모델에 상태 전이를 표현할 최소 필드 추가
    - `created_at`
    - `updated_at`
    - `remaining_quantity`
  - `GET /orders?status=...&limit=...`
    - **현재 로그인 사용자 기준** 내 주문 목록
    - user query param 을 public API 에 노출하지 않는다
  - `DELETE /orders/{id}`
    - 취소
- frontend
  - `/orders` 페이지를 "My orders" 중심으로 정상화
  - 현재처럼 id 를 직접 입력해 lookup 하는 디버그형 구조에서 벗어난다
  - open order 취소 기능 추가

스코프 밖:

- 실제 체결
- 주문 수정 (order amend)
- 체결 평균가 계산 고도화

---

## Phase 2 — 이벤트 버스 도입

**목표**:
Phase 3 matcher 가 발행하는 체결 이벤트를 marketdata / wallet 이 받을 수 있는
토대를 만든다.

이 순서를 뒤집으면 marketdata 가 결국 DB polling 기반으로 만들어지고,
나중에 다시 갈아엎게 된다.

선택지:

| 옵션 | 장점 | 단점 | 추천도 |
|---|---|---|---|
| A. NATS JetStream | 가볍고 Helm 연동이 단순함 | Kafka 대비 학습 폭은 좁음 | **추천** |
| B. Redis Streams | 비교적 단순하고 익숙할 수 있음 | 소비자 그룹/운영 의미론이 제한적 | 차선 |
| C. Kafka (MSK / Strimzi) | "진짜 거래소 스택" 느낌 | infra 부담이 큼 | 현재 단계에 과함 |
| D. in-process pubsub | 당장 제일 쉬움 | 서비스 간 결합이 불가능 | 비추천 |

추가:

- order-service
  - 주문 생성 시 `order.created` publish
  - 주문 상태 변경 시 `order.updated` publish
- marketdata-service
  - consumer wiring
  - 초기에는 수신 로그/카운터 수준으로 시작 가능
- wallet-service
  - consumer wiring
  - 초기에는 수신 로그/카운터 수준으로 시작 가능
- Helm / values
  - exchange-app `charts/` 와 환경별 values 에 event bus 배포 wiring 반영

스코프 밖:

- exactly-once
- transactional outbox
- schema registry
- DLQ 고도화

---

## Phase 3 — 최소 matching engine

**목표**:
체결 이벤트를 만들어낸다.
이 phase 를 지나기 전까지는 차트/호가창/최근 체결 UI 가 원천적으로 불가능하다.

**위험**:
가장 실수하기 쉬운 phase 다.
"제대로 만들면 끝이 없다."
의도적으로 작게 만든다.

강제 제약:

- **가격-시간 우선순위 (price-time priority) 만**
- **limit order 만**
- pro-rata, iceberg, stop, post-only, IOC, FOK 금지
- **단일 프로세스, 단일 goroutine** 으로 order book 유지
- lock-free / sharding / multi-book concurrency 금지
- **in-memory book** 이 primary
- DB 는 journal / 복구용
- 재기동 시 DB 의 open 주문을 읽어 book 재구성
- **pair 당 하나의 book**
- 초기에는 **`BTC-KRW` 한 개만**
- matcher 는 **order-service 내부 컴포넌트**
  - 별도 서비스 분리 금지

추가:

- order-service 내부에 matcher 컴포넌트 내장
- 체결 시 `trade` 레코드 저장
  - 예: `trade_id`, `pair`, `price`, `quantity`, `maker_order_id`, `taker_order_id`, `executed_at`
- `trade.executed` 이벤트 publish
- 주문 상태 자동 전이
  - `open → partially_filled → filled`
- cancel 된 주문은 book 에서 제거

스코프 밖:

- call auction
- 마진 / 숏 / 청산
- 수수료 모델
- cross-pair arbitrage 방지
- 고급 self-trade prevention
- 멀티 pair 동시 운영

---

## Phase 4 — marketdata aggregation (REST)

**목표**:
marketdata-service 가 실제로 일하기 시작한다.
호가창, 캔들, 최근 체결, 요약 시세를 REST 로 제공한다.

추가:

- consumer
  - `trade.executed`
  - `order.created`
  - `order.updated`
- in-memory 상태
  - **order book snapshot**
    - bid/ask ladder
    - 가격 단계별 누적 quantity
  - **OHLCV bucket**
    - `1m`, `5m`, `1h`
  - **recent trades**
  - **ticker summary**
    - 현재가
    - 24h 변화율
    - 24h 고가/저가
    - 24h 거래량

REST:

- `GET /marketdata/ticker/{pair}`
- `GET /marketdata/orderbook/{pair}?depth=20`
- `GET /marketdata/candles/{pair}?interval=1m&limit=200`
- `GET /marketdata/trades/{pair}?limit=50`

frontend / proxy:

- frontend-nginx 에 `/api/marketdata/*` rewrite 추가
- 브라우저는 여전히 frontend origin 만 호출

스토리지 전략:

1. **1차**: 순수 in-memory
   - 재기동 시 차트가 비어도 수용
2. **2차 (선택)**: postgres / TimescaleDB 등에 캔들 누적
   - 1차와 한 번에 묶지 않는다

스코프 밖:

- WebSocket push
- 히스토리컬 대량 백필 import
- 외부 데이터 연동

---

## Phase 4.5 — Polling 기반 트레이딩 터미널 UI

**목표**:
WebSocket 이전에도 **포트폴리오에서 임팩트 있는 거래소형 화면** 을 먼저 만든다.
이 phase 를 지나면 더 이상 "주문 CRUD 백오피스급 앱" 처럼 보이지 않아야 한다.

핵심 원칙:

- fake 데이터 금지
- Phase 4 의 실제 REST 응답만 사용
- 아직 실시간이 아니어도 괜찮다
- **보이는 화면의 수준** 을 먼저 끌어올린다

차트 라이브러리:

- `lightweight-charts`
  - 크기가 작고 사용이 단순하다
  - mock 거래 터미널 수준에서는 가장 덜 아프다

신규 라우팅:

- `/trade/:pair`
- 초기 기본 진입 경로는 `/trade/BTC-KRW`

컴포넌트:

- `MarketSummaryBar`
  - 현재가
  - 전일 대비
  - 24h 고가/저가
  - 24h 거래량
- `CandleChart`
  - 초기 200개 로드
  - polling 으로 갱신
- `OrderBook`
  - depth 20 기준
  - bid/ask depth bar 시각화
- `TradeTape`
  - 최근 체결 리스트
- `OrderTicket`
  - 현재 `/orders` 페이지의 생성 폼을 이관
- `OpenOrdersPanel`
  - 내 open 주문 리스트
  - cancel 가능

데이터 갱신 전략:

- `ticker`, `orderbook`, `trades`, `candles`, `my orders` 를 2~5초 간격 polling
- 주문 생성 / 취소 직후에는 관련 영역을 즉시 refetch

라우팅 역할 분리:

- `/trade/:pair`
  - 메인 trading surface
- `/orders`
  - 내 주문 관리 / 디버그 / 보조 화면

스코프 밖:

- 글로벌 거래소 네비게이션 복제
- 자산/입출금/아카이브 화면
- 고급 차트 도구
- 멀티 마켓 검색

---

## Phase 5 — 실시간 스트리밍

**목표**:
Phase 4.5 의 화면은 그대로 유지하고, 데이터 공급 방식을 polling 에서
streaming 으로 바꾼다.
여기서부터 "진짜 거래소 같은" 느낌이 들어온다.

**주의**:
이 phase 는 **exchange-app 리포 단독으로 완결되지 않는다.**

영향 범위:

- ALB / ingress idle timeout
- long-lived connection
- WebSocket proxy
- health check 경로
- values / ingress 설정

따라서 exchange-infra 와 exchange-gitops 가 같이 움직인다.

추가:

- marketdata-service 에 **WebSocket** endpoint
  - `ws://.../marketdata/stream`
- subscribe 예시:
  ```json
  {
    "type": "subscribe",
    "channels": [
      "orderbook:BTC-KRW",
      "trades:BTC-KRW",
      "candles:BTC-KRW:1m"
    ]
  }
  ```
- push payload
  - 호가창 diff
  - 최근 체결
  - 마지막 캔들 업데이트
  - ticker summary 변경분
- frontend nginx 또는 이후 api-gateway 에 WebSocket proxy 설정
  - `proxy_http_version 1.1`
  - `Upgrade` / `Connection` 헤더 pass-through
  - idle timeout 조정
- 브라우저는 최소 수준의 재연결 + 지수 백오프 구현

스코프 밖:

- 멀티 프로세스 fanout
- per-connection rate limiting 고도화
- 개인 체결 / 개인 주문 상태용 authenticated channel
- presence

---

## Phase 6 — 프론트엔드 다듬기 / 포트폴리오 완성도 강화

**목표**:
핵심 trading surface 를 **보기 좋은 하나의 터미널** 로 다듬는다.
여전히 전체 거래소 UI 를 복제하지는 않는다.

추가:

- 화면 배치 최적화
  - 상단 market summary
  - 중앙 chart
  - 우측 order ticket
  - 하단 open orders / recent trades
  - 또는 chart / orderbook / ticket 3분할
- 상태 처리 보강
  - loading
  - empty
  - stale
  - reconnecting
  - error
- 시각 표현 보강
  - 최근 체결 buy/sell 색상 구분
  - order book depth 시각화
  - last price 강조
- 입력 보강
  - 수량 / 가격 validation
  - tick size / precision display
- 라우팅 및 컴포넌트 경계 정리
  - `/trade/:pair` 가 메인
  - `/orders` 는 관리 화면
  - `/health` 는 운영 확인용 유지

스코프 밖:

- drawing tools
- 지표
- 멀티 차트 레이아웃
- 심볼 검색 패널
- 다크모드/테마 시스템 고도화
- 모바일 퍼스트 최적화
- 거래소 전체 메뉴 구조 재현

---

## Phase 7 (선택) — Wallet 반영

**순서 flexible**:
차트/호가창과 독립적이다.
Phase 3 이후라면 어느 시점에 끼워넣어도 된다.

추가:

- wallet-service
  - `trade.executed` consume
  - mock 잔고 반영
- 필요 시 단계적으로 추가
  - 주문 open 시 hold
  - cancel 시 release
  - fill 시 balance 이동
- API
  - `GET /wallet/balance`
- frontend
  - available / locked balance 표시

스코프 밖:

- 실제 온체인 서명
- 출금 처리
- 외부 custody 연동

---

## 공통 전략 — Demo seed / DEV_MODE

포트폴리오 화면을 살리기 위해 seed 데이터가 필요할 수 있다.
단, 아래 원칙을 지킨다.

- seed 는 **DEV_MODE 에서만** 허용
- seed 데이터는 **실제 API / 이벤트 경로** 를 통해 주입한다
- 프론트 컴포넌트 내부 하드코딩 fixture 로 대체하지 않는다
- seed 가 없어도 UI 는 empty state 를 정상 처리해야 한다

권장 방식:

- Phase 3 이후
  - demo 주문/체결을 넣는 seed script
  - 또는 DEV_MODE bootstrap
- 결과:
  - chart 가 너무 비어 보이는 문제 완화
  - 하지만 데이터 경로는 실제 구현과 동일하게 유지

---

## 의존 관계 요약

```text
Phase 0 (현재 — 주문 CRUD 앱)
  │
  ▼
Phase 1: order lifecycle + 내 주문 목록
  │
  ▼
Phase 2: 이벤트 버스
  │
  ▼
Phase 3: 최소 matcher
  │
  ▼
Phase 4: marketdata REST 집계
  │
  ▼
Phase 4.5: polling 기반 trading terminal UI
  │
  ▼
Phase 5: WebSocket 스트리밍
  │
  ▼
Phase 6: UI 다듬기 / 포트폴리오 완성도 강화

Phase 7: wallet (Phase 3 이후 어디서나 독립 진행 가능)
```

핵심 포인트:

- **Phase 3 전까지는 체결이 없다**
- **Phase 4 전까지는 marketdata REST 가 없다**
- **Phase 4.5 전까지는 포트폴리오에서 "거래소처럼 보이는 화면" 이 나오기 어렵다**
- **Phase 5 는 3개 리포가 함께 움직일 가능성이 높다**

---

## 놓치기 쉬운 함정

1. **Phase 4.5 를 건너뛰고 바로 WebSocket 부터 붙이려는 유혹**
   - streaming 자체보다 먼저 중요한 것은 "터미널 형태의 화면" 이다
   - polling 기반으로 먼저 화면을 완성하고, 이후 transport 만 교체하는 편이 안전하다

2. **Phase 3-4 없이 프론트만 예쁘게 만드는 유혹**
   - fake 데이터가 필요해지고
   - 실데이터 연결 시 대부분 다시 만든다
   - 절대 금지

3. **Phase 3 matcher 를 "제대로" 만들려는 유혹**
   - price-time priority / single goroutine / single pair 제약을 반드시 지킨다
   - 그 이상은 별도 phase 로 쪼갠다

4. **float 사용**
   - 가격/수량/금액 계산에 float 를 쓰면
   - 체결/집계/캔들 계산에서 신뢰도가 떨어진다
   - 초기에 규칙을 못 박아야 한다

5. **Phase 2 의 선택이 infra 를 흔든다**
   - Kafka 를 고르면 exchange-infra 와 exchange-gitops 변경폭이 커진다
   - 현재 학습 단계에서는 NATS JetStream 이 가장 덜 아프다

6. **Phase 5 는 더 이상 exchange-app 만의 문제가 아니다**
   - WebSocket 은 ingress/ALB/timeout/sticky 문제를 같이 만든다
   - 이 phase 부터는 3개 리포가 한 묶음으로 움직인다고 생각해야 한다

7. **api-gateway phase 와의 상호작용**
   - 현재 frontend nginx 가 임시 reverse proxy 책임을 맡고 있다
   - 이후 전용 `api-gateway` phase 에서 이 책임이 gateway 로 이관되면,
     `/api/marketdata/*` rewrite 와 WebSocket proxy 도 같이 옮겨진다
   - 따라서 Phase 4 이상을 시작하기 전에 api-gateway phase 의 순서를 한 번 더 점검한다

---

## 이 문서의 사용법

- 이 문서는 **계획 문서일 뿐** 이다
- 이 문서가 생겼다고 해서 어떤 phase 도 자동으로 시작되지 않는다
- 각 phase 는 별도 이슈 / PR 로 나누어 진행한다
- phase 를 시작할 때는, 해당 phase 의 더 구체적인 구현 계획 문서를 별도로 만든 뒤 승인받는다
- 범위를 벗어나는 옆길 유혹
  - 예: matcher 에 수수료 모델 추가
  - 예: 차트에 drawing tool 추가
  - 예: 글로벌 거래소 메뉴까지 복제
  가 생기면, 이 문서의 **스코프 밖** 항목을 근거로 거절한다