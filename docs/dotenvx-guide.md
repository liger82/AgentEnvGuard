# aeg init 과 dotenvx 사용 가이드

`aeg init` 이 실제로 무엇을 하는지, dotenvx 가 키를 어디에 두는지,
마이그레이션한 뒤 로컬에서 코드를 어떻게 실행하는지 정리한다.

## aeg init [경로] 가 하는 일

경로를 생략하면 현재 디렉터리(`.`)를 대상으로 한다. 구현은
`internal/cmd/initcmd.go` 의 `Init` 이다. 순서는 다음과 같다.

1. **사전 확인**
   - `<경로>/.env` 가 없으면 오류로 끝난다 (종료코드 1).
   - `dotenvx --version` 이 실패하면 설치 방법을 안내하고 끝난다.
2. **`.env` 상태에 따라 암호화**
   - 이미 암호화되어 있으면 그대로 둔다.
   - 값이 없으면 암호화를 건너뛴다.
   - 평문이면 그 디렉터리에서 `dotenvx encrypt` 를 실행한다.
3. **`.gitignore` 에 `.env.keys` 추가** — 없을 때만 추가한다. `.gitignore`
   자체가 없으면 만든다.
4. **`.gitignore` 의 `.env` 줄은 안내만 한다** — 암호화된 `.env` 는 커밋해도
   안전하니 빼면 팀과 공유할 수 있다고 알려주지만, 자동으로 고치지 않는다.
5. **git 히스토리 검사** — `git log --all --oneline -- .env` 로 평문 `.env`
   가 과거에 커밋된 적이 있는지 본다. 있으면 해당 커밋을 보여주고, 암호화해도
   히스토리의 평문은 남으니 발급처에서 키를 로테이션하라고 경고한다. git 이
   없거나 저장소가 아니면 수동으로 확인하라고 알린다.
6. **실행 방법 안내** — `dotenvx run -- <실행할 명령>`.

하지 않는 것:

- `.env` 만 다룬다. `.env.local`, `.env.production` 같은 변형은
  `dotenvx encrypt -f <파일>` 로 직접 암호화한다.
- 훅 설치(`aeg install`)와는 별개다. `~/.claude/settings.json` 을 건드리지 않는다.
- 히스토리를 지우거나 키를 로테이션하지 않는다. 경고만 한다.

## dotenvx 암호화 방식

파일을 다른 곳으로 옮기는 게 아니다. **`.env` 안의 값을 그 자리에서
암호문으로 바꾼다.** `dotenvx encrypt` 는 프로젝트마다 공개키·개인키
쌍(secp256k1)을 만들고 다음처럼 나눠 둔다.

| | 저장 위치 | 커밋 |
|---|---|---|
| 공개키 | `.env` 맨 위의 `DOTENV_PUBLIC_KEY="..."` | 가능 |
| 암호화된 값 | `.env` 의 원래 자리 (`encrypted:...`) | 가능 |
| 개인키 | 같은 폴더에 새로 생기는 `.env.keys` 의 `DOTENV_PRIVATE_KEY="..."` | **금지** |

예시 (값은 축약):

```
# 전: .env
OPENAI_API_KEY=sk-abc123

# 후: .env
DOTENV_PUBLIC_KEY="03f8a1..."
OPENAI_API_KEY="encrypted:BG7x9k..."

# 새로 생김: .env.keys
DOTENV_PRIVATE_KEY="a4c2e9..."
```

`.env.production` 을 암호화하면 `DOTENV_PUBLIC_KEY_PRODUCTION` /
`DOTENV_PRIVATE_KEY_PRODUCTION` 처럼 접미사가 붙는다.

- **암호화에는 공개키만 필요하다.** 개인키가 없는 팀원도 `dotenvx set` 으로
  값을 추가할 수 있다.
- **복호화에는 개인키가 필요하다.** `dotenvx run` 은 `.env.keys` 또는
  환경변수 `DOTENV_PRIVATE_KEY` 에서 개인키를 찾는다. CI·서버에서는 보통
  환경변수로 넣는다.
- 복호화한 값은 자식 프로세스의 환경변수로만 들어간다. 디스크에 평문을 다시
  쓰지 않는다.

지켜야 할 것은 `.env.keys` 하나다. 그래서 `aeg init` 은 이를 `.gitignore`
에 넣고, 훅은 에이전트가 `.env.keys` 를 읽거나 `DOTENV_PRIVATE_KEY` 를
출력하지 못하게 막는다. 암호화된 `.env` 는 읽혀도 값이 드러나지 않으므로
허용한다. `.env.keys` 는 비밀번호 관리자 같은 별도 경로로 공유한다.

## 로컬에서 코드 실행하기

### 코드는 환경변수를 읽기만 한다

```python
import os
api_key = os.environ["OPENAI_API_KEY"]
```

```js
const apiKey = process.env.OPENAI_API_KEY
```

### 실행할 때 dotenvx run 을 앞에 붙인다

```bash
dotenvx run -- python main.py
dotenvx run -- node app.js
dotenvx run -- npm run dev
dotenvx run -f .env.production -- node app.js   # 다른 파일
```

자주 쓰면 스크립트에 넣는다.

```json
"scripts": {
  "dev": "dotenvx run -- next dev"
}
```

### .env 를 직접 읽는 코드를 점검한다

`python-dotenv` 의 `load_dotenv()`, Node `dotenv` 의 `config()` 를 쓰고
있다면 주의한다.

- 기본으로 이미 있는 환경변수를 덮어쓰지 않으므로, `dotenvx run` 으로
  실행하면 대개 문제없다.
- `dotenvx run` 없이 실행하면 `"encrypted:..."` 문자열이 값으로 들어간다.
  에러 없이 인증만 실패해서 원인을 찾기 어렵다.
- `override=True` / `{ override: true }` 를 쓰면 `dotenvx run` 으로 실행해도
  암호문이 평문을 덮어쓴다.

가장 깔끔한 방법은 이런 로딩 코드를 지우는 것이다. Node 라면 `dotenv` 대신
`@dotenvx/dotenvx` 를 써서 코드 안에서 복호화할 수도 있다. 이 경우
`dotenvx run` 없이 `node app.js` 로 실행해도 된다.

```js
require('@dotenvx/dotenvx').config()
```

### IDE 디버거

VS Code·PyCharm 의 실행 버튼은 `dotenvx run` 을 거치지 않는다. 실행 설정의
명령을 `dotenvx run -- ...` 로 감싸거나, 위의 `@dotenvx/dotenvx` 방식을 쓴다.

### Claude Code 가 실행할 때

- `dotenvx run -- python main.py` 같은 일반 실행은 막지 않는다.
- `dotenvx run -- printenv`, `dotenvx run -- env` 처럼 값을 통째로 출력하는
  명령과 `dotenvx get` / `decrypt` / `keypair` 는 막는다.
- 코드 자체가 `print(os.environ["OPENAI_API_KEY"])` 처럼 값을 출력하는 것은
  막지 못한다. 그런 디버그 출력은 남기지 않는다.
