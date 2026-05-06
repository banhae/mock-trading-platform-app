# FRONTEND_PLAN

exchange mock system에 프론트엔드를 추가하기 위한 **최종 수정 구현 계획**.
이 문서는 `mock-trading-platform-app` 리포에 위치하지만, 작업 범위는 `mock-trading-platform-app` /
`mock-trading-platform-gitops` / `mock-trading-platform-infra` 3개 리포 모두를 포함한다.

이 버전은 다음 두 기준을 동시에 만족하도록 정리했다.

1. 현재 리포의 실제 패턴과 충돌하지 않을 것
2. 브라우저 직접 호출, 포트 혼선, CI 낭비, 문서 책임 경계 혼선을 줄일 것

---

## 1. 목적과 범위

### 목적

- 기존 Go 마이크로서비스(`order-service`, `auth-service`, `wallet-service`,
  `marketdata-service`)에 대응하는 **최소 프론트엔드**를 EKS 위에 배포한다.
- 학습용 mock 시스템이라는 `SYSTEM_BLUEPRINT.md`의 원칙을 지키기 위해
  **엔터프라이즈급 프레임워크 및 상태관리 도입은 피한다.**
- 프론트엔드는 기존 배포/GitOps 파이프라인(동일 CI → ECR → ArgoCD → EKS)에
  **기존 서비스와 동일한 패턴으로** 합류해야 한다.
- 브라우저가 `auth-service` / `order-service`를 직접 호출하는 구조는 피하고,
  **frontend nginx를 통한 동일 origin reverse proxy** 로 연결한다.

### 이 phase에서 하는 것

- `services/frontend/` 디렉터리에 SPA 소스 + Dockerfile + nginx proxy 설정
- `charts/frontend/` Helm chart
- mock-trading-platform-app `ci.yaml`에 `ci-frontend` job 추가
- mock-trading-platform-gitops에 `frontend` Application + dev values 추가
- mock-trading-platform-infra `ecr.tf`에 `frontend` ECR 리포 추가

### 이 phase에서 하지 않는 것

- **별도 `api-gateway` 서비스 추가**
- Ingress / ALB 공개
- ACM / Route53 / 커스텀 도메인
- Next.js SSR
- 상태관리 라이브러리(Redux, Zustand 등)
- i18n, 테마, 디자인 시스템, Storybook
- E2E 테스트(Playwright/Cypress)

### 책임 경계에 대한 명시

이번 phase에서는 별도 `api-gateway` 서비스를 만들지 않는다.
대신 **frontend 컨테이너의 nginx가 dev 검증용으로 제한된 reverse proxy 책임을 임시 흡수**한다.

이것은 영구 구조가 아니다.
다음 `api-gateway` phase의 명시적 작업은 아래와 같다.

- frontend nginx에서 API routing 책임 제거
- 동일 public path를 전용 gateway로 이관
- frontend는 정적 파일 서빙만 담당하도록 단순화

즉, 이번 phase의 프록시 책임은 **임시적이며 제거 대상**이다.

---

## 2. 기술 선택

### 2.1 스택

| 항목 | 선택 | 근거 |
|------|------|------|
| 프레임워크 | **Vite + React + TypeScript** | SSR 불필요, 정적 빌드 산출물 배포가 단순함 |
| 라우팅 | React Router | SPA 최소 요건 |
| HTTP 클라이언트 | `fetch` (native) | 의존성 최소화 |
| 테스트 | Vitest | Vite 기본 조합, 빠름 |
| 런타임 이미지 | `nginx:1.27-alpine` | 정적 서빙 + reverse proxy 가능 |

**Next.js를 선택하지 않은 이유**:
SSR을 쓰면 런타임 Node 프로세스가 생기고 이미지/chart/probe 설계가 커진다.
현재 phase에서는 요구되지 않는다.

### 2.2 public API path 원칙

프론트가 호출하는 public path는 이번 phase와 이후 전용 gateway phase에서
가능한 한 **동일하게 유지**한다.

이번 phase의 public path는 아래로 고정한다.

- `POST /api/auth/login`
- `GET /api/auth/health`
- `GET /api/auth/ready`
- `POST /api/orders`
- `GET /api/orders/{id}`
- `GET /api/orders/health`
- `GET /api/orders/ready`

