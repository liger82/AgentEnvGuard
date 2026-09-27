# AgentEnvGuard 조사 기록: dotenvx 의 OS 키체인 전환과 출력 리댁션 검토

작성일: 2026-09-27
대상 브랜치: `feat/aeg-mvp`
상태: **조사만 완료.** 코드를 바꾸지 않았고, 대응 방침도 결정하지 않았다.

이 문서는 2026-09-27 세션에서 조사한 내용을 남긴다. `docs/decisions-2026-09-13.md`
가 1차 개발에서 **왜 그렇게 만들었는지**를 담는다면, 이 문서는 그 뒤 2주 동안
**전제 하나가 어떻게 무너졌는지**와 검토했으나 채택하지 않은 것을 담는다.

---

## 1. 요약

1. **dotenvx 가 2.25.0(2026-09-14)부터 개인키를 `.env.keys` 가 아니라 OS 시크릿
   저장소에 기본 저장한다.** aeg 의 1차 개발 결정이 굳은 날짜가 2026-09-13 이다.
   하루 차이로 aeg 의 핵심 전제가 기본값에서 밀려났다.
2. **그 결과 에이전트 관점의 보호가 오히려 약해졌다.** aeg 가 지키던 단 하나의
   파일이 생기지 않고, 대신 aeg 가 막지 않는 한 줄 명령으로 개인키를 꺼낼 수 있다.
   macOS 확인창 장벽은 없다(실측).
3. **출력 리댁션(output redaction)은 검토 후 보류했다.** 구멍을 실제로 덮지만
   aeg 가 개인키 값을 읽어야 하고, 세션을 영구히 망가뜨리는 실패 양식이 있다.

---

## 2. 문서 관계

| 문서 | 역할 |
|---|---|
| `docs/decisions-2026-09-13.md` | 1차 개발 판정과 남은 이슈 |
| `docs/dotenvx-guide.md` | 사용자용 dotenvx 안내. **§4 기준으로 일부 낡음** |
| `README.md` | 사용자용 차단 범위와 한계. **§4 기준으로 일부 낡음** |
| 이 문서 | 2026-09-27 조사 결과와 미결정 항목 |

---

## 3. dotenvx 의 OS 시크릿 저장소 전환

### 3.1 기능 개요

개인키를 `.env.keys` 파일 대신 OS 시크릿 저장소에 둔다. macOS 키체인,
Windows Credential Manager, Linux Secret Service 를 모두 다룬다.

```
dotenvx native up     # 키를 OS 시크릿 저장소에 저장
dotenvx native down   # OS 시크릿 저장소에서 .env.keys 로 되돌림
dotenvx native push   # .env.keys 에서 저장소로 밀어 넣음
dotenvx native pull   # 저장소에서 .env.keys 로 꺼내옴
```

`dotenvx run` 은 저장소에서 키를 찾으므로 실행 방식은 그대로다.
`--no-native` 를 주면 예전처럼 파일만 쓴다. `encrypt` 와 `set` 에 이 플래그가 있다.

Linux 는 `secret-tool`(보통 `libsecret-tools` 패키지), 동작 중인 Secret Service,
사용자 D-Bus 세션이 필요하다. 없으면 `.env.keys` 로 폴백한다.

### 3.2 버전 연혁 (변경 로그 기준)

| 버전 | 날짜 | 변화 |
|---|---|---|
| 2.2.0 | 2026-07-07 | macOS 키체인 지원 시작 (`keychain up`/`down`) |
| 2.3.0 | 2026-07-08 | BREAKING: `keychain` 을 `native` 로 개명 |
| 2.5.0~2.7.0 | 2026-07-11~13 | Windows, Linux 로 확장 |
| **2.25.0** | **2026-09-14** | **기본 저장 위치가 `.env.keys` → OS 시크릿 저장소** |
| 2.26.x | 2026-09-15 | 1Password, Bitwarden 도 보관처로 추가 |
| 2.27.0 | 2026-09-16 | BREAKING: `run --validate` 와 `validate` 명령 제거 |
| 2.28.0 | 2026-09-16 | 키 보관처 선택 모델 문서화 |
| 2.28.1 | 2026-09-19 | 키를 argv 대신 stdin 으로 전달(보안 수정) |
| 2.28.2 | 2026-09-19 | `.env.keys` 를 소유자만 읽고 쓰게 변경 |
| 2.29.0 | 2026-09-21 | `dotenvx protect` 추가. `gitignore`·`precommit` 폐기 예정 |
| 2.30.0 | 2026-09-22 | 조사 시점 최신 |

