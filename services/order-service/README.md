# order-service

코인 거래소 mock 시스템의 핵심 서비스. 주문 생성 및 조회 API를 제공한다.

## API

| Method | Path                          | 설명                           | 인증                            |
|--------|-------------------------------|--------------------------------|---------------------------------|
| POST   | /orders                       | 주문 생성                      | 필요 (DEV_MODE=true면 스킵)     |
| GET    | /orders?status=&limit=        | 현재 사용자의 주문 목록        | 필요 (DEV_MODE=true면 스킵)     |
| GET    | /orders/{id}                  | 주문 조회 (본인 소유만)        | 필요 (DEV_MODE=true면 스킵)     |
| DELETE | /orders/{id}                  | 주문 취소 (본인 소유만)        | 필요 (DEV_MODE=true면 스킵)     |
| GET    | /health                       | Liveness probe                 | 불필요                          |
| GET    | /ready                        | Readiness probe                | 불필요                          |

### 주문 상태 (status)

| 값                  | 의미                                          |
|---------------------|-----------------------------------------------|
| `open`              | 접수된 주문. 취소 가능.                        |
| `partially_filled`  | 일부 체결 (Phase 3 matcher 이후). 취소 가능.   |
| `filled`            | 전량 체결 (종결). 취소 불가.                   |
| `cancelled`         | 취소 (종결). 재취소 불가.                      |

현재 `POST /orders` 는 order-service 내부 in-memory matcher 경로로 연결되어
요청 시점에 즉시 매칭을 수행한다. 따라서 주문 생성 직후 상태는 `open` 뿐 아니라
`partially_filled` / `filled` 로도 응답될 수 있다.

`DELETE /orders/{id}` 는 `open`, `partially_filled` 상태에서만 취소 가능하며,
성공 시 matcher book 에서도 함께 제거된다.

### 사용자 scoping

- `GET /orders` 는 반드시 JWT(또는 DEV_MODE 의 `dev-user`) 로 식별된 현재 사용자의
  주문만 반환한다. `user` query param 은 public API 에 존재하지 않는다.
- `GET /orders/{id}` 는 타 사용자 주문에 대해 존재를 감추기 위해 404 를 반환한다.
- `DELETE /orders/{id}` 는 타 사용자 주문에 대해 404, 종결 상태 주문에 대해 409 를
  반환한다.

### 숫자 규칙

가격(`price`), 수량(`quantity`, `remaining_quantity`) 은 전부 **decimal string**
으로 주고받는다. float 로 변환하지 않는다.

## 환경변수

| 이름              | 필수   | 기본값   | 설명                                              |
|-------------------|--------|----------|---------------------------------------------------|
| PORT              | 아니오 | 8080     | HTTP 서버 포트                                    |
| DATABASE_URL      | 예     | -        | PostgreSQL 접속 URL                               |
| JWT_SECRET        | 조건부 | -        | JWT 검증 키. DEV_MODE=false일 때 필수             |
| DEV_MODE          | 아니오 | false    | true면 JWT 스킵 + 테이블 자동 생성                |
| EVENT_BUS_ENABLED | 아니오 | false    | true면 NATS JetStream publisher 활성화            |
| NATS_URL          | 조건부 | -        | EVENT_BUS_ENABLED=true 일 때 필수 (예: `nats://nats:4222`) |
| NATS_STREAM       | 아니오 | ORDERS   | JetStream stream 이름                             |

## 이벤트 발행 (Phase 2)

Phase 2 에서 order-service 는 주문 lifecycle 이벤트를 NATS JetStream 으로 발행한다.

| Subject         | 발행 시점                        |
|-----------------|----------------------------------|
| `order.created` | 주문 생성 row insert 성공 직후     |
| `order.updated` | 주문 상태 전이 후 (match/cancel)   |
| `trade.executed` | trade row insert 성공 후          |

이벤트 envelope 은 아래 모양을 가진다. 숫자 필드는 모두 decimal string 이며 float
로 변환되지 않는다.

```json
{
  "type": "order.created",
  "version": 1,
  "occurred_at": "2026-04-13T00:00:00Z",
  "order": {
    "id": "...",
    "user_id": "...",
    "pair": "BTC-KRW",
    "side": "buy",
    "quantity": "0.5",
    "remaining_quantity": "0.5",
    "price": "50000",
    "status": "open",
    "created_at": "2026-04-13T00:00:00Z",
    "updated_at": "2026-04-13T00:00:00Z"
  }
}
```

### 발행 정책

- DB 가 authoritative 하다. insert/update 가 성공하면 HTTP 응답은 성공으로 확정된다.
- 이벤트 publish 는 **best-effort** 이다. publish 실패는 구조화된 로그 (`error`
  level) 로 남기지만 HTTP 응답을 실패로 만들지 않는다.
- 재시도, transactional outbox, exactly-once 보장은 Phase 2 범위 밖이다.
- `EVENT_BUS_ENABLED=false` 이면 no-op publisher 를 쓰며 NATS 의존성 없이도 로컬
  개발이 가능하다.

stream 은 publisher 가 자동으로 `ORDERS` (subjects=`order.>`) 형태로 생성 시도
한다. 이미 operator 가 다른 설정으로 stream 을 프로비저닝해 두었다면 publisher 는
경고 로그만 남기고 그대로 사용한다.

## 런타임 matcher / 재기동 복구

- 단일 pair `BTC-KRW` / limit order만 지원한다.
- in-memory order book 이 primary matcher 상태이며 price-time priority 로 동작한다.
- **수평 확장(복수 active replica)은 지원하지 않는다.** matcher runtime 활성 상태에서는
  단일 active `order-service` 인스턴스만 운영해야 한다.
- 서비스 시작 시 DB의 `open`, `partially_filled` 주문만 읽어 book 을 복원한다.
- `filled`, `cancelled` 주문은 재기동 시 book 에 올리지 않는다.
- 복구 실패 시 서버는 기동하지 않고 종료한다(조용한 degraded 상태 금지).

## 빌드

```bash
cd services/order-service
go build -o order-service .
```

## 테스트

```bash
cd services/order-service
go test -v ./...
```

## 로컬 실행 (dev mode)

PostgreSQL이 실행 중이어야 한다.

```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/orders?sslmode=disable"
export DEV_MODE=true
go run .
```

## Docker

```bash
docker build -t order-service .
docker run -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/orders?sslmode=disable" \
  -e DEV_MODE=true \
  order-service
```

## 요청 예시

주문 생성:
```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"pair":"BTC-KRW","side":"buy","quantity":"0.5","price":"50000"}'
```

주문 목록 (현재 사용자):
```bash
curl "http://localhost:8080/orders?status=open&limit=20"
```

주문 조회:
```bash
curl http://localhost:8080/orders/{id}
```

주문 취소:
```bash
curl -X DELETE http://localhost:8080/orders/{id}
```

헬스 체크:
```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## 다음 단계

- 최소 matching engine (Phase 3 — `partially_filled` / `filled` 자동 전이, `trade.executed` 이벤트)
- DB 마이그레이션 도구 분리