즉, frontend 코드는 항상 위 경로만 본다.
현재는 frontend nginx가 이를 backend service로 프록시하고,
나중에는 같은 public path를 전용 `api-gateway`가 이어받는다.

### 2.3 API 연결 전략

이번 phase에서 브라우저는 backend service를 직접 호출하지 않는다.
대신 frontend 컨테이너의 nginx가 아래 public path를 reverse proxy 한다.

- `/api/auth/*` → `auth-service`
- `/api/orders*` → `order-service`

브라우저는 항상 **같은 origin** 만 호출한다.
예:

- `POST /api/auth/login`
- `POST /api/orders`
- `GET /api/orders/{id}`

이 방식의 장점:

1. **CORS 문제를 회피**할 수 있다.
2. GitOps values에 브라우저용 `localhost` URL을 넣지 않아도 된다.
3. 프론트 배포 성공과 API 연동 실패 원인을 분리하기 쉽다.
4. 나중에 전용 `api-gateway`가 생겨도 frontend 코드를 거의 바꾸지 않는다.

### 2.4 nginx upstream 주입 전략

frontend nginx는 런타임 시 아래 upstream 값을 받아 config template을 렌더링한다.

- auth upstream: `http://mock-trading-platform-dev-auth-service.mock-trading-platform-dev.svc.cluster.local:8081`
- order upstream: `http://mock-trading-platform-dev-order-service.mock-trading-platform-dev.svc.cluster.local:8080`

주의:

- 이 값들은 **브라우저가 아니라 nginx가 사용하는 내부 upstream** 이다.
- 따라서 클러스터 내부 DNS를 그대로 써도 된다.
- 브라우저는 오직 frontend service만 본다.

---

## 3. 리포별 변경 계획

### 3.1 mock-trading-platform-app

#### 3.1.1 신규 디렉터리: `services/frontend/`

```text
services/frontend/
├── Dockerfile
├── README.md
├── package.json
├── package-lock.json
├── tsconfig.json
├── vite.config.ts
├── index.html
├── nginx.conf.template
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── routes/
│   │   ├── LoginPage.tsx
│   │   ├── OrdersPage.tsx
│   │   └── HealthPage.tsx
│   ├── api/
│   │   ├── client.ts
│   │   ├── auth.ts
│   │   └── orders.ts
│   ├── lib/
│   │   └── token.ts
│   └── __tests__/
│       └── client.test.ts
└── .dockerignore
```

#### 3.1.2 라우트/페이지 최소 범위

- `LoginPage`
  - username/password 입력
  - `/api/auth/login` 호출
  - access token을 `sessionStorage` 저장

- `OrdersPage`
  - 주문 생성 폼
  - 주문 ID 입력 후 조회
  - `Authorization: Bearer <token>` 헤더로 `/api/orders`, `/api/orders/{id}` 호출

- `HealthPage`
  - frontend 자체 상태 표시
  - 선택적으로 아래 경로를 호출하여 프록시 상태 확인
    - `/api/auth/health`
    - `/api/auth/ready`
    - `/api/orders/health`
    - `/api/orders/ready`

주의:

- 현재 backend는 `/health`, `/ready`를 각각 제공한다.
- 따라서 HealthPage는 `"/health/ready"` 같은 단일 경로가 아니라
  **각 엔드포인트를 개별적으로** 다룬다.

#### 3.1.3 API client 원칙

프론트 코드에서 backend 절대 URL은 사용하지 않는다.
모든 호출은 상대 경로를 사용한다.

예:

```ts
await fetch('/api/auth/login', { ... })
await fetch('/api/orders', { ... })
await fetch(`/api/orders/${id}`, { ... })
```

이 원칙을 지키면 환경별 URL 분기가 거의 사라진다.

#### 3.1.4 Dockerfile 구조

1. `FROM node:20-alpine AS builder`
   - `npm ci`
   - `npm run build`
   - `dist/` 생성

2. `FROM nginx:1.27-alpine`
   - `dist/` 복사
   - `nginx.conf.template` 를 `/etc/nginx/templates/default.conf.template` 로 복사
   - `NGINX_ENVSUBST_FILTER` 지정
   - 공식 nginx entrypoint의 내장 `envsubst` 메커니즘 사용

