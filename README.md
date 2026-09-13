# AgentEnvGuard

코딩 에이전트가 시크릿 평문을 읽지 못하게 막는 Claude Code 가드레일.

- **시크릿 매니저가 아니다.** 
- 볼트·암호화·주입은 [dotenvx](https://dotenvx.com) 가 한다. 
- AgentEnvGuard 는 그 위에 얹혀서,
에이전트가 규칙을 지키도록 강제하는 얇은 층이다.
(같은 이름의 [amannirala13/envguard](https://github.com/amannirala13/envguard)
와는 무관한 별개 프로젝트다.)

## 차단 범위

에이전트가 `cat .env` 한 번을 하면 시크릿이 대화 로그와 컨텍스트에 남고,
그 뒤로 어디로 흘러가는지 통제할 수 없다. AgentEnvGuard 는 Claude Code 의
PreToolUse 훅으로 `Bash`, `Read`, `Grep` 을 검사해 다음을 막는다.

- `.env.keys` 읽기 (dotenvx 개인키) — 따옴표로 감싼 경로, `Grep` 의 `glob` 포함
- **평문** `.env` 읽기 — 암호화된 `.env` 는 값이 암호문이라 그대로 읽게 둔다
- `dotenvx get`, `dotenvx decrypt`, `dotenvx keypair` (개인키를 그대로 출력한다)
  — `npx @dotenvx/dotenvx`, `pnpm exec`, `sudo` 같은 래퍼 뒤에 와도 막는다
- `dotenvx run -- printenv`, `dotenvx run -- env` 처럼 환경변수를 출력하는 자식 명령
  — `dotenvx run` 은 기본으로 **평문 값을 그대로** 주입한다
- `DOTENV_PRIVATE_KEY` 출력

`Grep` 을 포함하는 것이 중요하다. Grep 은 매칭된 줄 내용을 반환하므로
`Grep(pattern="DOTENV_PRIVATE_KEY", path=".env.keys")` 한 번이면
Bash 도 Read 도 거치지 않고 개인키가 나온다.

차단할 때는 거부만 하지 않고 올바른 대안을 함께 알려준다. 그러지 않으면
에이전트가 우회를 시도한다.

## 한계

**의도적 탈취는 막지 못한다.** 명령 문자열 매칭이므로 이런 것은 통과한다.

```bash
d=dotenvx; $d get MY_KEY
```

bash 를 실행할 수 있는 에이전트에게서 값을 완전히 숨기는 것은 불가능하다.
이 도구의 목표는 **우발적 노출 차단**이다. 그 이상을 기대하면 안 된다.

그 밖에 알려진 한계:

- 광범위한 검색 — `Grep` 의 path 가 없거나 디렉터리이고 glob 도 `.env.keys`
  를 가리키지 않는 경우 — 은 **평문** `.env` 안의 매칭된 줄을 드러낼 수 있고,
  검색 도구가 숨김·gitignore 파일까지 포함하도록 설정되어 있으면 `.env.keys`
  의 줄도 드러낼 수 있다. 막는 것은 `DOTENV_PRIVATE_KEY` 를 노린 패턴과
  `.env.keys`·평문 `.env` 를 직접 가리키는 path, `.env.keys` 를 가리키는 glob 이다. 광범위한 검색 자체를
  막으면 에이전트의 주된 검색 기능이 망가지므로, 해법은 검색 차단이 아니라
  `aeg scan` / `aeg init` 으로 평문 `.env` 자체를 없애는 것이다.
- 정규식으로 흐린 패턴(예: `DOTENV_PRIVATE_KE[Y]`)은 패턴 검사를
  피해간다. 의도적 탈취와 같은 부류의 우회다.
- 명령 안의 `cd DIR &&` 는 추적하지 않는다. `cd other && cat .env` 는 훅이
  받은 작업 디렉터리 기준으로 판정한다.
- git 히스토리 읽기(`git show HEAD:.env`, `git diff .env`)는 막지 않는다.
- `dotenvx run` 없이 실행한 `env`, `printenv` 는 막지 않는다. 그 셸에 시크릿이
  주입되어 있지 않다면 드러날 값이 없고, 막으면 오탐이 너무 많다.
- 암호화는 git 히스토리에 이미 커밋된 평문까지 지우지 않는다. `aeg init`
  은 히스토리에서 평문 `.env` 커밋을 발견하면 경고하지만, 실제로는 해당
  키를 발급처에서 로테이션해야 한다.
- `aeg install` 을 다시 실행해도 이미 등록된 훅의 matcher 나 args 를
  누가 손으로 고쳐놓은 것까지 복구하지 않는다. 의심되면
  `~/.claude/settings.json` 을 직접 확인하라.

## 설치 방법

```bash
brew install dotenvx/brew/dotenvx   # 볼트는 dotenvx 가 맡는다
go install github.com/liger82/AgentEnvGuard/cmd/aeg@latest
aeg install                         # 노트북당 한 번
```

`aeg install` 은 `~/.claude/settings.json` 을 읽어 PreToolUse 훅을
병합한다. 처음으로 파일을 바꿀 때 원본을 `~/.claude/settings.json.aeg-backup`
에 남기고, 백업이 이미 있으면 덮어쓰지 않는다 — 설치 전 최초 상태가 보존된다.
재직렬화 과정에서 기존 키 순서가 알파벳순으로 바뀌니 diff 를 볼 때
당황하지 말 것. 문제가 생기면 되돌린다.

```bash
cp ~/.claude/settings.json.aeg-backup ~/.claude/settings.json
```

## 사용 방법

```bash
aeg scan                      # 평문 .env 를 쓰는 프로젝트를 전부 찾는다
aeg init ~/projects/myapp     # 하나를 dotenvx 로 마이그레이션한다
dotenvx run -- python app.py  # 이제부터 스크립트는 이렇게 실행한다
```

`dotenvx run` 은 값을 가리지 않는다. 스크립트가 값을 출력하면 그대로
보이므로, 실행하는 명령 자체가 시크릿이나 환경변수를 출력하지 않게 한다.

키가 어디에 저장되는지, 코드에서 환경변수를 어떻게 쓰는지는
[docs/dotenvx-guide.md](docs/dotenvx-guide.md) 에 정리했다.

`aeg scan` 은 기본으로 `$HOME` 아래를 깊이 6까지 훑는다.
`node_modules`, `.git`, `Library` 등은 건너뛴다. `--depth N` 으로 탐색
깊이를 조정할 수 있다 (기본값 6). 느리면 범위를 좁힌다.
평문 `.env` 는 `aeg init` 으로, `.env.local` 같은 평문 변형은
`dotenvx encrypt -f <파일>` 로 암호화하라고 안내한다. `.env.keys` 는
`.gitignore` 에 없을 때만 보고한다.

## 예외 처리

오탐이 나면 `~/.config/aeg/allow` 에 경로를 한 줄씩 적는다.
그 경로 이하는 판정을 건너뛴다.

## 라이선스

MIT
