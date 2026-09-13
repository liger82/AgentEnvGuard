# AgentEnvGuard

코딩 에이전트가 시크릿 평문을 읽지 못하게 막는 Claude Code 가드레일.

**시크릿 매니저가 아니다.** 볼트·암호화·주입·출력 마스킹은
[dotenvx](https://dotenvx.com) 가 한다. AgentEnvGuard 는 그 위에 얹혀서,
에이전트가 규칙을 지키도록 강제하는 얇은 층이다.
(같은 이름의 [amannirala13/envguard](https://github.com/amannirala13/envguard)
와는 무관한 별개 프로젝트다.)

## 무엇을 막는가

에이전트가 `cat .env` 한 번을 하면 시크릿이 대화 로그와 컨텍스트에 남고,
그 뒤로 어디로 흘러가는지 통제할 수 없다. AgentEnvGuard 는 Claude Code 의
PreToolUse 훅으로 `Bash`, `Read`, `Grep` 을 검사해 다음을 막는다.

- `.env.keys` 읽기 (dotenvx 개인키)
- **평문** `.env` 읽기 — 암호화된 `.env` 는 값이 암호문이라 그대로 읽게 둔다
- `dotenvx get`, `dotenvx decrypt`
- `--no-redact`, `--mask 0` (출력 마스킹 우회)
- `DOTENV_PRIVATE_KEY` 출력

`Grep` 을 포함하는 것이 중요하다. Grep 은 매칭된 줄 내용을 반환하므로
`Grep(pattern="DOTENV_PRIVATE_KEY", path=".env.keys")` 한 번이면
Bash 도 Read 도 거치지 않고 개인키가 나온다.

차단할 때는 거부만 하지 않고 올바른 대안을 함께 알려준다. 그러지 않으면
에이전트가 우회를 시도한다.

## 무엇을 막지 못하는가

**의도적 탈취는 막지 못한다.** 명령 문자열 매칭이므로 이런 것은 통과한다.

```bash
d=dotenvx; $d get MY_KEY
```

bash 를 실행할 수 있는 에이전트에게서 값을 완전히 숨기는 것은 불가능하다.
이 도구의 목표는 **우발적 노출 차단**이다. 그 이상을 기대하면 안 된다.

그 밖에 알려진 한계:

- 경로 없는 일반 재귀 검색(예: `Grep(pattern="API_KEY")`, path 없음)은
  **평문** `.env` 안의 매칭된 줄을 그대로 드러낼 수 있다. 이걸 막으려고
  경로 없는 광범위한 검색 자체를 차단하면 에이전트의 주된 검색 기능이
  망가진다. 해법은 검색 차단이 아니라 `aeg scan` / `aeg init` 으로 평문
  `.env` 자체를 없애는 것이다.
- 정규식으로 흐린 패턴(예: `DOTENV_PRIVATE_KE[Y]`)은 패턴 검사를
  피해간다. 의도적 탈취와 같은 부류의 우회다.
- 암호화는 git 히스토리에 이미 커밋된 평문까지 지우지 않는다. `aeg init`
  은 히스토리에서 평문 `.env` 커밋을 발견하면 경고하지만, 실제로는 해당
  키를 발급처에서 로테이션해야 한다.
- `aeg install` 을 다시 실행해도 이미 등록된 훅의 matcher 나 args 를
  누가 손으로 고쳐놓은 것까지 복구하지 않는다. 의심되면
  `~/.claude/settings.json` 을 직접 확인하라.

## 설치

```bash
brew install dotenvx/brew/dotenvx   # 볼트는 dotenvx 가 맡는다
go install github.com/liger82/AgentEnvGuard/cmd/aeg@latest
aeg install                         # 노트북당 한 번
```

`aeg install` 은 `~/.claude/settings.json` 을 읽어 PreToolUse 훅을
병합하고, 덮어쓰기 전에 `~/.claude/settings.json.aeg-backup` 을 남긴다.
재직렬화 과정에서 기존 키 순서가 알파벳순으로 바뀌니 diff 를 볼 때
당황하지 말 것. 문제가 생기면 되돌린다.

```bash
cp ~/.claude/settings.json.aeg-backup ~/.claude/settings.json
```

## 사용

```bash
aeg scan                      # 평문 .env 를 쓰는 프로젝트를 전부 찾는다
aeg init ~/projects/myapp     # 하나를 dotenvx 로 마이그레이션한다
dotenvx run -- python app.py  # 이제부터 스크립트는 이렇게 실행한다
```

`aeg scan` 은 기본으로 `$HOME` 아래를 깊이 6까지 훑는다.
`node_modules`, `.git`, `Library` 등은 건너뛴다. `--depth N` 으로 탐색
깊이를 조정할 수 있다 (기본값 6). 느리면 범위를 좁힌다.

## 예외 처리

오탐이 나면 `~/.config/aeg/allow` 에 경로를 한 줄씩 적는다.
그 경로 이하는 판정을 건너뛴다.

## 라이선스

MIT
