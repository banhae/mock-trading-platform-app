# MATCHER_PLAN.md

## 1. 목적과 범위

이 문서는 **Phase 3(최소 매칭 엔진)** 구현 전에 설계를 고정하기 위한 계획 문서다. 구현 목표는 `order-service` 내부에서 **단일 페어 `BTC-KRW` 대상 limit 주문 매칭**을 수행하고, 주문 상태/체결 이력을 일관되게 남기는 것이다.

본 문서는 설계 결정만 다루며, 런타임 코드/핸들러/프론트/헬름/웹소켓/마켓데이터 REST 추가는 범위 밖이다.

---

## 2. Phase 3의 하드 제약

다음 제약은 선택사항이 아니라 **강제사항**이다.

- matcher 위치: `order-service` 내부 컴포넌트
- 배포/프로세스: 별도 matching-service 금지, 단일 프로세스
- 실행 모델: 단일 goroutine
- 거래 대상: 단일 pair `BTC-KRW`만 지원
- 매칭 규칙: price-time priority만
- 주문 유형: limit order만
- 미지원: market/stop/IOC/FOK/post-only/iceberg
- 미지원: fee engine, margin/short/liquidation/risk engine
- 숫자 규칙: float 기반 핵심 로직 금지
- 미포함: marketdata REST/WebSocket/프론트 trading UI/wallet balance logic

---

## 3. `order-service` 내부 아키텍처 제안

`order-service` 내부에 아래 최소 컴포넌트를 둔다.

- `MatcherEngine` (단일 goroutine 이벤트 루프)
  - 입력: 신규 주문 요청, 취소 요청
  - 출력: 주문 상태 변경, 체결 레코드, 이벤트 발행 요청
- `OrderBook` (in-memory)
  - bids/asks 구조 + order index
- `OrderRepository` (DB)
  - 주문 원장(상태/잔량 포함) 영속화
- `TradeRepository` (DB)
  - 체결 최소 레코드 영속화
- `EventPublisher`
  - `order.created`, `order.updated`, `trade.executed` 발행

핸들러는 직접 매칭하지 않고, 검증 후 `MatcherEngine`에 명령을 전달한다.

---

## 4. DB vs in-memory book 중 무엇을 어떻게 진실의 원천으로 둘지

결정:

- **실시간 매칭의 진실의 원천은 in-memory order book**
- **내구성/복구의 진실의 원천은 DB(order/trade journal)**

운영 원칙:

1. 매칭 판단(최우선 주문 선택, 잔량 차감, 제거)은 in-memory에서 수행
2. 결과 상태(주문/체결)는 DB에 순차 반영
3. 재기동 시 DB의 open/partially_filled 주문으로 in-memory book 재구성

즉, 런타임 성능은 메모리, 장애 복구는 DB를 사용한다.

---

## 5. 주문 생성 시 처리 순서

신규 limit 주문 처리 순서(결정, **동기 처리**):

1. 입력 검증
   - pair=`BTC-KRW`, side, price, quantity, 양수/스케일 유효성 확인
2. 주문 ID/타임스탬프 생성 및 결과 수신 채널(또는 future 유사 핸들) 준비
3. handler가 matcher goroutine에 `NewOrder` 명령 제출(enqueue)
4. handler는 결과 채널에서 **완료까지 대기**
5. matcher 루프가 매칭 시도 및 영속화 수행
   - 최초 주문 row INSERT (`status=open`, `remaining=quantity`)
   - 이후 매칭 실행
6. 매칭 결과를 DB에 반영
   - 주문 remaining/status UPDATE
   - 체결 발생 시 trade INSERT
7. DB 반영 성공 후 이벤트 발행
   - 주문 생성 시 `order.created`(기존 계약 유지)
   - 주문 상태 변경 건마다 `order.updated`
   - 체결 건마다 `trade.executed`
8. matcher가 최종 결과를 결과 채널로 반환
9. handler가 **taker 주문의 post-match 최종 상태**를 API 응답으로 반환

명시:

- Phase 3는 비동기 acceptance(접수만 하고 나중 처리) semantics를 도입하지 않는다.
- `POST /orders`는 matcher/persistence 완료 이후 응답하며, 응답 값은 taker 주문의 post-match 최종 상태다.

---

## 6. 매칭 루프 설계

단일 goroutine 루프에서 아래를 직렬 처리한다.

- `NewOrder` 처리
- `CancelOrder` 처리

`NewOrder` 알고리즘:

