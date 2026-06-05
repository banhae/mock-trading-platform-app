# loadtest — 부하 생성 + 5xx 장애 주입 런북

`mock-trading-platform-dev` 클러스터에서 order-service 에 부하를 주고, 의존성 장애를
주입해 관측성 스택(RED 대시보드 · PrometheusRule)이 5xx·p99 를 잡아내는지 검증한다.

- `loadgen.yaml` — curl 워커 N개로 `POST /api/orders` 를 반복 호출하는 Deployment.

## 1. 정상 부하 (2xx)

```sh
kubectl apply -f loadgen.yaml          # 기본 TARGET = order-service ClusterIP
```

→ RED 대시보드 `Request rate` 상승, `p50/p99 latency` 에 실데이터. 5xx 는 0.

## 2. 5xx 장애 주입 (HighErrorRate 발화)

핵심: order-service `readiness(/ready)` 가 DB 의존이라 **DB 를 내리면 파드가 NotReady →
Service 엔드포인트에서 빠짐**. 그래서 ClusterIP 로 때리면 요청이 안 닿아(HTTP 000)
5xx 카운터가 안 오른다. **파드 IP 직접** 타깃이면 요청이 닿아 서버가 500 을 응답하고,
Prometheus 는 NotReady 파드도 계속 스크랩하므로 Grafana 에 5xx 가 잡힌다.

```sh
# (a) ArgoCD selfHeal 동결 — 안 하면 postgres scale 0 을 즉시 되돌린다
kubectl scale statefulset argocd-application-controller -n argocd --replicas=0

# (b) DB 다운 (장애 주입)
kubectl scale statefulset mock-trading-platform-dev-postgres-postgresql -n mock-trading-platform-dev --replicas=0
kubectl delete pod mock-trading-platform-dev-postgres-postgresql-0 -n mock-trading-platform-dev --grace-period=0 --force  # 풀 커넥션 즉시 차단

# (c) order-service 파드 IP 로 부하 재타깃 + 워커 증설 (요청당 ~3s DB 재시도 → 워커 수가 처리율 좌우)
POD_IP=$(kubectl get pod -n mock-trading-platform-dev -l app.kubernetes.io/name=order-service -o jsonpath='{.items[0].status.podIP}')
kubectl set env deploy/loadgen -n mock-trading-platform-dev \
  TARGET="http://$POD_IP:8080/api/orders" WORKERS=60

# (d) 검증 — 5xx req/s · 에러율 · 알림(firing 까지 for=5m)
#     Prometheus 서비스명은 chart fullname truncation 에 따라 달라지므로 라벨로 조회한다.
PROM_SVC=$(kubectl get svc -n mock-trading-platform-dev -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}')
P=$PROM_SVC.mock-trading-platform-dev:9090
kubectl run q --rm -i --restart=Never -n mock-trading-platform-dev --image=curlimages/curl:8.10.1 -- \
  sh -c "curl -s --data-urlencode 'query=sum(rate(http_request_duration_seconds_count{namespace=\"mock-trading-platform-dev\",status=~\"5..\"}[1m]))' http://$P/api/v1/query"
```

→ RED `5xx error rate` 가 0 → 급등, `p99` 단계적 상승(실패 요청 ~3s), `MockTradingPlatformHighErrorRate` firing.
이때 Grafana `Mock Trading Platform — RED Overview`(Last 15m) + `Mock Trading Platform — Infrastructure` 에서 5xx·p99 상승을 확인한다.

## 3. 원복 (필수)

```sh
kubectl scale statefulset mock-trading-platform-dev-postgres-postgresql -n mock-trading-platform-dev --replicas=1
kubectl scale statefulset argocd-application-controller -n argocd --replicas=1   # selfHeal 재개
kubectl delete -f loadgen.yaml
```

→ order-service Ready 회복, `POST /api/orders` 201, 5xx rate 0,
`MockTradingPlatformHighErrorRate` 는 5m rate 윈도우가 지나면 자동 resolve.

## 검증되는 PrometheusRule (mock-trading-platform-app-alerts)

| alert | 식 | 이 런북에서 |
|---|---|---|
| `MockTradingPlatformHighErrorRate` | 5xx rate / total > 1% (5m) | 99%+ 로 firing |
| `MockTradingPlatformHighLatencyP99` | p99 > 500ms (10m) | 실패 재시도로 p99 ~수 초 |
| `MockTradingPlatformPodCrashLooping` | restarts > 3 / 15m | (이 시나리오에선 미발화 — liveness=/health 는 DB 비의존) |
