# AgentEnvGuard 설계

작성일: 2026-09-06

## 이름

프로젝트 이름은 **AgentEnvGuard**, 실행 바이너리는 **`aeg`** 다.

`Agent` 접두어는 이 도구의 차별점을 담는다 — 막는 상대가 git도 사람도 아니라
코딩 에이전트다. 바이너리를 `aeg` 로 짧게 따로 두는 이유는 CLI 관례
(`git`, `jq`, `rg`, `age`, `sops`, `direnv`)에 맞추기 위해서다. 13자짜리
명령은 모두가 alias를 걸게 되고, 그러면 이름이 아무 일도 하지 않는다.

기존 `amannirala13/envguard` 와의 혼동 가능성은 남는다. README 첫 문단에서
"dotenvx 위에 얹는 에이전트 가드레일이며 시크릿 매니저가 아니다"를 명시해
포지셔닝으로 해소한다.

## 문제

코딩 에이전트(Claude Code)에게 API 키가 평문으로 노출된다. `.env` 파일을 쓰면
에이전트가 `cat` 한 번으로 키를 읽고, 그 값이 대화 로그와 컨텍스트에 남아
어디로 흘러가는지 통제할 수 없다.

이 문제를 노트북 전체에서, 프로젝트를 가리지 않고, 어떤 OS에서든 막고 싶다.

## 목표

- 에이전트 컨텍스트에 시크릿 평문이 **우발적으로** 들어가지 않는다.
- 언어 무관. Python·Node·shell 등 에이전트가 실행하는 아무 스크립트에 적용된다.
- 크로스플랫폼. macOS·Linux·Windows, 그리고 헤드리스 Linux·Docker·SSH 포함.
- 머신 단위 1회 설치. 프로젝트마다 설정하지 않는다.
- 에이전트가 **어떤 키가 존재하는지는** 스스로 알 수 있다.

## 비목표

- **의도적 탈취 차단이 아니다.** 훅은 커맨드 문자열 매칭이므로
  `d=dotenvx; $d get X` 같은 방식으로 우회된다. bash를 실행할 수 있는
  에이전트에게서 값을 완전히 숨기는 것은 불가능하다. 이 한계를 README에
  명시한다. 흐리면 잘못된 안전감을 파는 도구가 된다.
- 사람(사용자 본인)이 터미널에서 시크릿을 보는 것은 막지 않는다.
- 시크릿 저장·암호화를 직접 구현하지 않는다. dotenvx에 위임한다.
- 팀 공유, 클라우드 동기화, 시크릿 로테이션 자동화는 범위 밖이다.

## 선행 조사

### 기존 EnvGuard (github.com/amannirala13/envguard) — 부적합

TypeScript/Node. 시크릿을 OS 키체인에 저장한다. 확인 결과 다음이 맞지 않는다.

- **`run` 계열 명령이 없다.** 전체 명령은 `init`, `status`, `migrate`, `edit`,
  `set`, `show`, `get`, `del`, `list`, `copy`, `check`, `template`, `export`.
  Python 스크립트가 키를 받는 경로는 `envg get KEY`(stdout에 평문) 또는
  `envg export --unsafe --to .env`(평문 파일)뿐이다. 막으려는 실패 모드 그 자체다.
- 마스킹이 자기 출력(`show`)에만 적용되고 자식 프로세스 출력은 다루지 않는다.
- 저장소가 OS 키체인이라 Secret Service가 없는 헤드리스 Linux·Docker에서 깨진다.
- alpha, 스타 2개, 단독 개발자, Node 18+ 상시 의존.

### dotenvx — 채택

필요한 것이 이미 전부 구현되어 있다.

| 요구 | dotenvx |
|---|---|
| 임의 자식 프로세스 주입 | `dotenvx run -- python3 x.py` |
| 자식 출력 마스킹 | `--redact` (기본 동작, `--mask 0` 지원) |
| 이식 가능한 볼트 | ECIES/secp256k1 암호화 `.env`, 커밋 가능 |
| 헤드리스 동작 | OS 키체인 / `.env.keys` / `DOTENV_PRIVATE_KEY` |
| 배포 | 단일 바이너리, brew·curl·winget·npm·docker |

