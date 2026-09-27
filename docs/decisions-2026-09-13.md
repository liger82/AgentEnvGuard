# AgentEnvGuard 1차 개발 결정 기록

작성일: 2026-09-13
대상 브랜치: `feat/aeg-mvp` (기준 `main` `1996ec1`)
상태: **1차 개발 완료.** 계획 11개 태스크, 태스크별 리뷰, 전체 브랜치 최종 리뷰와 수정,
사용자 실사용 검증(새 Claude Code 세션에서 차단 확인 + 일반 명령의 권한 확인 유지)까지 통과.

이 문서는 `docs/handoff-2026-09-13.md`(구현 착수 전 인수인계)를 대체한다.
git 히스토리는 **무엇이** 바뀌었는지를 담고, 이 문서는 **왜** 그렇게 결정했는지와
**무엇을 일부러 남겨 두었는지**를 담는다.

---

## 1. 문서 관계

| 문서 | 역할 |
|---|---|
| `docs/superpowers/specs/2026-09-06-agentenvguard-design.md` | 설계 스펙. 구속력 있는 권위. 최종 리뷰에서 일부 수정됨(§2.4) |
| `docs/superpowers/plans/2026-09-06-agentenvguard.md` | 구현 계획(11 태스크). 스펙의 논증. 충돌 시 스펙이 이긴다 |
| `README.md` | 사용자용 차단 범위와 한계 |
| 이 문서 | 실행 중 내린 판정과 남은 이슈 |

개발 방식은 superpowers:subagent-driven-development. 태스크마다 구현 → 리뷰(스펙 준수 + 품질)
→ 필요 시 수정 → 재리뷰를 거쳤고, 끝에 전체 브랜치 리뷰를 한 번 더 했다.

---

## 2. 판정 기록

각 항목은 **결정 — 이유 — 틀렸을 때 비용** 순서다. 시간순이다.

### 2.1 착수 전 사전 점검

1. **모듈 경로는 `github.com/liger82/AgentEnvGuard`.** 계획의 `github.com/stuartkim/agentenvguard` 가 아니다.
   — Go 모듈 경로는 원격 저장소 경로와 같아야 `go install …@latest` 가 동작한다. 계획이 원격 생성보다 앞섰다.
   — `module` 한 줄과 import 약 6줄 수정.
2. **Task 7 의 usage 문구는 아직 연결되지 않은 `install`/`scan`/`init` 을 미리 광고한 채 둔다.**
   — Task 10 에서 간극이 닫히고, 줄였다 늘리면 같은 파일을 네 번 뒤집는다.
   — 브랜치 중간 커밋 세 개 동안 `aeg install` 이 "알 수 없는 명령".
3. **Task 8 에 `args: ["hook"]` 와 `matcher: "Bash|Read|Grep"` 를 검사하는 테스트를 추가한다.**
   — 스펙상 둘 다 핵심(exec form 은 성능, Grep 은 정책 전체의 의미)인데 계획의 테스트는 둘을 검사하지 않았다.
   — 테스트 하나.
4. **`aeg scan` 은 `Library` 라는 이름의 모든 디렉터리를 건너뛴다**(`~/Library` 만이 아니다).
   — `Scan` 은 홈 경로 맥락이 없고, 이름이 `Library` 인 프로젝트 폴더에 평문 `.env` 가 있을 확률은 `~/Library` 를 훑는 비용보다 훨씬 낮다.
   — 그런 폴더 안의 평문 `.env` 를 놓친다.
5. **Task 11 Step 4(`sudo cp`, 실제 `~/.claude/settings.json` 에 `aeg install`, 새 세션 확인)는 서브에이전트에 맡기지 않고 사용자가 한다.**
   — 저장소 밖에 쓰고, sudo 가 필요하고, 사용자의 실제 Claude Code 설정을 바꾸며, 계획이 명시한 실질 관문이다.
   — 종단 간 검증 없이 병합될 뻔함. (실제로 사용자가 수행해 통과.)

### 2.2 태스크 리뷰 중

6. **(Task 3) `--mask=0` 등호 표기도 탐지한다.**
   — 같은 플래그의 다른 표기일 뿐이다.
   — 조건 하나. **→ 판정 20 에서 이 탐지 범주 전체가 제거됨.**
7. **(Task 4) `Covers` 가 예외 목록의 루트 경로도 `filepath.Clean` 한다.**
   — 공개 함수이고 `Engine.AllowRoots` 도 공개 필드라, `LoadFrom` 을 거치지 않은 호출자가 끝의 `/` 때문에 예외를 잃는다.
   — 호출마다 `Clean` 몇 번.