개발 환경에 설치된 버전은 2.24.1(2026-09-11)로, 아직 예전 동작이다. 그러나
README 가 안내하는 `brew install dotenvx/brew/dotenvx` 는 최신을 설치한다.
**오늘 README 를 따라 처음 설치하는 사용자는 이미 키체인 기본값을 받는다.**

### 3.3 실측 검증

임시 디렉터리에 2.30.0 을 따로 설치해 확인했다. 전역 2.24.1 은 건드리지 않았다.

| 확인 항목 | 결과 |
|---|---|
| `.env.keys` 생성 여부 | **생성되지 않음** |
| 개인키 저장 위치 | macOS 로그인 키체인 (`login.keychain-db`) |
| 키체인 클래스 | `genp` (generic password) |
| 서비스 이름 (`svce`) | `dotenvx` |
| 계정 이름 (`acct`) | **공개키 전체 hex 문자열** |
| 표시 라벨 | `dotenvx (공개키 앞자리를 나눈 형태)` |
| `.env.keys` 없이 `dotenvx run` | 정상 복호화, 평문 값 주입됨 |

`.env` 는 기존과 같다. 맨 위에 `DOTENV_PUBLIC_KEY`, 값은 `encrypted:` 접두어.

**계정 이름이 공개키라는 점이 핵심이다.** 공개키는 `.env` 에 평문으로 적혀 있고,
aeg 는 암호화된 `.env` 읽기를 의도적으로 허용한다. 키체인 항목을 찾는 인덱스가
aeg 가 허용하는 파일에 그대로 들어 있다.

### 3.4 확인창 장벽은 없다

조사 초기에 "macOS 는 앱별 접근 제어가 있으니 GUI 확인창이 에이전트를 막아줄
것"이라는 가설을 세웠다. **틀렸다.** 평범한 셸에서 아래 한 줄로 64자 hex
개인키가 확인창 없이 즉시 나왔다.

```
security find-generic-password -s dotenvx -a <공개키> -w
```

원인은 dotenvx 가 `security` 명령을 통해 키를 저장하기 때문이다. 그러면
`security` 자체가 그 항목의 신뢰 앱 목록에 들어간다. 2.28.1 의 "키를 argv 대신
stdin 으로 전달"이 그 구현을 뒷받침한다.

같은 함정이 Go 라이브러리에도 있다. `zalando/go-keyring` 은 macOS 에서
`/usr/bin/security` 를 실행한다. 이 라이브러리만으로는 접근 제어가 생기지 않는다.
실제 접근 제어를 구현한 사례(`agentsecrets`)는 별도 데몬을 두고 유닉스 도메인
소켓으로 호출자 PID 를 커널에 물어 실행 파일 SHA-256 을 대조한다. Linux 는
`SO_PEERCRED`, macOS 는 `LOCAL_PEERPID` 를 쓰므로 **Windows 에는 이식되지 않는다.**

### 3.5 탈취 경로

추측도 우회도 필요 없는 두 단계다.

1. `cat .env` 로 공개키를 읽는다 (aeg 가 허용)
2. 그 값을 `-a` 에 넣어 `security find-generic-password` 를 실행한다 (aeg 가 허용)

### 3.6 aeg 훅 실측

현재 브랜치를 빌드해 훅에 PreToolUse JSON 을 직접 넣었다.