즉, **커스텀 `docker-entrypoint.d/*.sh` 스크립트는 두지 않는다.**

예시:

```dockerfile
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
ENV NGINX_ENVSUBST_FILTER="AUTH_UPSTREAM|ORDER_UPSTREAM"
```

#### 3.1.5 nginx 설정 원칙

- `/` → SPA fallback (`index.html`)
- `index.html`은 no-cache 권장
- 정적 자산은 캐시 가능
- API routing은 명시적 경로 매핑으로 구현
- trailing slash / prefix 제거 동작을 암묵적으로 믿지 말고,
  **backend path에 맞는 명시적 mapping** 을 쓴다

예시:

```nginx
server {
  listen 80;
  server_name _;

  root /usr/share/nginx/html;
  index index.html;

  location = /api/auth/login {
    proxy_pass ${AUTH_UPSTREAM}/login;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }

  location = /api/auth/health {
    proxy_pass ${AUTH_UPSTREAM}/health;
    proxy_set_header Host $host;
  }

  location = /api/auth/ready {
    proxy_pass ${AUTH_UPSTREAM}/ready;
    proxy_set_header Host $host;
  }

  location = /api/orders {
    proxy_pass ${ORDER_UPSTREAM}/orders;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }

  location ~ ^/api/orders/(.+)$ {
    proxy_pass ${ORDER_UPSTREAM}/orders/$1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  }

  location = /api/orders/health {
    proxy_pass ${ORDER_UPSTREAM}/health;
    proxy_set_header Host $host;
  }

  location = /api/orders/ready {
    proxy_pass ${ORDER_UPSTREAM}/ready;
    proxy_set_header Host $host;
  }

  location / {
    try_files $uri $uri/ /index.html;
  }
}
```

핵심은 다음이다.

- `/api/auth/login` → auth-service `/login`
- `/api/orders` → order-service `/orders`
- `/api/orders/{id}` → order-service `/orders/{id}`

즉, public path와 backend 실제 path가 다르므로 rewrite를 명시적으로 제어한다.

#### 3.1.6 신규 디렉터리: `charts/frontend/`

기존 `charts/order-service/`를 참고하되, 포트 구조와 values namespace는 그대로 복제하지 않는다.
현재 chart 템플릿은 `service.port`를 container port와 service targetPort에 같이 쓰는 구조이므로,
frontend는 아래처럼 값을 **분리**해야 한다.

```text
charts/frontend/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── _helpers.tpl
    ├── deployment.yaml
    └── service.yaml
```

`values.yaml` 핵심:

```yaml
replicaCount: 1

image:
  repository: mock-trading-platform/frontend
  tag: latest
  pullPolicy: IfNotPresent

container:
  port: 80

service:
  type: ClusterIP
  port: 8084
  targetPort: 80

env: {}

nginx:
  upstreams:
    auth: ""
    order: ""

resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 100m
    memory: 128Mi

probes:
  liveness:
    path: /
    port: 80
    initialDelaySeconds: 5
    periodSeconds: 10
  readiness:
    path: /
    port: 80
    initialDelaySeconds: 2
    periodSeconds: 5
```

설계 원칙:

- `env:` 는 **범용 애플리케이션 런타임 env** 용으로 비워 둔다
- nginx template 치환용 값은 `nginx.upstreams.*` 로 별도 네임스페이스 분리
- deployment 템플릿에서만 `AUTH_UPSTREAM`, `ORDER_UPSTREAM` 로 명시 매핑

`deployment.yaml` 주의:

- `containerPort: {{ .Values.container.port }}`
- `AUTH_UPSTREAM` 는 `.Values.nginx.upstreams.auth` 에서 주입
- `ORDER_UPSTREAM` 는 `.Values.nginx.upstreams.order` 에서 주입
- `NGINX_ENVSUBST_FILTER` 는 고정값으로 주입
- `env:` 블록은 별도 range로 범용 env만 추가
- probe는 `/`

예시 개념:

```yaml
env:
  - name: AUTH_UPSTREAM
    value: {{ .Values.nginx.upstreams.auth | quote }}
  - name: ORDER_UPSTREAM
    value: {{ .Values.nginx.upstreams.order | quote }}
  - name: NGINX_ENVSUBST_FILTER
    value: "AUTH_UPSTREAM|ORDER_UPSTREAM"
  {{- range $key, $value := .Values.env }}
  - name: {{ $key }}
    value: {{ $value | quote }}
  {{- end }}
```