1. taker 주문(side)에 따라 반대편 최우선 price level 조회
2. 가격 교차 여부 확인
   - bid taker: best ask <= bid price
   - ask taker: best bid >= ask price
3. 교차 시 해당 level의 FIFO head(maker)와 체결
4. `fillQty = min(takerRemaining, makerRemaining)`
5. 양측 remaining 차감, 0이면 book에서 제거
6. 체결 레코드 생성(가격은 maker price)
7. taker remaining > 0 이고 더 이상 교차 불가면 자신의 side book에 삽입(FIFO tail)
8. 체결/상태 변경 결과를 한 번에 커밋 단위로 DB 반영 후 결과 채널 응답

루프는 단일 스레드 가정으로 락 없이 동작한다.

---

## 7. bid/ask 정렬 규칙과 price-time priority 유지 방식

정렬 규칙:

- bid: 높은 가격 우선
- ask: 낮은 가격 우선
- 동일 가격 내: 먼저 들어온 주문(FIFO) 우선

구조 결정:

- price level 컨테이너
  - bid: 내림차순 정렬 가능한 구조
  - ask: 오름차순 정렬 가능한 구조
- 각 price level 내부
  - linked list 또는 deque로 FIFO 유지
- `orderId -> node` 인덱스 유지
  - 취소 O(1) 제거를 위해 필요

price-time priority는 “가격 우선 + 동일가 도착순”으로 고정한다.

---

## 8. 부분 체결 / 완전 체결 / 잔량 처리 방식

상태 전이 규칙:

- 최초: `open`
- 일부만 체결: `partially_filled`
- 전량 체결: `filled`
- 사용자 취소: `cancelled`

처리 규칙:

- `remaining_quantity`를 단일 진실 필드로 사용
- 체결마다 remaining 감소
- remaining이 0이면 즉시 book 제거 + `filled`
- taker 미체결 잔량이 남고 교차 불가면 resting order로 book 적재

---

## 9. 취소 시 resting order 제거 방식

취소는 matcher 루프에서 직렬 처리한다.

1. DB에서 주문 소유자/상태/remaining 확인
2. 취소 가능 상태(`open`, `partially_filled`) + remaining>0 인지 확인
3. in-memory `orderId -> node`로 주문 노드 조회
4. level 큐에서 노드 제거, 빈 level이면 level 삭제
5. DB 상태 `cancelled` 및 `updated_at` 반영
6. `order.updated` 발행

취소 API 계약(고정):

- `open`/`partially_filled` + `remaining_quantity > 0`: 취소 성공(200 계열), book 제거 + DB `cancelled` + `order.updated`
- `filled` 또는 `cancelled`: **409 Conflict** 유지
- 주문 없음 또는 권한 없는 사용자: **404** 유지

---

## 10. 숫자 표현 방식

float 금지. 내부/DB/외부 표현은 아래로 고정한다.

- in-memory matcher
  - 가격: `int64` (KRW 최소단위 1원 기준)
  - 수량: `int64` (BTC를 `1e8` 스케일 satoshi 정수로 표현)
- DB persistence(고정: scaled integer 저장)
  - `price`: `BIGINT` (KRW minor unit)
  - `quantity`: `BIGINT` (`1e8` 스케일)
  - `remaining_quantity`: `BIGINT` (`1e8` 스케일)
- 외부 표현(REST/event): decimal string 직렬화
- notional 연산: in-memory에서만 `big.Int` 사용 가능, 필요 시 계산하며 Phase 3에서는 notional DB 저장을 강제하지 않음

비교/차감 규칙:

- 비교: 정수 비교(`>`, `<`, `==`)만 사용
- 차감: `a-b` 전 `a>=b` 보장, 음수 방지
- 파싱: API decimal string -> 스케일 정수 변환 시 자리수 검증
- 직렬화: 응답/이벤트에서는 decimal string으로 재변환

핵심은 **매칭 판단 및 remaining 계산 전 구간에서 부동소수점 배제**다.

---

## 11. 최소 trade persistence 설계

`trades` 최소 컬럼(결정):

- `trade_id` (PK)
- `pair` (`BTC-KRW`)
- `price` (`BIGINT`, KRW minor unit)
- `quantity` (`BIGINT`, `1e8` 스케일)
- `maker_order_id`
- `taker_order_id`
- `executed_at`

원칙:

- 체결 1건당 row 1개
- 부분 체결이 여러 번 발생하면 여러 row 저장
- 집계(OHLCV 등)는 Phase 4에서 소비자가 수행

---

## 12. 재기동 시 book rebuild 방식