| 명령 | aeg 판정 |
|---|---|
| `security find-generic-password -s dotenvx -a <공개키> -w` | 통과 |
| `cat .env` (암호화된 상태) | 통과 |
| `dotenvx native pull` | 통과 |
| `dotenvx native down` | 통과 |
| `dotenvx keypair` (대조군) | 차단 |

`native pull` 과 `native down` 은 키를 다시 `.env.keys` 파일로 꺼낸다. 기존에
차단하는 `keypair` 와 위험도가 같은데 막히지 않는다. `1password down|pull` 과
`bitwarden down|pull` 도 같은 부류다(2.26.x 에서 추가).

---

## 4. aeg 에 생긴 어긋남

위협 모델별로 방향이 갈린다. **git 유출 관점에서는 키체인이 낫다.** 커밋할 파일이
아예 없으니 실수로 커밋하거나 zip 으로 묶거나 클라우드 동기화될 일이 없다.
**그러나 에이전트 관점에서는 지금이 더 나쁘다.** 지키던 파일이 사라지고 막히지
않는 한 줄이 열렸다. 이건 의도적 탈취가 아니라 우발적 노출 범주이고, aeg 가
목표로 삼은 바로 그 범주다.

구체적 어긋남은 다섯 개다.

1. **1순위 보호 대상이 존재하지 않을 수 있다.** 새로 `dotenvx encrypt` 를 하면
   `.env.keys` 가 생기지 않는다. 훅의 핵심 규칙, `DecideUnparsed` 의 `.env.keys`
   문자열 검사, `aeg scan` 의 `.env.keys` gitignore 보고, `aeg init` 의 gitignore
   추가가 모두 없는 파일을 상대로 돈다.
2. **새 유출 경로가 열렸다.** macOS `security find-generic-password`, Windows
   PowerShell 자격증명 조회, Linux `secret-tool lookup`. Bash 판정에 없다.
3. **새 위험 명령이 생겼다.** `native down|pull`, `1password down|pull`,
   `bitwarden down|pull`. 기존 `get`·`decrypt`·`keypair` 와 같은 부류다.
4. **`aeg init` 의 gitignore 작업이 중복된다.** dotenvx 가 `gitignore` 와
   `precommit` 을 폐기하고 `dotenvx protect` 로 대체했다(2.29.0).
5. **`docs/dotenvx-guide.md` 의 저장 위치 표가 틀렸다.** 개인키가 `.env.keys` 에
   있다고 적혀 있다. `README.md` 의 차단 범위 설명도 같은 전제에 서 있다.

부수적으로, 결정 기록 21번이 조사한 `validate` 는 2.27.0 에서 제거됐다. 차단
대상은 아니었으니 영향은 작다.

---

## 5. 출력 리댁션(output redaction) 검토 — 보류

### 5.1 무엇인가

도구 실행 **뒤**, 결과가 모델에게 돌아가기 직전에 시크릿처럼 보이는 값을 가리는
기법이다. 영어로 output redaction 또는 tool-output redaction 이라 하고,
`agent-guard` 는 output masking 이라 부른다.

Claude Code 의 메커니즘은 PostToolUse 훅이다. 표준입력으로 `tool_response` 를
받고 아래 필드로 교체본을 낸다.

```json
{"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedToolOutput": ...}}
```

**필드 이름 주의.** 공식 문서 페이지가 길어 잘려 읽히는 탓에
`updatedToolResult` 라는 잘못된 이름이 돌아다닌다. 실제로 동작하는 구현이 쓰는
이름은 `updatedToolOutput` 이다. 이 확인은 공식 문서에서 직접 못 했고,
`agent-guard` 의 프로덕션 구현 소스를 읽어 교차 검증했다. 도입한다면 실측으로
한 번 더 확인해야 한다.

### 5.2 aeg 에 도입할 때의 장단점

**장점**