`service.yaml` 주의:

- `port: {{ .Values.service.port }}`
- `targetPort: {{ .Values.service.targetPort }}`

즉 frontend chart는 현재 order-service chart와 비슷하지만,
**port 참조 방식과 nginx 관련 values namespace는 그대로 복사하면 안 된다.**

#### 3.1.7 수정: `.github/workflows/ci.yaml`

기존 CI 구조에 `frontend`를 추가한다.
현재 CI는 아래 원칙을 갖는다.

- workflow는 `services/**`, `charts/**`, `ci.yaml` 변경 시 트리거된다
- service job은 `detect-changes` 결과에 따라 실행/스킵된다
- chart-only 변경이면 service job은 스킵되고 `lint-charts`만 돈다

frontend도 이 패턴을 **그대로 따른다.**

핵심 수정:

- `detect-changes.outputs.frontend` 추가
- `dorny/paths-filter` 에 `frontend` 추가
- `frontend` filter는 **`services/frontend/**` 만 포함**
- `ci-frontend` job 추가
- `lint-charts`에 `helm lint charts/frontend` 추가

즉, `charts/frontend/**` 변경만으로 `ci-frontend`가 실행되지는 않게 한다.
chart-only 변경은 기존 서비스들과 동일하게 `lint-charts`가 전담한다.

예시:

```yaml
frontend: ${{ steps.set-all.outputs.all == 'true' || steps.filter.outputs.frontend == 'true' }}
```

```yaml
frontend:
  - 'services/frontend/**'
```

`ci-frontend` 예시:

```yaml
ci-frontend:
  needs: detect-changes
  if: needs.detect-changes.outputs.frontend == 'true'
  runs-on: ubuntu-latest
  permissions:
    contents: read
    id-token: write
  env:
    SERVICE: frontend
    SERVICE_DIR: services/frontend
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with:
        node-version: 20
        cache: npm
        cache-dependency-path: services/frontend/package-lock.json
    - name: Install deps
      run: cd $SERVICE_DIR && npm ci
    - name: Run tests
      run: cd $SERVICE_DIR && npm test -- --run
    - name: Build
      run: cd $SERVICE_DIR && npm run build
    - name: Build Docker image
      run: docker build -t $SERVICE:ci $SERVICE_DIR
    # 이후 ECR push는 기존 서비스와 동일 패턴
```

#### 3.1.8 수정: `README.md`

- 서비스 테이블에 `frontend` 추가
- 포트 8084 추가
- `docker build -t frontend services/frontend` 예시 추가
- `kubectl port-forward svc/mock-trading-platform-dev-frontend 8084:8084` 예시 추가
- backend는 frontend proxy를 통해 간접 호출한다고 명시

#### 3.1.9 수정: `SYSTEM_BLUEPRINT.md`

frontend 설명 추가:

- 역할: mock UI 제공
- auth-service / order-service 흐름 데모
- JWT 저장 및 동일 origin 호출
- dev 단계에서는 frontend nginx가 임시 reverse proxy 역할 수행
- 다음 `api-gateway` phase에서 이 책임 제거 예정

---

### 3.2 mock-trading-platform-gitops

#### 3.2.1 신규: `argocd/applications/dev/frontend.yaml`

`order-service.yaml`을 참고하여 작성:

- `metadata.name`: `mock-trading-platform-dev-frontend`
- `sources[0].path`: `charts/frontend`
- `helm.valueFiles`: `$values/environments/dev/values/frontend.yaml`
- `sync-wave`: `"4"`

이유:
frontend는 auth/order 뒤에서 동작하는 것이 자연스럽다.

#### 3.2.2 신규: `environments/dev/values/frontend.yaml`

주의:

- 여기에는 브라우저용 URL이 아니라 **nginx upstream** 을 넣는다.
- 값은 `env:` 가 아니라 `nginx.upstreams.*` 아래에 둔다.