부팅 순서:

1. matcher 루프 시작 전 DB에서 `pair=BTC-KRW` AND `status in (open, partially_filled)` 조회
2. `created_at ASC, id ASC`로 정렬하여 순차 삽입
3. 각 주문의 `remaining_quantity` 기준으로 level 재구성
4. 재구성 완료 후 matcher 명령 수신 시작

주의:

- 재기동 시 과거 trade를 재실행하지 않는다(중복 체결 방지)
- open 주문만 재적재하여 book 상태만 복구
- `created_at`만으로 정렬하면 동률 시 DB 반환 순서가 비결정적일 수 있으므로, `id ASC`를 안정적인 2차 키로 강제해 FIFO 재구성을 결정적으로 유지한다.

---

## 13. API 영향 범위

Phase 3에서 API는 최소 영향만 허용한다.

- 기존 `POST /orders` 응답의 `status`, `remaining_quantity`가 실제 매칭 결과를 반영할 수 있음
- 기존 `GET /orders/{id}`에서 상태 전이 결과 조회 가능
- `DELETE /orders/{id}`는 resting 주문 제거 결과를 반영

추가 엔드포인트(예: orderbook/trades marketdata REST)는 **이번 phase 범위 밖**이다.

---

## 14. 이벤트 영향 범위

Phase 3 이벤트 세트(확장, 대체 아님):

- `order.created` (기존, 유지)
- `order.updated` (기존, 유지)
- `trade.executed` (Phase 3 신규 추가)

발행 순서/단위(결정):

1. DB 쓰기(주문/체결) 성공
2. 그 결과를 기준으로 이벤트 발행

세부:

- 체결 발생 시 동일 매칭 step 내에서
  - 관련 주문 상태 업데이트 DB 반영
  - trade row INSERT
  - 이벤트 발행 시
    - `trade.executed`: **persisted trade row당 1회**
    - `order.updated`: **해당 매칭 step 종료 시 영향받은 주문당 1회(final state 기준)**
      - taker: step 내 여러 partial fill이 있어도 중간 상태를 여러 번 발행하지 않고 마지막 상태 1회 발행
      - maker: 체결에 참여해 상태/remaining이 바뀐 주문만 주문별 1회 발행
  - 위 기준으로 `order.updated`(final states) 후 `trade.executed`(per trade row) 순으로 발행
- 발행 실패는 로깅/재시도 정책 대상으로 두되, DB 상태를 롤백하지 않는다(Phase 3 단순화)

---

## 15. 테스트 전략

우선순위 테스트:

1. 단위 테스트 (matcher core)
   - price-time priority
   - 부분 체결/완전 체결
   - 동일 가격 FIFO
   - 교차 불가 시 resting 적재
2. 단위 테스트 (숫자)
   - decimal string 파싱/직렬화
   - 스케일 정수 비교/차감
3. 통합 테스트 (repository+matcher)
   - 주문 생성→체결→상태 반영
   - 취소→book 제거→상태 반영
   - 재기동 rebuild
4. 이벤트 테스트
   - DB 반영 이후 발행 순서 검증

테스트는 결정적 입력/출력 중심으로 작성한다.

---

## 16. 흔한 실수 / 리스크

- float 사용으로 잔량 0 비교 실패
- 동시성 욕심으로 락/고루틴 확장 후 비결정성 유입
- DB 반영 전 이벤트 선발행으로 불일치 발생
- 취소 시 book/DB 한쪽만 갱신되어 유령 주문 발생
- 재기동 시 정렬 기준 누락으로 FIFO 깨짐
- 범위 확장(수수료, 마켓오더, 멀티페어)로 Phase 3 일정 붕괴

---

## 17. 실제 구현 PR를 위한 구현 슬라이스 제안

작게 나눈 구현 순서:

1. **Slice A: 숫자 타입/변환 유틸 + 테스트**
   - price/qty 정수 스케일 확정
2. **Slice B: in-memory book 자료구조 + priority/FIFO 테스트**
3. **Slice C: matcher 루프(NewOrder) + 부분/완전체결 테스트**
4. **Slice D: cancel 경로 + orderId index 제거 테스트**
5. **Slice E: DB 연동(order/trade persistence) + 통합 테스트**
6. **Slice F: 이벤트 발행(order.updated, trade.executed) 순서 검증**
7. **Slice G: 재기동 rebuild + 회귀 테스트**

각 슬라이스는 독립 PR로 쪼개되, Phase 3 범위를 넘는 기능 추가는 금지한다.
