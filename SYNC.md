# SYNC.md — upstream(exchange-app) 동기화 규칙

## 관계

- **upstream (source of truth, 비공개)**: `exchange-app`
- **이 리포 (공개 mirror)**: `mock-trading-platform-app`
- **방향**: upstream → mock **단방향**. mock 의 변경을 upstream 으로 역류시키지 않는다.

mock 은 upstream 의 **애플리케이션 구조/로직을 mock 네이밍 + placeholder 형태로** 가진다.
**동기화는 "파일 복사"가 아니라 "로직 이식"이다** — exchange 고유 네이밍·account·시크릿은
절대 따라오지 않는다.

> ⚠️ 과거에 upstream account 가 공개 리포로 복사되어 리포를 통째로 삭제·재생성한
> 사고가 있었다. 아래 C 분류와 `scripts/sync-guard.sh` 는 그 재발 방지가 목적이다.

---

## 분류 — 무엇을 어떻게 다루나

| 분류 | 처리 | 이 리포의 대상 |
|---|---|---|
| **A. 구조/로직** | upstream → mock **이식**(네이밍 변환하여) | `services/*`(소스코드), `charts/*`(Helm 템플릿 구조), `deploy/`, `loadtest/`, `docs/`, `.github/workflows/*`, README |
| **B. 네이밍** | **변환**(무시 아님) | `exchange` → `mock-trading-platform`. Go module path(`github.com/banhae/exchange-app/...` → `.../mock-trading-platform-app/...`), 이미지 repository(`exchange/<svc>` → `mock-trading-platform/<svc>`), chart 명, k8s 리소스명, ingress 명 |
| **C. account/시크릿** | **이식 금지 / placeholder·자체값 유지** (= 민감정보 drift 는 무시) | AWS account ID(CI 는 `vars.AWS_ACCOUNT_ID` 로만 참조 — literal 금지), ECR registry host, 실제 JWT/DB 등 시크릿, ARN, `.env` |

**"민감정보 drift 는 무시한다"** = C 분류. upstream 이 구체값으로 바뀌어도 mock 은
placeholder/자체 네이밍을 그대로 둔다. 이 drift 는 의도된 것이며 맞추지 않는다.

> CI(`.github/workflows`)는 account 를 `${{ vars.AWS_ACCOUNT_ID }}` 로만 참조한다.
> 워크플로에 12자리 account 를 literal 로 박지 않는다. ECR push 는 AWS 변수 미설정 시
> 자동 skip 되도록 유지(공개 fork 안전).

---

## 동기화 절차

upstream(`exchange-app`)에 변경이 생겼을 때:

1. **변경 성격 분류** — A(구조/로직)만 이식 대상. B는 변환, C는 건드리지 않음.
2. **로직 이식** — 해당 파일/부분을 mock 에 반영하되:
   - 모든 `exchange*` 네이밍을 `mock-trading-platform*` 로 변환(B) — Go module path 포함
   - 실제 account/ARN/시크릿이 보이면 placeholder/`vars.*` 로 되돌림(C)
3. **가드 실행** (필수):
   ```bash
   ./scripts/sync-guard.sh
   ```
   `✓ 통과` 가 떠야 한다. ❌ 가 뜨면 upstream 값/네이밍이 새어든 것 — 2번으로 돌아간다.
4. **빌드/테스트** — 변경한 서비스의 `go build ./...` / `go test ./...`(또는 frontend `npm`) 통과 확인.
5. **PR** 로 올린다. main 직접 푸시 금지.

### pre-push 자동화 (권장)
```bash
ln -sf ../../scripts/sync-guard.sh .git/hooks/pre-push
```
이후 `git push` 마다 가드가 자동 실행되어, 위반 시 푸시가 차단된다.

---

## 빠른 체크리스트

- [ ] 이식한 변경이 A(구조/로직)인가? (C 라면 멈춤)
- [ ] 모든 `exchange*` → `mock-trading-platform*` 변환했는가? (Go module path 포함)
- [ ] account/ARN/시크릿이 placeholder·`vars.*` 인가?
- [ ] `./scripts/sync-guard.sh` 통과 + 빌드/테스트 통과했는가?
- [ ] PR 로 올렸는가?