공개키 암호이므로 **볼트를 열지 않고 새 시크릿을 쓸 수 있다.**

암호화된 `.env` 형식:

```ini
DOTENV_PUBLIC_KEY="034af93e93708b994c10f236c96ef88e47291066946cce2e8d98c9e02c741ced45"
DART_API_KEY="encrypted:BDqDBibm4wsYqMpCjTQ6BsDHmMadg9K3dAt+Z9HPMfLE..."
```

**키 이름은 평문, 값만 암호문이다.** 따라서 에이전트가 `.env` 를 그냥 읽으면
어떤 키가 있는지 파악할 수 있다. 별도의 "값 없는 키 목록" 생성기가 필요 없다.

`.env.keys` 형식 (gitignore 필수):

```ini
DOTENV_PUBLIC_KEY="034af9..."
DOTENV_PRIVATE_KEY="122...0b8"
```

### 남는 공백 = 이 프로젝트의 범위

dotenvx는 볼트·주입·마스킹을 준다. 하지만 에이전트에게 다음을 강제하지 않는다.

- `.env` 를 직접 읽지 말고 `dotenvx run --` 로 실행할 것
- `dotenvx get` / `decrypt` / `.env.keys` 읽기를 하지 말 것
- 평문 `.env` 를 쓰는 프로젝트를 발견하고 마이그레이션할 것

이 도구는 **시크릿 매니저가 아니라 에이전트용 시크릿 가드레일**이다.

## 아키텍처

```text
Claude Code
  │ PreToolUse (Bash, Read, Grep)
  ▼
훅 어댑터  ──►  판정 엔진  ──►  allow / deny + 교정 메시지
(도구별)        (도구 중립)
                   │
                   ├─ 명령 분류기 (위험 명령 패턴)
                   ├─ 경로 분류기 (.env.keys, .env, 예외)
                   └─ .env 암호화 판별기
```

단일 Go 바이너리가 CLI와 훅 판정기를 겸한다. 판정 엔진은 에이전트 도구와
무관하게 순수 함수로 두고, Claude Code의 JSON 입출력 규약을 다루는 부분만
어댑터에 가둔다. 지금은 어댑터가 하나다.

**언어를 Go로 정한 이유**는 성능이다. PreToolUse 훅은 모든 Bash·Read·Grep 호출마다
실행되고, 전역 설치이므로 하루 종일 돈다. Node 기동은 50~100ms라 체감된다.
Go 단일 바이너리는 1~3ms다. 배포도 단일 파일이라 brew·curl 채널이 단순하다.

**성능 예산: 훅 1회 실행 10ms 이내.**

## 정책

### 허용

| 동작 | 이유 |
|---|---|
| `.env` 읽기 (암호문인 경우) | 값이 ECIES 암호문이라 컨텍스트에 들어가도 무해. 에이전트가 키 이름을 파악하는 경로 |
| `dotenvx run -- <cmd>` | 의도된 정상 경로. redact 기본값 유지 시 |
| `dotenvx set` | 공개키 암호라 볼트를 열지 않고 추가 가능 |
| `.env.example`, `.env.template`, `.env.sample` 읽기 | 설계상 공개 파일 |

### 차단

| # | 대상 | 검사 위치 |
|---|---|---|
| 1 | `.env.keys` 접근 (`cat`, `head`, `less`, `grep`, `sed`, `awk`, `cp`, `base64` 등) | Bash, Read, Grep |
| 2 | `DOTENV_PRIVATE_KEY` 출력 (`echo`, `env`, `printenv`, `set`) | Bash |
| 3 | `dotenvx get <KEY>` | Bash |
| 4 | `dotenvx decrypt` (특히 `--stdout`) | Bash |
| 5 | redact 우회 플래그 `--no-redact`, `--mask 0` | Bash |
| 6 | **평문** `.env` 읽기 | Bash, Read, Grep |