- **PreToolUse 가 원리적으로 못 보는 것을 본다.** 사전 검사는 명령 문자열과
  경로만 본다. 무엇이 출력되는지는 영원히 못 본다. 출력 시점 검사는 명령을 어떻게
  썼는지와 무관하므로 README 가 한계로 명시한 것들이 한꺼번에 잡힌다.
  `d=dotenvx; $d get MY_KEY` 우회, 코드가 환경변수를 직접 출력하는 경우,
  `git show HEAD:.env`, `cd DIR &&` 추적 실패, 주입된 셸에서의 맨 `printenv`.
- **aeg 는 gitleaks 방식보다 유리하다.** `agent-guard` 는 정규식으로 "시크릿처럼
  보이는 것"을 추측한다. aeg 는 개인키가 어디 있는지 알기 때문에 **정확히 일치
  비교**할 수 있다. 오탐과 미탐이 모두 없다. 막을 대상이 하나로 좁아
  과탐 문제도 구조적으로 발생하지 않는다. 외부 의존성도 필요 없다.

**단점과 위험**

- **aeg 가 평문 개인키를 만지는 프로그램이 된다.** 지금은 키 값을 절대 읽지
  않고 파일을 분류만 한다. 정확 일치 리댁션은 훅이 키를 읽어야 하고, 그러면
  모든 툴 호출마다 개인키가 훅 프로세스 메모리에 올라간다. aeg 자신의 stderr 나
  크래시 덤프로 새는 경로가 생기고 "aeg 는 시크릿 매니저가 아니다"는 서사가
  흔들린다. **이것이 보류의 주된 이유다.**
  - 완화책은 있다. `envsitter` 방식대로 `aeg init` 시점에 **한 번만** 읽어 HMAC
    지문만 저장하고, 훅에서는 후보 문자열의 지문을 비교한다. 평상시 훅에 키 값이
    올라가지 않는다.
- **판별자 함정이 치명적이고 영구적이다.** Claude 의 content block 은
  discriminated union 이라 `type`·`media_type`·`mimeType` 값이 블록 모양을
  결정한다. 이 값을 마스킹하면 이후 모든 요청이 `Input tag '[REDACTED]' ... does
  not match any of the expected tags` 로 거부된다. 호스트가 바뀐 블록을 대화
  기록에 **영속화**하므로 재시도해도 같은 블록이 재생되어 세션이 영구히 망가진다.
  `agent-guard` 가 실제로 이 사고를 겪었다. 회피책은 키 이름이 아니라 값 자체를
  프로토콜 토큰 허용 목록과 대조하는 것이다. `image/png` 와 같은 시크릿은 없으므로
  구멍이 되지 않는다는 논리다. 미묘하다.
- **설계 순수성을 잃는다.** 현재 `Decide` 는 파일 읽기 외 부작용이 없는 거의 순수
  함수다. 출력 리댁션은 두 번째 훅 등록, 임의 모양 JSON 트리 순회, 값 로딩을
  요구한다. PostToolUse 는 PreToolUse 가 막은 호출에는 발동하지 않으므로 기존
  판정 경로와 겹치지 않는 별개 서브시스템이고 테스트 표면도 따로 생긴다.
- **성능 부담이 다른 차원이다.**

  | 항목 | 현재 aeg | 출력 리댁션 |
  |---|---|---|
  | 호출당 읽는 양 | 최대 64KB | 응답 전체, 최대 10MiB |
  | 호스트 시간 예산 | 여유 (호출당 약 1.7ms) | 약 20초, 초과 시 fail-open |

- **원리적으로 이미 늦었다.** 명령이 이미 실행됐다. 값은 이미 자식 프로세스에
  있고 파일에 써졌거나 네트워크로 나갔을 수 있다. 리댁션은 모델 컨텍스트 진입만
  막는다. `agent-guard` 주석 그대로 완료된 결과를 회수할 수는 없다.
- **정직한 한계 서술이 흐려진다.** README 가 "우발적 노출 차단이 목표, 그 이상
  기대 말라"고 적어둔 것이 강점이다. 출력 리댁션이 붙으면 사용자가 의도적 탈취도
  막힌다고 오해한다. base64 로 감싸거나 쪼개거나 파일로 쓰면 그대로 나간다.

