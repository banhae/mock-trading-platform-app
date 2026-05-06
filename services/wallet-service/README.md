# wallet-service

잔고 관리를 위한 mock 서비스. 현재 phase 에서는 health/ready 엔드포인트와
**Phase 2 이벤트 consumer wiring** 만 가진다.

향후 phase 에서 order.* / trade.executed 이벤트를 소비하여 mock 잔고 hold/release
/ balance 이동을 구현할 예정이다. Phase 2 단계에서는 balance 로직이 스코프 밖이다.

## API

| Method | Path    | 설명            |
|--------|---------|-----------------|
| GET    | /health | Liveness probe  |
| GET    | /ready  | Readiness probe |

아직 `/wallet/*` 경로는 제공하지 않는다.

## 이벤트 소비 (Phase 2)

- 이벤트 버스: **NATS JetStream**
- 구독 subject: `order.*`
  - `order.created`
  - `order.updated`
- durable consumer 이름: 기본값 `wallet-consumer`
- 현재 동작:
  - envelope 파싱
  - 구조화된 로그 출력
  - in-memory 카운터 (`Created`, `Updated`, `Unknown`, `Failed`) 증가
- 현재 **하지 않는 것**:
  - 잔고 hold / release / 이동
  - `GET /wallet/balance` 같은 API 노출
  - 영속 저장

### no-op 모드

`EVENT_BUS_ENABLED=false` 이면 NATS 연결을 시도하지 않고 no-op 모드로 기동한다.
이렇게 하면 NATS 가 없는 로컬 환경에서도 서비스가 정상적으로 시작된다.

## 환경변수

| 변수               | 기본값            | 설명                                       |
|--------------------|-------------------|--------------------------------------------|
| PORT               | 8082              | 서버 포트                                  |
| DEV_MODE           | false             | 개발 모드                                  |
| EVENT_BUS_ENABLED  | false             | true 면 NATS JetStream consumer 활성화      |
| NATS_URL           | -                 | EVENT_BUS_ENABLED=true 일 때 필수           |
| NATS_STREAM        | ORDERS            | JetStream stream 이름                      |
| NATS_DURABLE       | wallet-consumer   | JetStream durable consumer 이름            |

## 빌드 및 테스트

```bash
go test -v ./...
go build -o wallet-service .
docker build -t wallet-service .
```