### Grep 을 반드시 포함한다

`Grep` 은 매칭된 **줄 내용을 반환한다.** 따라서
`Grep(pattern="DOTENV_PRIVATE_KEY", path=".env.keys")` 한 번이면 Bash도 Read도
거치지 않고 개인키가 그대로 나온다. 매처에서 Grep을 빠뜨리면 정책 전체가
무의미해진다.

`Glob` 은 경로만 반환하므로 대상이 아니다. `Edit`·`Write` 는 파일 내용을
반환하지 않고, `Edit` 는 선행 `Read` 를 요구하므로 그 지점에서 이미 막힌다.

### 6번이 전역화의 핵심

디렉터리 하나면 "`.env` 는 암호문이니 허용"으로 끝난다. 노트북 전반이면
dotenvx를 안 쓰는 프로젝트가 대부분이고 그쪽 `.env` 는 평문이다.

판별 규칙: 파일 선두 64KB를 읽어 `#` 로 시작하는 주석 행과 빈 행을 버린 뒤,
남은 `KEY=VALUE` 행을 본다. `DOTENV_PUBLIC_KEY` 행이 존재하고, 그 행을 제외한
모든 값이 `encrypted:` 로 시작하면 암호문으로 판정한다. 하나라도 아니면 평문이다.
`KEY=VALUE` 행이 하나도 없으면(빈 파일, 주석뿐) 평문으로 보되 차단하지 않는다 —
읽어도 새는 것이 없다.

부수 효과가 본래 가치보다 클 수 있다. 깔아두면 평문 `.env` 를 쓰는 프로젝트가
에이전트가 그걸 건드리는 순간마다 드러나고 마이그레이션으로 유도된다.

### 차단 방식

거부만 하면 에이전트는 우회를 시도한다. 거부 메시지에 올바른 대안을 넣어
스스로 교정하게 한다.

```text
dotenvx get DART_API_KEY 는 평문을 대화 컨텍스트에 남깁니다.
값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- python 06_Scripts/fetch_dart.py
```

```text
이 프로젝트의 .env 는 아직 평문입니다.
  aeg init
로 암호화한 뒤 다시 시도하세요.
```

### 예외

- 확장자 예외: `.env.example`, `.env.template`, `.env.sample`
- 사용자 예외: `~/.config/aeg/allow` 에 경로를 한 줄씩. 해당 경로
  이하는 판정을 건너뛴다.

## CLI 표면

MVP는 네 개다.

### `aeg install`

`~/.claude/settings.json` 에 PreToolUse 훅을 건다. 기존 설정을 보존하며
병합하고, 멱등이라 두 번 돌려도 중복이 생기지 않는다. 노트북당 한 번.

### `aeg scan [경로]`

기본값 홈 디렉터리 아래를 훑어 평문 `.env` 를 쓰는 프로젝트를 목록으로 낸다.
사용자가 어느 프로젝트에 뭐가 있는지 기억할 필요를 없앤다.
"노트북 전반"이라는 요구에 직접 답하는 명령이고 실질적 첫 실행 경험이다.

`node_modules`, `.git`, `vendor`, `dist`, `build` 는 건너뛴다.

### `aeg init [경로]`

프로젝트 하나를 마이그레이션한다.

1. dotenvx 설치 확인. 없으면 설치 방법을 안내하고 중단.
2. `dotenvx encrypt` 로 기존 `.env` 를 제자리 암호화 (`.env.keys` 자동 생성).
3. `.gitignore` 에 `.env.keys` 추가. 없으면 생성.
4. `.env` 가 `.gitignore` 에 있으면 **제거하지 않고 안내만 한다.**
   암호화 후에는 커밋해도 안전하지만 자동 제거는 사고 위험이 크다.
   커밋하지 않아도 도구는 정상 동작한다.
5. `git log --all -- .env` 로 평문 `.env` 의 과거 커밋 여부를 확인한다.
   발견되면 경고하고 해당 키 로테이션을 권고한다.