### 5.3 판단

**보류.** 도입한다면 범위를 아주 좁게 잡는다. 개인키 지문 일치만, 판별자는 값
허용 목록으로, 모든 예외는 fail-open 으로, 기본 비활성 플래그 뒤에. 광범위
시크릿 스캔은 하지 않는다. aeg 의 강점은 정확히 하나의 값을 안다는 것이지
추측이 아니다.

`agent-guard` 가 문서에 명시한 한계도 참고할 것. 도구 응답 JSON 의 문자열 값만
훑으므로 시크릿이 객체 **키** 위치에 오면 안 가려진다. 키를 바꾸면 서로 다른 키가
한 자리로 합쳐지기 때문에 일부러 안 한다.

---

## 6. 동일 목적 오픈소스 조사

aeg 의 조합인 "제자리 암호화 + 훅 차단"을 그대로 하는 프로젝트는 찾지 못했다.
접근 방식이 다섯 갈래로 나뉜다.

| 갈래 | 프로젝트 | 방식 |
|---|---|---|
| 훅 차단 (aeg 와 같은 부류) | `JeongJaeSoon/agent-guard` | Claude Code·Codex 플러그인. 셸 스크립트. gitleaks 로 출력 리댁션까지. git hook·CI 공유 |
| | `file-guard`, `JohnXu22786/secret-guard`, `boxpositron/envsitter-guard` | 각각 Claude Code·dsh·OpenCode 용. `.env` 류 접근을 통째로 차단 |
| 에이전트 래핑 주입 | `jordanburke/agent-env` | bash. SOPS·age 로 암호화, 에이전트 프로세스를 감싸 환경변수 주입. 차단 훅 없음 |
| | 1Password `op run` | 같은 원리, 볼트가 1Password. 사실상 표준 관행 |
| 브로커 (값을 아예 안 줌) | `The-17/agentsecrets` | Go. OS 키체인 + 프로세스 해시 검증 데몬. Unix 전용 |
| | `Brissux-Labs/agent-secrets` | TypeScript. Bitwarden 백엔드, MCP 로 실행 도구만 노출. 프리릴리스 |
| argv 치환 + 출력 리댁션 | `lthoangg/secretsh` | Rust. 명령 안의 자리표시자를 실행 직전 치환, stdout·stderr 리댁션. `.env` 자체는 미보호 |
| 값 없이 검사만 | `boxpositron/envsitter` | TypeScript. 키 목록, HMAC 지문, 참·거짓 질의만 허용 |
| 사후 정리 | `Ishannaik/agent-sweep` | 이미 대화 기록에 새어 나간 시크릿을 찾아 지움 |

가장 가까운 것은 `agent-guard` 이고, 거기서 가져올 만한 것이 출력 리댁션이다(§5).
차단 관점에서 aeg 가 더 나은 점은 `.env` 를 통째로 막지 않고 값만 암호화해서
에이전트가 **어떤 키가 있는지는 보되 값은 못 보게** 한다는 것이다.

`agentsecrets` 의 OS 지원 범위는 계층마다 다르다. 저장 계층은
`zalando/go-keyring` 으로 세 OS 를 덮지만, 프로세스 해시 검증은 Unix 전용이다
(§3.4). Windows 에서는 Credential Manager 에 넣어두는 수준까지만 된다. Windows
전용으로 같은 일을 하는 `dev-foundations/agent-secrets` 가 따로 있다.

---

## 7. aeg 가 키체인을 직접 구현하는 안 — 불필요

세션 초반에 검토한 안이다. `aeg init` 이 `.env.keys` 를 키체인에 넣고 파일을
지우며, `aeg run` 이 키를 꺼내 환경변수로 `dotenvx run` 을 실행하는 구조였다.

**dotenvx 가 이미 한다(§3). 만들 필요가 없다.** 게다가 기대했던 접근 제어 이득도
실측으로 없음이 확인됐다(§3.4). 남는 이득은 "개인키가 프로젝트 폴더 밖에 있다"
하나이고, 그건 dotenvx 기본값으로 이미 얻는다.