```yaml
image:
  repository: <ACTUAL_AWS_ACCOUNT_ID>.dkr.ecr.ap-northeast-2.amazonaws.com/mock-trading-platform/frontend

nginx:
  upstreams:
    auth: http://mock-trading-platform-dev-auth-service.mock-trading-platform-dev.svc.cluster.local:8081
    order: http://mock-trading-platform-dev-order-service.mock-trading-platform-dev.svc.cluster.local:8080
```

이 값은 frontend pod 내부 nginx가 참조하므로 `localhost`가 아니다.

#### 3.2.3 수정 없음

- `argocd/projects/mock-trading-platform-dev.yaml`: 기존 sourceRepo/destination으로 충분
- `argocd/root-app.yaml`: `argocd/applications/dev` 바로 아래 파일 자동 감지 가능

---

### 3.3 mock-trading-platform-infra

#### 3.3.1 수정: `envs/dev/ecr.tf`

`ecr_repositories`에 `frontend` 추가:

```hcl
locals {
  ecr_repositories = toset([
    "api-gateway",
    "auth-service",
    "order-service",
    "wallet-service",
    "marketdata-service",
    "frontend",
  ])
}
```

다른 변경은 필요 없다.

---

## 4. 작업 순서 (Phase 분할)

### Phase F1 — 인프라 준비 (mock-trading-platform-infra)

1. `ecr.tf`에 `frontend` 추가
2. `terraform plan`
3. `terraform apply`
4. 검증: frontend ECR repo 생성 확인

### Phase F2 — 프론트엔드 소스 스캐폴드 (mock-trading-platform-app)

1. `npm create vite@latest services/frontend -- --template react-ts`
2. 기본 파일 정리
3. `LoginPage`, `OrdersPage`, `HealthPage` 구현
4. API client를 상대 경로(`/api/...`) 기준으로 정리
5. `nginx.conf.template` 작성
6. Dockerfile 작성
7. 로컬 빌드 및 테스트

**검증 기준**

- `npm test -- --run` 통과
- `npm run build` 성공
- `docker build` 성공
- 컨테이너가 `/`에서 index.html 반환

### Phase F3 — Helm chart 추가 (mock-trading-platform-app)

1. `charts/frontend/` 생성
2. `container.port`, `service.port`, `service.targetPort` 분리 반영
3. `nginx.upstreams.*` namespace 반영
4. `helm lint charts/frontend`
5. `helm template charts/frontend` 검증

### Phase F4 — CI 추가 (mock-trading-platform-app)

1. 변경 감지 로직에 `frontend` 추가
2. `frontend` filter는 `services/frontend/**`만 포함
3. `ci-frontend` job 추가
4. `lint-charts`에 frontend 추가
5. PR에서 frontend 소스 변경 시에만 job이 도는지 확인
6. chart-only 변경 시 `lint-charts`만 도는지 확인

### Phase F5 — GitOps 연결 (mock-trading-platform-gitops)

1. `frontend.yaml` Application 추가
2. `frontend.yaml` values 추가
3. account id placeholder 치환
4. main merge 후 ArgoCD sync 확인

### Phase F6 — reverse proxy 통합 검증

이 phase는 아래 둘 중 하나를 전제로 한다.

1. `auth-service`, `order-service` 가 이미 EKS에 Synced / Healthy 상태일 것
2. 또는 로컬 mock/backend를 따로 띄워 frontend nginx upstream으로 연결할 것

권장 경로는 **기존 GitOps 배포가 살아 있는 클러스터에서 검증**하는 것이다.

검증 절차:

1. `kubectl -n mock-trading-platform-dev get pods,svc`
2. `auth-service`, `order-service`, `frontend` 상태 확인
3. `kubectl -n mock-trading-platform-dev port-forward svc/mock-trading-platform-dev-frontend 8084:8084`
4. 브라우저에서 `http://localhost:8084` 접속
5. 네트워크 탭에서 요청이 모두 frontend origin 기준인지 확인
6. `/api/auth/login`, `/api/orders`, `/api/orders/{id}` 동작 확인

**검증 기준**

- 브라우저 네트워크 탭 기준 backend 직접 호출이 없음
- `/api/auth/login`, `/api/orders` 호출이 성공 또는 최소한 backend까지 도달
- CORS 에러가 없어야 함

### Phase F7 — 문서 갱신 (3 리포)

