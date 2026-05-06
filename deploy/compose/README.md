# deploy/compose — 로컬 데모 스택

> **범위**: 로컬 데모 / 개발 환경 smoke 전용.
> 정식 배포 경로는 EKS 위의 Helm + ArgoCD 입니다
> ([`charts/`](../../charts/) 와
> [`exchange-gitops`](https://github.com/banhae/mock-trading-platform-gitops) 레포 참고).
> 이 compose 스택은 지원되는 배포 경로가 **아니며** CI 에서 검증되지 않습니다.

## 실행되는 구성

EKS 배포와 동일한 서비스 토폴로지를 단일 호스트에서 흉내내며,
플랫폼 계층(Helm, ArgoCD, observability 스택, External Secrets, ALB)은
제외되어 있습니다.

```
browser ──▶ frontend (nginx :8080)
              ├─ /api/auth/*   ──▶ auth-service       :8081
              ├─ /api/orders*  ──▶ order-service      :8080 ──▶ postgres
              ├─ /api/wallet/* ──▶ wallet-service     :8082
              └─ /api/market/* ──▶ marketdata-service :8083
                                      ▲
                          NATS JetStream (ORDERS stream)
                                      ▲
                            order-service 가 publish
```

이 디렉터리의 nginx 설정은 클러스터 내 `mock-trading-platform-ingress` chart 역할을
대체합니다. 그 결과 브라우저는 운영 환경과 동일하게 frontend origin 으로만
same-origin 호출을 하게 됩니다.

## 실행

```bash
# 레포 루트에서
cp deploy/compose/.env.example deploy/compose/.env
docker compose -f deploy/compose/docker-compose.demo.yml \
  --env-file deploy/compose/.env up --build
```

이후 <http://localhost:8080> 접속.

최초 부팅은 1 분 정도 걸립니다 (Go 빌드 + npm ci + Vite 빌드).
이후에는 빌드 캐시를 재사용합니다.

## 종료

```bash
docker compose -f deploy/compose/docker-compose.demo.yml down -v
```

`-v` 플래그는 postgres / nats 볼륨까지 삭제합니다. 데모 주문을 다음 실행에
유지하고 싶다면 `-v` 를 빼고 실행하세요.

## 의도적으로 포함하지 않는 것

- Prometheus / Grafana / Loki — observability 는 gitops 레포의 책임입니다.
- ALB / Ingress controller — nginx proxy stanza 로 대체됩니다.
- ArgoCD / Helm — 단일 호스트 데모에는 필요 없습니다.
- TLS — 로컬 전용 스택이라 평문 HTTP 만 사용합니다.
- 시크릿 관리 — `.env.example` 만 사용하며 AWS Secrets Manager 연동은 없습니다.

## 알아둘 dev-mode 단축 동작

- 모든 Go 서비스에 `DEV_MODE=true` 가 설정됩니다. 그 결과:
  - `order-service` 가 부팅 시 `orders` / `trades` 테이블을 자동 생성합니다
    (별도 마이그레이션 도구를 거치지 않습니다).
  - `auth-service` 는 `JWT_SECRET` 미설정 시 `dev-secret` 으로 기본값을
    채웁니다.
- NATS `ORDERS` 스트림은 `order-service` publisher 가 최초 부팅 시점에
  자동 생성합니다. 별도 init container 나 사이드카가 필요 없습니다.

## 동작 확인

- `docker compose ps` — 각 서비스 상태 확인.
- `docker compose logs -f order-service` — 주문 생성 → 매칭 → trade
  이벤트가 NATS 로 흘러가는 흐름을 로그에서 확인.
- `GET /api/market/orderbook/BTC-KRW` — `POST /api/orders` 로 주문을 몇
  건 만든 뒤 호출하면 in-memory read model 의 호가창 응답을 확인할 수
  있습니다.