8. **(Task 5) 경로 없는 Grep 이라도 패턴에 `DOTENV_PRIVATE_KEY` 가 있으면 차단한다.**
   — Claude Code 의 Grep 은 `path` 가 선택이라 `Grep(pattern="DOTENV_PRIVATE_KEY")` 가 개인키 줄을 그대로 반환했다. 스펙이 Grep 을 매처에 넣은 이유 자체가 무력화된 상태였다.
   — 그 문자열을 정당하게 검색하는 경우의 오탐(`~/.config/aeg/allow` 로 회피 가능).
9. **(Task 5) 일반적인 재귀 검색이 평문 `.env` 의 줄을 우연히 매칭하는 경우는 막지 않는다.**
   — 막으려면 넓은 검색 자체를 막아야 하고, 그러면 에이전트의 주 검색 도구가 망가진다. 우발적 노출과 의도적 탈취의 경계 문제다. README 한계에 명시.
   — 평문 `.env` 가 무관한 검색으로 샐 수 있음. `aeg scan`/`aeg init` 으로 평문을 없애는 것이 대책.
10. **(Task 7) `recover` 를 `Hook` 최상단으로 올려 `Emit` 까지 덮는다.**
    — 스펙은 "패닉은 최상위에서 recover" 인데 계획 코드는 Parse/Decide 만 덮었다. `Emit` 에서 패닉이 나면 exit 0 보장이 깨진다.
    — 없음.
11. **(Task 8) `aeg install` 을 다시 실행해도, 사용자가 손댄 기존 aeg 등록(matcher·args)은 고치지 않는다.**
    — v1 이라 오래된 등록이 존재할 수 없고, 스펙은 "이미 있으면 아무것도 바꾸지 않는다" 이며, 사용자가 일부러 좁힌 설정을 되돌리는 게 더 나쁘다. 진단은 향후 `aeg doctor` 의 몫. README 한계에 명시.
    — 향후 matcher 를 바꾸는 릴리스에서 기존 사용자는 옛 matcher 를 경고 없이 유지.
12. **(Task 9) `--depth` 플래그를 추가한다.**
    — 스펙이 명시한 사용자 조정 수단인데 계획에 파싱이 없었다.
    — 인수 파싱 몇 줄.
13. **(Task 9) `go/pkg/mod` 제외를 부분 문자열이 아니라 경로 세그먼트 단위로 비교한다.**
    — `mongo/pkg/mod` 같은 무관한 경로를 조용히 건너뛰는 것은 "깨끗하다"는 거짓 결과를 준다. 사소 이슈지만 수정 라운드에 올렸다.
    — 코드 약간 증가.
14. **(Task 10) `aeg init` 의 git 히스토리 확인이 실패하면 안내를 출력하되 오류로 끝내지 않는다.**
    — 확인 실패가 무출력이면 "히스토리 깨끗함"과 구분되지 않아, 스펙이 금지한 거짓 안심을 준다. 반면 git 저장소가 아닌 프로젝트는 정상이고 암호화는 이미 성공했다.
    — git 밖에서 실행하면 한 줄 더 출력.

### 2.3 Task 11 (검증·README)

15. **dotenvx 는 공식 tap `brew install dotenvx/brew/dotenvx` 로 설치한다(2.24.1).**
    — `dotenvx` 는 Homebrew core 에 없다. 서드파티 tap 이라 사용자에게 먼저 물었고 사용자가 선택했다. README 와 `aeg init` 안내도 이 형태.
    — README 한 줄.
16. **Step 1(dotenvx 설치)과 Step 4(실제 설치)는 구현 에이전트가 아니라 컨트롤러와 사용자가 한다.**
    — 판정 5 와 같은 이유.
    — 없음.
17. **Step 9 의 "이슈로 기록"은 GitHub 이슈를 만들지 않는다.**
    — 공개 원격에 대한 외부 행동이고, 같은 항목이 이미 문서에 있다. (이제 이 문서 §4.)
    — 사용자가 나중에 이슈 4개를 직접 만듦.
18. **README 는 계획의 원문과 달리 실행 중 결정을 반영한다.**
    — 모듈 경로, tap 설치, 판정 9·11 의 한계 명시, `--depth`, 설정 백업과 키 재정렬 안내. 스펙이 권위이고 판정 9·11 이 README 명시를 요구했다.
    — README 가 계획보다 김.

### 2.4 최종 리뷰와 수정 (스펙 수정 포함)