5번이 중요하다. 암호화해도 히스토리의 평문은 남는다. 실무에서 가장 자주
놓치는 지점이고, 암호화했다는 이유로 안심시키면 안 된다.

평문 `.env` 백업 파일은 만들지 않는다. 평문이 하나 더 남을 뿐이다.

### `aeg hook`

훅이 내부적으로 호출하는 판정기. stdin으로 도구 호출 정보를 받아 판정한다.
사용자가 직접 부를 일은 없다.

### 범위 밖 (나중에)

`uninstall`, `doctor`. 필요해지면 붙인다.

## 훅 어댑터

Claude Code 공식 문서로 확인한 규약이다.

**stdin JSON** (PreToolUse):

```json
{
  "session_id": "...",
  "cwd": "/current/working/directory",
  "permission_mode": "default|plan|acceptEdits|auto|dontAsk|bypassPermissions",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": { "command": "cat .env.keys" },
  "tool_use_id": "toolu_01ABC..."
}
```

도구별 `tool_input`: Bash 는 `command`, Read 는 `file_path`,
Grep 은 `pattern` 과 `path`.

**출력은 JSON 방식을 쓴다.** exit 2 + stderr 도 차단이 되지만 채택하지 않는다.

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "교정 메시지"
  }
}
```

이유는 fail-open 이다. 종료코드를 판정 신호로 쓰면 **의도적 차단(exit 2)과
버그로 인한 비정상 종료가 구분되지 않는다.** 판정기가 크래시했을 때
에이전트의 모든 툴 호출이 막히는 것이 최악의 실패다. JSON 방식은 항상 exit 0
으로 끝나고 판정을 본문에 담는다.

**JSON 은 차단(`deny`)일 때만 낸다.** 허용·파싱 실패·알 수 없는 도구·내부
오류는 stdout 에 **아무것도 쓰지 않는다** — "판정 없음"이며, Claude Code 의
평소 권한 흐름이 그대로 적용된다. `permissionDecision: "allow"` 는 "판정
없음"이 아니라 권한 확인 프롬프트를 건너뛰는 적극적 승인이다. 이것을 내면
aeg 를 설치하는 것만으로 `rm -rf`, `git push --force` 같은 모든 Bash 호출이
자동 승인되므로 절대 내지 않는다.

**settings.json 등록 형태:**

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|Read|Grep",
        "hooks": [
          { "type": "command", "command": "/usr/local/bin/aeg", "args": ["hook"] }
        ]
      }
    ]
  }
}
```

`args` 를 명시하면 exec form 이 되어 셸을 경유하지 않는다. 셸 기동 비용이
사라져 10ms 예산에 여유가 생긴다.

어댑터 인터페이스:

```go
type ToolKind int // Bash | FileRead | ContentSearch

type ToolCall struct {
    Kind    ToolKind
    Command string // Bash
    Path    string // FileRead, ContentSearch
    Pattern string // ContentSearch
    Cwd     string
}

type Decision struct {
    Allow  bool
    Reason string // 차단 시 교정 메시지
}

type Adapter interface {
    Parse(stdin io.Reader) (ToolCall, error)
    Emit(w io.Writer, d Decision) error
}
```

판정 엔진은 `Decide(ToolCall) Decision` 하나다. 어댑터에 의존하지 않는다.
`Emit` 은 종료코드를 반환하지 않는다. 항상 0이다. `Emit` 은 `Allow` 인
판정에 대해 아무것도 쓰지 않는다.

## 오류 처리

**판정 불가 시 통과시킨다(fail-open).** 훅이 stdin 파싱에 실패하거나
설정 파일이 깨졌을 때 차단하면 에이전트의 모든 툴 호출이 막혀 작업이
불가능해진다. 우발적 노출 차단이 목표이지 가용성을 희생할 문제가 아니다.

구현상으로는 항상 exit 0 으로 끝내고 stdout 에 아무것도 쓰지 않는다(판정 없음).
패닉은 최상위에서 recover 하여 출력 없이 끝낸다. 경고는 stderr 에 남긴다.