Windows 지원을 목표로 한다면 키체인보다 **PowerShell 명령 판정이 먼저다.**
현재 Bash 문법 기준 파서로는 PowerShell 명령을 제대로 보지 못한다.
홈 디렉터리는 `os.UserHomeDir` 로 잡으므로 문제없다.

---

## 8. 미결정 항목

아무것도 결정하지 않았다. 대응하려면 각 항목이 별도 설계를 필요로 한다.

1. **`security find-generic-password` 차단 추가.** 가장 시급. Windows·Linux
   대응 명령(`secret-tool lookup`, PowerShell 자격증명 조회)도 같이 봐야 한다.
2. **`native down|pull`, `1password down|pull`, `bitwarden down|pull` 차단.**
3. **`cat .env` 는 계속 허용한다는 판단의 재확인.** 공개키는 설계상 공개이고
   git 에 커밋되므로 막아도 의미가 없다. 닫아야 하는 곳은 읽기 명령 쪽이다.
4. **`.env.keys` 부재를 정상으로 다루도록 `aeg scan`·`aeg init` 수정.**
   현재는 부재를 안전으로 읽을 수 있다.
5. **`README.md` 와 `docs/dotenvx-guide.md` 의 저장 위치·차단 범위 갱신.**
6. **`dotenvx protect` 로 대체된 gitignore 처리 정리.**
7. **출력 리댁션(§5).** 보류. 위 1~6 보다 뒤.
8. **훅 오탐 하나.** 이 문서를 heredoc 으로 작성하려다 aeg 훅에 차단됐다.
   개인키 환경변수 이름이 본문에 들어 있어 `msgPrivateKey` 규칙이 걸렸다.
   문서를 쓰는 것은 값을 출력하는 것이 아니다. 판정이 명령의 의도(출력 대상)와
   단순 문자열 포함을 구분하지 못한다. 결정 기록 26번이 리다이렉션 대상은
   읽기로 치지 않도록 고친 것과 같은 부류의 문제다.

우선순위 판단: 출력 리댁션은 없는 기능을 추가하는 일이지만, 1~6 은 현재 문서와
판정 규칙이 최신 dotenvx 에서 **틀린 안내를 하는 상태**다. 후자가 급하다.

---

## 9. 검증 기록과 재현 절차

2026-09-27 에 실행한 것. 코드와 전역 dotenvx 2.24.1 은 바꾸지 않았다.

1. 임시 디렉터리에 `npm install @dotenvx/dotenvx@latest` → 2.30.0.
2. 테스트용 가짜 값을 담은 평문 `.env` 를 만들고 `dotenvx encrypt`.
   → `◈ encrypted (.env)`. `.env.keys` 생성되지 않음.
3. `security dump-keychain | grep -i dotenv` 로 항목 확인.
   `svce="dotenvx"`, `acct=<공개키>`, `login.keychain-db`.
   비밀값을 읽지 않는 조회이므로 확인창이 뜨지 않는다.
4. `dotenvx run -- sh -c '...'` 로 주입된 값 확인 → 평문 출력. 키체인 복호화 확인.
5. `security find-generic-password -s dotenvx -a <공개키> -w` → 64자 hex 즉시 반환,
   확인창 없음. 10초 타임아웃을 걸어 GUI 프롬프트로 인한 대기와 구분했다.
6. `go build ./cmd/aeg` 후 훅에 PreToolUse JSON 직접 입력 → §3.6 표.
7. 정리: `security delete-generic-password -s dotenvx -a <공개키>` 로 테스트 항목
   삭제하고 조회되지 않음을 확인. 임시 디렉터리와 빌드 산출물 삭제.
   `git status` 로 작업 트리에 변화 없음 확인(기존 README 변경 제외).

주의: 5번은 실제 개인키를 반환한다. 재현할 때 값을 출력하거나 로그에 남기지 말 것.
이 조사에서는 길이와 hex 형식 일치 여부만 확인했다.
