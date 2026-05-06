# marketdata-service

시세 데이터를 위한 mock 서비스.

Phase 4에서는 이벤트(`order.created`, `order.updated`, `trade.executed`)만으로
in-memory read model을 구축하고 REST를 제공한다.

## 범위 (Phase 4)

- 단일 페어: `BTC-KRW`
- read model: ticker, orderbook, recent trades, candles(1m/5m/1h)
- 저장소 영속화 없음 (in-memory)
- WebSocket 없음
- frontend trading terminal UI 없음
- 외부 시세 연동 없음

## API

| Method | Path | 설명 |
|---|---|---|
| GET | `/health` | Liveness probe |
| GET | `/ready` | Readiness probe |
| GET | `/marketdata/ticker/{pair}` | 시세 요약 |
| GET | `/marketdata/orderbook/{pair}?depth=20` | 호가 스냅샷 |
| GET | `/marketdata/candles/{pair}?interval=1m&limit=200` | OHLCV 캔들 |
| GET | `/marketdata/trades/{pair}?limit=50` | 최근 체결 |

지원 페어는 현재 `BTC-KRW`만이며, 다른 pair 요청은 `400`으로 거절한다.

### Contract 규칙

- 숫자 필드: decimal string
- 타임스탬프: RFC3339 UTC
- empty state: 모든 marketdata endpoint는 `200` + 빈 JSON 구조를 반환

### Validation 규칙

- unsupported pair: `400 Bad Request`
- unsupported interval: `400 Bad Request`
- `depth` / `limit`는 정수이며 `> 0` 이어야 함. 아니면 `400 Bad Request`
- bound:
  - `depth`: 기본 20, 최대 50
  - `trades limit`: 기본 50, 최대 200
  - `candles limit`: 기본 200, 최대 500

### 응답 예시

`GET /marketdata/ticker/BTC-KRW`

```json
{
  "pair": "BTC-KRW",
  "last_price": "100000",
  "change_rate_24h": "0.01234567",
  "high_24h": "101000",
  "low_24h": "98000",
  "volume_24h": "12.3456789",
  "as_of": "2026-04-14T12:00:00Z"
}
```

`GET /marketdata/orderbook/BTC-KRW?depth=20`

```json
{
  "pair": "BTC-KRW",
  "depth": 20,
  "bids": [{"price":"99900","quantity":"1.25"}],
  "asks": [{"price":"100100","quantity":"0.75"}],
  "as_of": "2026-04-14T12:00:00Z"
}
```

`GET /marketdata/candles/BTC-KRW?interval=1m&limit=200`

```json
{
  "pair": "BTC-KRW",
  "interval": "1m",
  "candles": [
    {
      "timestamp": "2026-04-14T12:00:00Z",
      "open": "100000",
      "high": "100500",
      "low": "99900",
      "close": "100300",
      "volume": "1.25"
    }
  ]
}
```

`GET /marketdata/trades/BTC-KRW?limit=50`

```json
{
  "pair": "BTC-KRW",
  "trades": [
    {
      "trade_id": "trade-1",
      "pair": "BTC-KRW",
      "price": "100000",
      "quantity": "0.5",
      "maker_order_id": "o-1",
      "taker_order_id": "o-2",
      "executed_at": "2026-04-14T12:00:01Z"
    }
  ]
}
```

## 이벤트 소비

- 이벤트 버스: **NATS JetStream**
- 구독 subject: `order.*`, `trade.*`
- 사용 이벤트:
  - `order.created`
  - `order.updated`
  - `trade.executed`
- read model 갱신 규칙:
  - `order.created` / `order.updated` -> orderbook depth 재집계
  - `trade.executed` -> ticker, recent trades, candles, 24h summary

## 환경변수

| 변수 | 기본값 | 설명 |
|---|---|---|
| PORT | 8083 | 서버 포트 |
| DEV_MODE | false | 개발 모드 |
| EVENT_BUS_ENABLED | false | true면 NATS consumer 활성화 |
| NATS_URL | - | EVENT_BUS_ENABLED=true 일 때 필수 |
| NATS_STREAM | ORDERS | JetStream stream 이름 |
| NATS_DURABLE | marketdata-consumer | durable consumer 이름 |

## 빌드/테스트

```bash
go test -v ./...
go build -o marketdata-service .
docker build -t marketdata-service .
```