예외: `.env.keys` 경로 매칭처럼 판정이 명확한 경우는 파싱 실패와 무관하게
차단한다.

- `dotenvx` 미설치: `init` 에서만 오류. 훅은 영향 없음.
- `~/.claude/settings.json` 부재: `install` 이 생성한다.
- `settings.json` 이 JSON이 아님: `install` 중단, 수동 수정 안내. 덮어쓰지 않는다.
- `scan` 중 권한 없는 디렉터리: 건너뛰고 계속. 마지막에 건너뛴 수를 보고.

## 테스트 전략

- **판정 엔진 — 테이블 드리븐 단위 테스트.** 명령 문자열 → allow/deny.
  차단 6종과 허용 4종 각각에 대해 정상 케이스, 공백·따옴표 변형,
  파이프·리다이렉션 조합. 여기가 가장 중요하고 가장 촘촘해야 한다.
- **`.env` 암호화 판별기** — 픽스처 파일(암호문, 평문, 빈 파일, 혼합,
  `DOTENV_PUBLIC_KEY` 만 있고 값은 평문)로 검증.
- **예외 처리** — `.env.example` 류, `allow` 목록 경로.
- **`settings.json` 병합 멱등성** — 빈 파일, 기존 훅 있음, 이미 설치됨,
  다른 훅과 공존. 두 번 실행 후 결과가 한 번과 같아야 한다.
- **`init` 의 `.gitignore` 처리** — `.gitignore` 없음, `.env` 만 있음,
  `.env.keys` 이미 있음.
- **통합** — 실제 Claude Code 훅 형식의 JSON을 stdin에 넣고 종료코드와
  stderr를 확인.
- **성능** — 판정 1회가 10ms 이내인지 벤치마크.

## 결정 기록

설계 시점의 미해결 4건은 모두 조사로 해소했다.

1. **dotenvx 라이선스: BSD-3-Clause** (Copyright 2024, Scott Motte).
   상업 이용·재배포·의존 도구 제작에 제약이 없다. 더구나 우리는 dotenvx를
   링크하지 않고 **서브프로세스로 호출**하므로 라이선스 전파 문제 자체가
   발생하지 않는다. 바이너리를 번들하지 않고 사용자가 별도 설치한다.
2. **훅 스키마: 확인 완료.** 「훅 어댑터」 참조. 이 조사 결과로 설계가
   세 군데 바뀌었다 — Grep 매처 추가(보안 구멍), JSON 출력 채택(fail-open),
   `args` exec form(성능).
3. **`aeg scan` 탐색 범위: 결정.** 아래 참조.
4. **`aeg` 이름: 사용 가능.** PATH에 없고 Homebrew 포뮬러도 없다
   (유사 검색 결과 `libaegis`, `yaegi`, `agg`, `reg`, `peg` — 충돌 아님).

### `aeg scan` 탐색 범위

인자가 없으면 `$HOME` 부터 훑는다. 첫 실행에서 사용자에게 묻지 않는다 —
설치하고 바로 값을 봐야 한다.

느려지지 않게 세 가지를 건다.

- **깊이 제한 기본 6.** `--depth` 로 조정.
- **하드 제외**: `node_modules`, `.git`, `vendor`, `dist`, `build`, `target`,
  `.venv`, `venv`, `.cache`, `.Trash`, `go/pkg`, 그리고 macOS의 `~/Library`.
- **이름 매칭 우선.** `.env` 계열 파일명만 먼저 찾고, 내용 읽기는 후보에만 한다.

네트워크 동기화 폴더(Google Drive, Dropbox, iCloud)는 지연이 크므로 만나면
경고하고 계속한다. 전체가 느리면 `aeg scan ~/projects` 로 좁히라고 안내한다.

## 사전 준비

구현 시작 전에 로컬에 없어서 설치해야 하는 것.

- **Go** — 미설치. `brew install go`
- **dotenvx** — 미설치. `brew install dotenvx`