최종 리뷰는 "병합 불가"(Critical 4, Important 7)였다. 핵심 주장은 컨트롤러가 실제 바이너리와 dotenvx 2.24.1 로 재현한 뒤 판정했다.

19. **훅은 차단하지 않는 호출에 아무것도 출력하지 않는다.** `permissionDecision: "allow"` 를 내지 않는다. 스펙 수정.
    — Claude Code 에서 `allow` 는 권한 확인을 건너뛴다. 스펙대로면 aeg 설치가 모든 Bash/Read/Grep 호출(`rm -rf` 포함)을 자동 승인해 버린다. 시크릿 가드가 사용자의 주 안전장치를 없애면 안 된다. 무출력 + exit 0 이 "판정 없음, 평소 권한 흐름"이다.
    — 기능상 비용 없음.
20. **"마스킹 우회" 차단 범주(`--no-redact`, `--mask 0`)를 제거하고, `dotenvx run … -- env|printenv|set|export -p|declare -x/-p` 를 차단한다.** 차단 메시지에서 "`dotenvx run --` 이 출력을 가린다"는 주장을 뺀다. 스펙 수정.
    — dotenvx 2.24.1 실측: `run` 은 기본으로 평문을 주입하고, `--redact`/`--mask` 는 선택 사항이며 주입값만 바꾼다. `--no-redact` 는 존재하지 않는다. 스펙의 전제가 틀려서 이 범주는 없는 위협을 막고 실제 위협(`run -- printenv`)을 놓치고 있었다.
    — 향후 dotenvx 가 진짜 출력 마스킹을 추가하면 메시지가 과소 설명.
21. **`dotenvx keypair` 를 차단한다.** 다른 하위 명령은 `--help`·실행으로 개인키 출력이 확인된 것만 차단한다.
    — 실측으로 `DOTENV_PRIVATE_KEY` 를 그대로 출력했다. 조사한 나머지(`encrypt --stdout`, `ls`, `validate`, `genexample`)는 값을 내지 않았다.
    — 드문 명령의 오탐(예외 목록으로 회피 가능).
22. **`cd DIR &&` 이후 경로 추적과 `git show HEAD:.env`/`git diff .env` 같은 히스토리 읽기는 막지 않고 README 에 적는다.**
    — 셸 상태 시뮬레이션은 스펙의 비목표가 경고하는 문자열 매칭의 늪이다. 히스토리의 평문은 `aeg init` 의 경고와 키 로테이션 권고가 다룬다.
    — 문서화된 우발적 읽기 간극 두 개가 남음.
23. **`PRIVATE_KEY`/`DOTENV` 같은 넓은 재귀 Grep 은 막지 않는다. Grep 의 `glob` 이 `.env.keys` 를 가리키면 막는다.**
    — 넓은 패턴을 막으면 일반 코드 검색이 오탐. 명시적으로 겨냥한 형태만 닫는다.
    — 숨김 파일까지 뒤지는 넓은 검색이 `.env.keys` 줄을 보여줄 수 있음.
24. **진입점을 `cmd/aeg/main.go` 로 옮긴다.** 설치는 `go install github.com/liger82/AgentEnvGuard/cmd/aeg@latest`.
    — 루트에 `main.go` 가 있으면 설치된 바이너리 이름이 `AgentEnvGuard` 가 되어 README 의 다음 명령 `aeg install` 이 실패한다.
    — 디렉터리 이동 하나.
25. **dotenvx 없이 실행한 맨 `env`/`printenv`/`set` 은 허용하고 README 에 적는다.**
    — `set -e` 같은 흔한 사용에 오탐이 나고, 훅 환경이 Bash 에 상속되는지 확인되지 않아 환경변수 기반 판별도 채택하지 않았다.
    — `DOTENV_PRIVATE_KEY` 를 export 해 둔 환경에서 환경 덤프가 샘.