1. `mock-trading-platform-app/README.md`
2. `mock-trading-platform-app/SYSTEM_BLUEPRINT.md`
3. `mock-trading-platform-gitops/README.md` 및 `SYSTEM_BLUEPRINT.md`
4. `mock-trading-platform-infra/SYSTEM_BLUEPRINT.md`

---

## 5. 검증 체크리스트

- [ ] `cd services/frontend && npm test -- --run`
- [ ] `cd services/frontend && npm run build`
- [ ] `docker build -t frontend services/frontend`
- [ ] frontend 컨테이너 `/` 응답 확인
- [ ] `helm lint charts/frontend`
- [ ] `helm template charts/frontend`
- [ ] frontend chart의 `container.port`, `service.port`, `service.targetPort` 분리 확인
- [ ] frontend values에 `nginx.upstreams.auth`, `nginx.upstreams.order` 사용 확인
- [ ] CI에서 `ci-frontend`가 `services/frontend/**` 변경 시에만 트리거
- [ ] chart-only 변경 시 `lint-charts`만 트리거
- [ ] ECR에 `mock-trading-platform/frontend:latest`, `sha-xxxxxxx` push 확인
- [ ] ArgoCD `mock-trading-platform-dev-frontend` Synced / Healthy
- [ ] `kubectl -n mock-trading-platform-dev port-forward svc/mock-trading-platform-dev-frontend 8084:8084`
- [ ] `http://localhost:8084` 접속 후 UI 렌더링 확인
- [ ] 브라우저 네트워크 탭에서 backend 직접 호출이 없는지 확인
- [ ] 로그인/주문 흐름이 frontend origin 기준으로 동작 확인
- [ ] 3개 리포의 `SYSTEM_BLUEPRINT.md` 설명 동기화

---

## 6. 리스크 및 대응

| 항목 | 영향 | 대응 |
|------|------|------|
| frontend nginx가 임시 gateway 역할까지 맡음 | 책임이 일시적으로 늘어남 | 문서에 제거 대상임을 명시하고 다음 phase에서 이관 |
| proxy path rewrite 실수 | 로그인/주문 API 오동작 | 명시적 location 매핑 사용, 통합 검증 필수 |
| chart 포트 미분리 | Service 연결 실패 | `container.port`, `service.port`, `targetPort` 분리 |
| chart-only 변경 시 service job까지 실행 | CI 낭비 | `frontend` filter는 `services/frontend/**` 만 사용 |
| nginx 템플릿 치환과 앱 runtime env 혼동 | values 확장성 저하 | `env:` 와 `nginx.upstreams.*` namespace 분리 |
| auth/order가 준비되지 않은 상태에서 프록시 검증 시도 | Phase 순서 혼선 | F6에 명시적 전제 조건 추가 |

---

## 7. 변경 규모 요약

| 리포 | 신규 파일 | 수정 파일 | 비고 |
|------|-----------|-----------|------|
| mock-trading-platform-app | `services/frontend/*`, `charts/frontend/*` | `ci.yaml`, `README.md`, `SYSTEM_BLUEPRINT.md` | 프론트 앱 + chart + CI |
| mock-trading-platform-gitops | `argocd/applications/dev/frontend.yaml`, `environments/dev/values/frontend.yaml` | `README.md`, `SYSTEM_BLUEPRINT.md` | Application + values |
| mock-trading-platform-infra | - | `envs/dev/ecr.tf`, `SYSTEM_BLUEPRINT.md` | ECR repo 추가 |

---

## 8. 후속 phase로 미루는 작업

1. **전용 `api-gateway` 서비스 구현**
   - 현재 frontend nginx에 있는 API routing 책임 제거
   - 동일 public path를 gateway가 이어받도록 이관

2. **ALB Ingress 추가**
   - mock-trading-platform-app에 ingress 템플릿
   - mock-trading-platform-gitops에 ALB controller wiring

3. **ACM + Route53 + 커스텀 도메인**
   - mock-trading-platform-infra에 인증서/도메인 리소스 추가

4. **Secret 관리 일원화**
   - JWT_SECRET, DATABASE_URL 외부화

5. **E2E 테스트**
   - Playwright 기반 end-to-end 검증

6. **관측성 보강**
   - 프론트 에러 핸들링, backend trace 연결
