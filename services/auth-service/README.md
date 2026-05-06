# auth-service

코인 거래소 mock 시스템의 인증 서비스. Mock 유저 로그인 및 JWT 발급을 담당한다.

## API

| Method | Path     | 설명             | 인증   |
|--------|----------|------------------|--------|
| POST   | /login   | 로그인 및 JWT 발급 | 불필요 |
| GET    | /health  | Liveness probe   | 불필요 |
| GET    | /ready   | Readiness probe  | 불필요 |

### POST /login

**요청:**
```json
{
  "username": "alice",
  "password": "password1"
}
```

**응답 (200):**
```json
{
  "access_token": "eyJhbGci...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

**에러 응답:**
- `400` — 요청 형식 오류 또는 필수 필드 누락
- `401` — 잘못된 자격 증명

## Mock 유저

| Username | Password  | User ID |
|----------|-----------|---------|
| alice    | password1 | user-1  |
| bob      | password2 | user-2  |
| dev      | dev       | dev-user (DEV_MODE=true일 때만) |

## 환경변수

| 이름          | 필수   | 기본값 | 설명                                         |
|---------------|--------|--------|----------------------------------------------|
| PORT          | 아니오 | 8081   | HTTP 서버 포트                               |
| JWT_SECRET    | 조건부 | -      | JWT 서명 키. DEV_MODE=false일 때 필수        |
| TOKEN_EXPIRY  | 아니오 | 1h     | 토큰 만료 시간 (Go duration: 30m, 1h, 24h)  |
| DEV_MODE      | 아니오 | false  | true면 dev/dev 로그인 허용 + 기본 secret 사용 |

## 빌드

```bash
cd services/auth-service
go build -o auth-service .
```

## 테스트

```bash
cd services/auth-service
go test -v ./...
```

## 로컬 실행

```bash
export DEV_MODE=true
go run .
```

## Docker

```bash
docker build -t auth-service .
docker run -p 8081:8081 -e DEV_MODE=true auth-service
```

## 사용 예시

### 로그인 (dev mode)

```bash
curl -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"dev","password":"dev"}'
```

### 로그인 (mock 유저)

```bash
curl -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"password1"}'
```

## order-service와의 JWT 연동

auth-service가 발급한 토큰을 order-service에서 그대로 사용할 수 있다.

**조건:**
- 두 서비스에 동일한 `JWT_SECRET` 환경변수를 주입해야 한다.
- auth-service는 HMAC-SHA256으로 서명하고, `sub` 클레임에 user_id를 넣는다.
- order-service의 `middleware.go`는 같은 방식으로 토큰을 검증하고 `sub`에서 user_id를 추출한다.

**예시 흐름:**

```bash
# 1. auth-service에서 토큰 발급
TOKEN=$(curl -s -X POST http://localhost:8081/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"password1"}' | jq -r '.access_token')

# 2. order-service에 토큰으로 주문 생성
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"pair":"BTC/USDT","side":"buy","quantity":"0.5","price":"50000"}'
```

**Kubernetes에서:**
- 동일한 Secret 리소스에서 `JWT_SECRET`을 양쪽 Deployment에 주입한다.
- 또는 Helm install 시 `--set env.JWT_SECRET=<값>`을 양쪽에 동일하게 전달한다.

## 다음 단계

- Kubernetes Secret으로 JWT_SECRET 관리
- api-gateway에서 인증 전달 구조 도입
- 토큰 갱신 (refresh token) 지원