26. **수정 과정에서 생긴 이스케이프 따옴표 회귀는 병합 차단 사유다.** 사용자 승인으로 추가 수정 라운드를 열어, 원래 있던 두 구멍(`sudo cat .env.keys`, `cat .env.keys>/tmp/x`)과 함께 고쳤다.
    — 백슬래시를 문자로 취급해 `echo 'it'\''s'; cat .env.keys` 가 통과했다. 수정 전 커밋에서는 차단되던 명령이다.
    — 없음.
    - 수정 방식: 따옴표는 bash 규칙을 따른다(따옴표 밖 `\` 는 다음 글자 이스케이프, 큰따옴표 안은 `"`·`\`·`$`·`` ` `` 만, 작은따옴표 안은 문자 그대로). 명령 판별 전에 `sudo`/`doas`/`env`/`command`/`builtin`/`exec`/`nice`/`nohup`/`time`/`VAR=val` 접두어를 건너뛴다. `>`·`>>`·`2>`·`&>` 등 리다이렉션을 붙은 경로에서 떼어 내고, 출력 대상은 읽기로 치지 않는다.
    - 의도한 동작 변경: `cat foo > .env`, `cat > .env.keys` 는 이제 허용(파일에 쓰기만 하고 내용을 출력하지 않음).

---

## 3. 남은 이슈

모두 1차 병합을 막지 않는다고 판단한 것이다. 2026-09-13 `ff58938` 기준으로 아직 유효한 것만 적었다(수정된 항목은 제외).

### 3.1 차단 판정의 간극 (2차 후보)

비목표인 의도적 탈취(`d=dotenvx; $d get X`, `DOTENV_PRIVATE_KE[Y]` 같은 정규식 우회)는 여기 적지 않는다. README 한계에 있다.

| 간극 | 현재 동작 | 비고 |
|---|---|---|
| 목록에 없는 접두어 값 옵션 | `sudo -h host cat .env.keys` 허용 | `sudo -h/-p/-C/-D/-r/-t`, `env -S` 가 인자를 먹는 것을 모름 |
| `dotenvx run --` 뒤의 접두어·셸 | `dotenvx run -- sudo printenv`, `… -- sh -c printenv`, `… -- env -u FOO` 허용 | 덤프 검사가 접두어를 건너뛰지 않음 |
| `declare -p NAME` / `export -p NAME` | `dotenvx run --` 아래에서 허용 | 판정 20 의 문언("추가 인자 없음")을 따름 |
| 명령 치환 | `$(...)`, 백틱, `<(...)` 안을 별도 명령으로 나누지 않음 | 원래부터 |
| `cd DIR &&`, git 히스토리 읽기 | 허용 | 판정 22, README 에 명시 |
| 넓은 재귀 Grep, 와일드카드 glob(`.env*`) | 허용 | 판정 9·23, README 에 명시 |
| 맨 `env`/`printenv`/`set` | 허용 | 판정 25, README 에 명시 |
| 대소문자 | `.ENV.KEYS` 같은 이름 미탐 | macOS 파일시스템은 대소문자 무시 |

오탐(무해):
- dotenvx 토큰을 세그먼트 어디서나 찾으므로 `echo dotenvx get` 이 차단된다.
- 명령 문자열에 `DOTENV_PRIVATE_KEY` 가 들어 있으면 `unset DOTENV_PRIVATE_KEY` 같은 무해한 명령도 차단된다. (실제로 이 문서를 쓰는 동안 검증 스크립트가 이 규칙에 걸렸다.)
- `echo a <> .env.keys` 차단. heredoc 본문 줄도 명령으로 검사.
- `.env.vault`(암호문)를 평문으로 판정해 차단.

### 3.2 `aeg install`

- 설정 파일이 심볼릭 링크(dotfiles 관리 도구)이면 원자적 쓰기의 rename 이 링크를 일반 파일로 바꾼다. `filepath.EvalSymlinks` 로 해결 가능.
- 백업 파일 `os.Stat` 이 NotExist 가 아닌 오류(권한 등)를 내면 백업을 조용히 건너뛰고 설치를 진행한다.
- 등록 식별이 `command` 경로 완전 일치라, 설치 위치가 바뀌면(예: 버전별 Homebrew 경로) 옛 등록이 남고 새 등록이 하나 더 생긴다.
- JSON 재직렬화로 키 순서가 알파벳순으로 바뀐다(README 에 명시).
- 사용자가 손댄 등록은 고치지 않는다(판정 11).

### 3.3 `aeg scan`

- 이름 기반 제외(`Library`, `build`, `dist` 등)가 위치와 무관하게 적용된다(판정 4).
- `go/pkg/mod` 제외는 기본 GOPATH 를 가정한다. 사용자 지정 GOPATH 의 모듈 캐시는 훑는다.
- `.gitignore` 확인이 같은 폴더의 정확한 한 줄(`.env.keys`)만 인정한다. 상위 `.gitignore`, `/.env.keys`, `.env*` 는 "없음"으로 보고(과잉 보고 방향).
- 64KB 이후 내용은 읽지 않는다(훅과 같은 판정기, 성능 예산).
- 제외·깊이 검사에 `path != root` 가드가 없어 `aeg scan ~/go/pkg/mod/somepkg` 는 조용히 0건.
- `skipped` 는 디렉터리와 파일 오류를 함께 세면서 "건너뛴 디렉터리"로 표시하고, 테스트가 없다. "범위를 좁히세요" 안내는 항상 출력.
- `ParseScanArgs`: `--depth N`/`--depth=N` 검증 코드 중복, 반복 지정 시 마지막 값 사용, `--` 종결자 미지원.

### 3.4 `aeg init`

- `.env` 존재 확인이 dotenvx 설치 확인보다 먼저다(스펙은 dotenvx 확인이 1단계).
- `git log` 에 스펙에 없는 `--oneline` 추가(존재 여부만 보므로 무해).
- `.env.keys` 가 이미 든 `.gitignore` 로 `Init` 전체를 도는 테스트가 없다(`EnsureGitignore` 멱등 테스트로만 간접 확인).
- 조언용으로 `.gitignore` 를 한 번 더 읽는다.

### 3.5 코드 품질 (그대로 둬도 됨)

- `go.mod` 가 `go 1.27.1` 패치 버전까지 고정.
- `envfile.go` 의 `sc.Buffer(..., 64*1024)` 는 bufio 기본값과 같다.
- `path.go` 의 `base == ".env.keys" ||` 는 뒤의 `HasPrefix` 에 포함되어 중복.
- `allowlist.go` 의 `path == ""` 검사 중복, `Path()`/`Load()` 직접 테스트 없음, 루트 `/` 는 자기 자신만 매칭.
- `Cwd == ""` 경로 테스트 없음. 예외 목록이 `.env.keys` 차단보다 먼저 적용됨(의도: 사용자가 쓴 전체 예외).
- `engine.go` 에 판정 로직과 긴 메시지 상수가 함께 있음.
- `claudecode.go` 의 필드 주석이 영어(`// Bash` 등).
- `cmd/aeg/main.go` 가 `policy.New()`(예외 목록 파일 읽기)를 `Hook` 의 recover 밖에서 호출. 현재 패닉 경로는 없음.
- `install_test.go` 의 익명 구조체 중복, 파일 권한 보존 테스트 없음.

---

## 4. 공개 배포 전 남은 항목 (범위 밖)

1. **메시지 영어화.** 차단 메시지(`internal/policy/engine.go` 의 `msg*` 상수), CLI 출력(`internal/cmd/*.go`), 도움말(`cmd/aeg/main.go`). 테스트는 메시지 본문이 아니라 명령 이름만 검사하므로 바꿔도 테스트가 깨지지 않는다.
2. **`aeg` 이름 최종 확인.** PATH·Homebrew 는 비어 있음을 확인했다. npm·crates.io·GitHub 도 확인한다.
3. **크로스 컴파일 배포.** `GOOS`/`GOARCH` 조합 바이너리와 Homebrew tap 또는 `curl | sh`. Windows 경로(`filepath.Separator`)는 고려했으나 실제 테스트 필요.
4. **다른 에이전트 어댑터.** `adapter.Adapter` 인터페이스가 분리되어 있어 Cursor·Codex 지원은 구현 하나 추가.
5. **`aeg doctor`.** 기존 등록의 matcher·args 가 현재 버전과 다른지 진단(판정 11).

---

## 5. 검증 기록

- 단위 테스트: `go vet ./...`, `go test ./...`, `gofmt -l .` 모두 깨끗(`ff58938`).
- 실제 dotenvx 2.24.1 로 임시 프로젝트에서 `aeg init` → 암호화·`.gitignore` 등록 확인.
- 실제 바이너리에 훅 JSON 입력: 최종 수정 후 신규 23개 + 기존 11개 사례 모두 기대대로(차단은 deny JSON, 허용은 무출력), 모두 exit 0.
- 성능: 훅 100회 0.297초 vs `/usr/bin/true` 100회 0.125초 → 호출당 약 1.7ms 순증. 예산 10ms.
- 최종 수정 재리뷰: 수정 전후 바이너리를 엣지 케이스 37개로 비교, 의도한 변경(`cat > .env.keys` 허용) 외 회귀 없음.
- **사용자 실사용 검증(2026-09-13, 통과):** `/usr/local/bin` 설치, `aeg install` 과 재실행(멱등·백업 유지), 새 Claude Code 세션에서 `.env.keys` 읽기(Bash·Read)·경로 없는 Grep·`dotenvx get`/`keypair`/`run -- printenv`·평문 `.env` 차단, 암호화된 `.env` 읽기 허용, 일반 명령(`rm`)의 권한 확인 유지, `aeg scan`.
