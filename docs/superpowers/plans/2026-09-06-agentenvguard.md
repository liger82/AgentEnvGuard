# AgentEnvGuard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 코딩 에이전트가 시크릿 평문을 우발적으로 읽는 것을 막는 Claude Code PreToolUse 훅과, 평문 `.env` 를 찾아 dotenvx로 마이그레이션하는 CLI 를 단일 Go 바이너리 `aeg` 로 만든다.

**Architecture:** 판정 엔진(`internal/policy`)은 에이전트 도구와 무관한 순수 함수다. Claude Code 의 JSON 입출력 규약은 어댑터(`internal/adapter`)에만 가둔다. 어댑터가 policy 를 import 하고 그 반대는 없다. 시크릿 저장·암호화·주입·마스킹은 전부 dotenvx 서브프로세스에 위임하며 이 프로젝트는 구현하지 않는다.

**Tech Stack:** Go (표준 라이브러리만. 외부 의존 0), dotenvx (서브프로세스 호출, BSD-3-Clause)

**Spec:** `docs/superpowers/specs/2026-09-06-agentenvguard-design.md`

## Global Constraints

- 바이너리 이름은 `aeg`. 프로젝트 이름은 AgentEnvGuard.
- **외부 Go 의존성 0.** 표준 라이브러리만 쓴다. 훅이 매 툴 호출마다 실행되므로 기동 시간이 곧 비용이다.
- **성능 예산: `aeg hook` 1회 실행 10ms 이내.**
- **`aeg hook` 은 항상 exit 0 으로 끝난다.** 판정은 stdout JSON 본문에 담는다. 종료코드를 판정 신호로 쓰지 않는다.
- **Fail-open.** 파싱 실패·내부 오류·패닉은 전부 `permissionDecision: "allow"` 로 떨어진다. 단, `.env.keys` 경로 매칭처럼 판정이 명확한 경우는 예외로 차단한다.
- `.env` 판별은 파일 선두 **64KB** 만 읽는다.
- 거부 메시지는 반드시 **올바른 대안을 포함**한다. 거부만 하면 에이전트가 우회를 시도한다.
- 차단 대상 도구는 `Bash`, `Read`, `Grep` 세 개. Grep 을 빠뜨리면 정책 전체가 무의미해진다.
- 평문 `.env` 백업 파일을 만들지 않는다.
- 사용자 메시지는 스펙을 따라 한국어로 둔다. 공개 배포 전 영어화는 Task 11 에서 별도 항목으로 다룬다.

## File Structure

```text
agentenvguard/
├── go.mod                              module github.com/stuartkim/agentenvguard
├── .gitignore
├── main.go                             서브커맨드 디스패치만. 로직 없음
├── internal/
│   ├── policy/
│   │   ├── types.go                    ToolKind, ToolCall, Decision
│   │   ├── envfile.go                  .env 암호화 판별기
│   │   ├── envfile_test.go
│   │   ├── path.go                     경로 분류기
│   │   ├── path_test.go
│   │   ├── command.go                  Bash 명령 분류기
│   │   ├── command_test.go
│   │   ├── engine.go                   Decide() — 위 셋을 조립
│   │   └── engine_test.go
│   ├── allowlist/
│   │   ├── allowlist.go                ~/.config/aeg/allow
│   │   └── allowlist_test.go
│   ├── adapter/
│   │   ├── adapter.go                  Adapter 인터페이스
│   │   ├── claudecode.go               PreToolUse Parse/Emit
│   │   └── claudecode_test.go
│   └── cmd/
│       ├── hook.go                     aeg hook
│       ├── hook_test.go
│       ├── install.go                  aeg install
│       ├── install_test.go
│       ├── scan.go                     aeg scan
│       ├── scan_test.go
│       ├── initcmd.go                  aeg init
│       └── initcmd_test.go
├── README.md
└── LICENSE
```

**의존 방향:** `cmd` → `adapter` → `policy`, 그리고 `cmd` → `policy`, `policy` → `allowlist`. 역방향 import 는 없다.

**`initcmd.go` 로 이름 붙인 이유:** Go 에서 `init` 은 특별한 함수 이름이라 혼동을 피한다. 파일명만 다르고 서브커맨드 이름은 `aeg init` 이다.

---

### Task 1: 프로젝트 초기화 + `.env` 암호화 판별기

전체 정책이 "이 `.env` 가 암호문인가"에 걸려 있다. 가장 안쪽 순수 함수부터 만든다.

**Files:**
- Create: `go.mod`, `.gitignore`
- Create: `internal/policy/envfile.go`
- Test: `internal/policy/envfile_test.go`

**Interfaces:**
- Consumes: (없음)
- Produces:
  - `type EnvFileKind int` — 상수 `EnvEncrypted`, `EnvPlaintext`, `EnvEmpty`
  - `func ClassifyEnvContent(r io.Reader) EnvFileKind`
  - `func ClassifyEnvFile(path string) EnvFileKind` — 읽기 실패 시 `EnvEmpty` 반환(fail-open)

- [ ] **Step 1: Go 설치 확인**

Go 가 로컬에 없다. 설치한다.

```bash
command -v go || brew install go
go version
```

Expected: `go version go1.2x.x darwin/arm64` 형태 출력

- [ ] **Step 2: 모듈과 .gitignore 생성**

```bash
cd ~/projects/agentenvguard
go mod init github.com/stuartkim/agentenvguard
cat > .gitignore <<'EOF'
# 빌드 산출물
/aeg
/dist/

# 에디터
*.swp
*.swo
.DS_Store

# 이 저장소는 시크릿을 담지 않는다. 실수 방지용.
.env
.env.keys
.env.local
EOF
```

- [ ] **Step 3: 실패하는 테스트 작성**

`internal/policy/envfile_test.go`:

```go
package policy

import (
	"strings"
	"testing"
)

func TestClassifyEnvContent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want EnvFileKind
	}{
		{
			name: "dotenvx 암호문",
			in: `DOTENV_PUBLIC_KEY="034af93e93708b994c10f236c96ef88e47291066946cce2e8d98c9e02c741ced45"
DART_API_KEY="encrypted:BDqDBibm4wsYqMpCjTQ6BsDHmMadg9K3dAt"
OPENAI_KEY="encrypted:AnotherCiphertextHere"`,
			want: EnvEncrypted,
		},
		{
			name: "평문",
			in: `DART_API_KEY=abc123
OPENAI_KEY=sk-realsecret`,
			want: EnvPlaintext,
		},
		{
			name: "공개키는 있으나 값 하나가 평문 (혼합)",
			in: `DOTENV_PUBLIC_KEY="034af9"
A="encrypted:xxx"
B=plainvalue`,
			want: EnvPlaintext,
		},
		{
			name: "공개키 없이 encrypted 접두어만",
			in:   `A="encrypted:xxx"`,
			want: EnvPlaintext,
		},
		{
			name: "빈 파일",
			in:   ``,
			want: EnvEmpty,
		},
		{
			name: "주석과 빈 줄뿐",
			in: `# 여기에 키를 넣으세요

# TODO
`,
			want: EnvEmpty,
		},
		{
			name: "공개키 행만 있고 데이터 없음",
			in:   `DOTENV_PUBLIC_KEY="034af9"`,
			want: EnvEmpty,
		},
		{
			name: "export 접두어",
			in: `export DOTENV_PUBLIC_KEY="034af9"
export SECRET="encrypted:xxx"`,
			want: EnvEncrypted,
		},
		{
			name: "따옴표 없는 암호문",
			in: `DOTENV_PUBLIC_KEY=034af9
A=encrypted:xxx`,
			want: EnvEncrypted,
		},
		{
			name: "등호 없는 쓰레기 줄은 무시",
			in: `이건 그냥 문장
DOTENV_PUBLIC_KEY="034af9"
A="encrypted:xxx"`,
			want: EnvEncrypted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyEnvContent(strings.NewReader(tt.in))
			if got != tt.want {
				t.Errorf("ClassifyEnvContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyEnvContentStopsAt64KB(t *testing.T) {
	// 64KB 뒤에 평문이 있어도 읽지 않으므로 앞부분만으로 판정한다.
	head := "DOTENV_PUBLIC_KEY=\"034af9\"\nA=\"encrypted:xxx\"\n"
	padding := strings.Repeat("# 주석으로 채운다\n", 8000) // 넉넉히 64KB 초과
	in := head + padding + "LEAKED=plaintextsecret\n"
	if got := ClassifyEnvContent(strings.NewReader(in)); got != EnvEncrypted {
		t.Errorf("64KB 이후를 읽었다: got %v, want EnvEncrypted", got)
	}
}

func TestClassifyEnvFileMissingIsEmpty(t *testing.T) {
	// fail-open: 읽을 수 없으면 차단하지 않는다.
	if got := ClassifyEnvFile("/nonexistent/path/.env"); got != EnvEmpty {
		t.Errorf("got %v, want EnvEmpty", got)
	}
}
```

- [ ] **Step 4: 테스트가 실패하는지 확인**

Run: `go test ./internal/policy/ -run TestClassifyEnv -v`
Expected: 컴파일 실패 — `undefined: ClassifyEnvContent`, `undefined: EnvFileKind`

- [ ] **Step 5: 최소 구현 작성**

`internal/policy/envfile.go`:

```go
package policy

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// maxEnvScanBytes 는 .env 판별 시 읽는 최대 바이트다.
// 파일 전체를 읽지 않는 이유는 훅이 매 툴 호출마다 실행되기 때문이다.
const maxEnvScanBytes = 64 * 1024

type EnvFileKind int

const (
	// EnvEmpty 는 KEY=VALUE 행이 하나도 없는 경우다. 읽어도 새는 것이 없다.
	EnvEmpty EnvFileKind = iota
	// EnvEncrypted 는 dotenvx 로 암호화된 파일이다. 값이 암호문이라 읽어도 안전하다.
	EnvEncrypted
	// EnvPlaintext 는 평문 시크릿을 담고 있을 수 있는 파일이다.
	EnvPlaintext
)

func (k EnvFileKind) String() string {
	switch k {
	case EnvEmpty:
		return "EnvEmpty"
	case EnvEncrypted:
		return "EnvEncrypted"
	case EnvPlaintext:
		return "EnvPlaintext"
	}
	return "EnvFileKind(?)"
}

// ClassifyEnvContent 는 .env 내용을 읽어 암호문인지 평문인지 판정한다.
//
// 판정 규칙: 주석(#)과 빈 줄을 버리고 남은 KEY=VALUE 행을 본다.
// DOTENV_PUBLIC_KEY 행이 존재하고 그 행을 제외한 모든 값이 "encrypted:" 로
// 시작하면 암호문이다. 데이터 행이 하나도 없으면 EnvEmpty 다.
func ClassifyEnvContent(r io.Reader) EnvFileKind {
	sc := bufio.NewScanner(io.LimitReader(r, maxEnvScanBytes))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024)

	sawPublicKey := false
	dataLines := 0
	allEncrypted := true

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		eq := strings.Index(line, "=")
		if eq < 1 {
			continue // KEY=VALUE 형태가 아니면 무시
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)

		if key == "DOTENV_PUBLIC_KEY" {
			sawPublicKey = true
			continue
		}
		dataLines++
		if !strings.HasPrefix(value, "encrypted:") {
			allEncrypted = false
		}
	}

	if dataLines == 0 {
		return EnvEmpty
	}
	if sawPublicKey && allEncrypted {
		return EnvEncrypted
	}
	return EnvPlaintext
}

// ClassifyEnvFile 은 경로를 열어 판정한다.
// 열 수 없으면 EnvEmpty 를 반환한다 — fail-open 이므로 차단하지 않는다.
func ClassifyEnvFile(path string) EnvFileKind {
	f, err := os.Open(path)
	if err != nil {
		return EnvEmpty
	}
	defer f.Close()
	return ClassifyEnvContent(f)
}
```

- [ ] **Step 6: 테스트 통과 확인**

Run: `go test ./internal/policy/ -v`
Expected: 모든 테스트 PASS

- [ ] **Step 7: 커밋**

```bash
git add go.mod .gitignore internal/policy/envfile.go internal/policy/envfile_test.go
git commit -m "feat: .env 암호화 판별기

주석·빈 줄을 버리고 KEY=VALUE 행만 본다. DOTENV_PUBLIC_KEY 가 있고
나머지 값이 모두 encrypted: 로 시작하면 암호문. 선두 64KB만 읽는다."
```

---

### Task 2: 경로 분류기

`.env.keys` 는 무조건 차단, `.env.example` 류는 무조건 허용, `.env` 계열은 내용을 봐야 한다. 이 분기를 파일명만으로 결정한다.

**Files:**
- Create: `internal/policy/path.go`
- Test: `internal/policy/path_test.go`

**Interfaces:**
- Consumes: (없음)
- Produces:
  - `type PathKind int` — 상수 `PathOther`, `PathEnvKeys`, `PathEnvFile`, `PathEnvExample`
  - `func ClassifyPath(p string) PathKind`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/policy/path_test.go`:

```go
package policy

import "testing"

func TestClassifyPath(t *testing.T) {
	tests := []struct {
		in   string
		want PathKind
	}{
		{".env.keys", PathEnvKeys},
		{"/Users/me/proj/.env.keys", PathEnvKeys},
		{".env.keys.bak", PathEnvKeys},
		{"./.env.keys", PathEnvKeys},

		{".env", PathEnvFile},
		{"/Users/me/proj/.env", PathEnvFile},
		{".env.local", PathEnvFile},
		{".env.production", PathEnvFile},

		{".env.example", PathEnvExample},
		{".env.template", PathEnvExample},
		{".env.sample", PathEnvExample},
		{"/Users/me/proj/.env.example", PathEnvExample},

		{"main.go", PathOther},
		{"README.md", PathOther},
		{"envelope.txt", PathOther},
		{"my.env.txt", PathOther},
		{"", PathOther},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ClassifyPath(tt.in); got != tt.want {
				t.Errorf("ClassifyPath(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/policy/ -run TestClassifyPath -v`
Expected: 컴파일 실패 — `undefined: ClassifyPath`

- [ ] **Step 3: 최소 구현 작성**

`internal/policy/path.go`:

```go
package policy

import (
	"path/filepath"
	"strings"
)

type PathKind int

const (
	// PathOther 는 시크릿과 무관한 경로다.
	PathOther PathKind = iota
	// PathEnvKeys 는 dotenvx 개인키 파일이다. 내용과 무관하게 항상 차단한다.
	PathEnvKeys
	// PathEnvFile 은 .env 계열이다. 내용을 봐서 암호문이면 허용한다.
	PathEnvFile
	// PathEnvExample 은 설계상 공개되는 예시 파일이다. 항상 허용한다.
	PathEnvExample
)

func (k PathKind) String() string {
	switch k {
	case PathOther:
		return "PathOther"
	case PathEnvKeys:
		return "PathEnvKeys"
	case PathEnvFile:
		return "PathEnvFile"
	case PathEnvExample:
		return "PathEnvExample"
	}
	return "PathKind(?)"
}

var exampleSuffixes = []string{".example", ".template", ".sample"}

// ClassifyPath 는 파일명만 보고 종류를 정한다. 파일을 열지 않는다.
func ClassifyPath(p string) PathKind {
	if p == "" {
		return PathOther
	}
	base := filepath.Base(filepath.Clean(p))

	// .env.keys 를 가장 먼저 본다. .env.keys.bak 같은 파생도 개인키를 담는다.
	if base == ".env.keys" || strings.HasPrefix(base, ".env.keys") {
		return PathEnvKeys
	}
	if base != ".env" && !strings.HasPrefix(base, ".env.") {
		return PathOther
	}
	for _, suf := range exampleSuffixes {
		if strings.HasSuffix(base, suf) {
			return PathEnvExample
		}
	}
	return PathEnvFile
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/policy/ -v`
Expected: 모든 테스트 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/policy/path.go internal/policy/path_test.go
git commit -m "feat: 경로 분류기

.env.keys 는 파생 포함 항상 차단, .env.example 류는 항상 허용,
.env 계열은 내용 판별로 넘긴다."
```

---

### Task 3: Bash 명령 분류기

Bash 는 경로 하나가 아니라 임의 문자열이다. 위험 명령 패턴을 찾고, 파일을 읽는 명령이면 대상 경로를 뽑아낸다.

**Files:**
- Create: `internal/policy/command.go`
- Test: `internal/policy/command_test.go`

**Interfaces:**
- Consumes: `ClassifyPath` (Task 2)
- Produces:
  - `type CmdRisk int` — 상수 `CmdSafe`, `CmdDotenvxGet`, `CmdDotenvxDecrypt`, `CmdRedactBypass`, `CmdPrivateKeyEcho`
  - `type CmdFinding struct { Risk CmdRisk; Paths []string }`
  - `func AnalyzeCommand(cmd string) CmdFinding`

`Paths` 는 이 명령이 읽으려는 `.env` 계열 경로다. `Risk` 가 `CmdSafe` 여도 `Paths` 가 비어 있지 않을 수 있고, 그때는 엔진이 내용을 보고 판정한다.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/policy/command_test.go`:

```go
package policy

import (
	"reflect"
	"testing"
)

func TestAnalyzeCommandRisk(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want CmdRisk
	}{
		{"dotenvx get", "dotenvx get DART_API_KEY", CmdDotenvxGet},
		{"dotenvx get 공백 많음", "dotenvx   get   KEY", CmdDotenvxGet},
		{"dotenvx decrypt", "dotenvx decrypt", CmdDotenvxDecrypt},
		{"dotenvx decrypt stdout", "dotenvx decrypt --stdout", CmdDotenvxDecrypt},
		{"no-redact", "dotenvx run --no-redact -- python x.py", CmdRedactBypass},
		{"mask 0", "dotenvx run --mask 0 -- node a.js", CmdRedactBypass},
		{"private key echo", "echo $DOTENV_PRIVATE_KEY", CmdPrivateKeyEcho},
		{"private key printenv", "printenv DOTENV_PRIVATE_KEY", CmdPrivateKeyEcho},

		{"정상 run", "dotenvx run -- python 06_Scripts/fetch.py", CmdSafe},
		{"정상 set", "dotenvx set DART_API_KEY abc", CmdSafe},
		{"무관한 명령", "ls -la", CmdSafe},
		{"get 이 다른 단어의 일부", "dotenvx run -- ./getdata.sh", CmdSafe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AnalyzeCommand(tt.cmd).Risk; got != tt.want {
				t.Errorf("AnalyzeCommand(%q).Risk = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestAnalyzeCommandPaths(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string
	}{
		{"cat .env", "cat .env", []string{".env"}},
		{"grep 로 읽기", "grep API .env", []string{".env"}},
		{"경로 지정", "cat /Users/me/p/.env.keys", []string{"/Users/me/p/.env.keys"}},
		{"파이프 뒤쪽도 본다", "ls | cat .env", []string{".env"}},
		{"세미콜론 분리", "cd /tmp; head .env.local", []string{".env.local"}},
		{"리다이렉션", "base64 < .env.keys", []string{".env.keys"}},
		{"cp 로 빼돌리기", "cp .env.keys /tmp/x", []string{".env.keys"}},

		{"읽기 명령이 아니면 무시", "rm .env", nil},
		{"echo 는 무시", "echo .env 를 확인하세요", nil},
		{"dotenvx run 은 무시", "dotenvx run -- python x.py", nil},
		{"무관한 파일", "cat README.md", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeCommand(tt.cmd).Paths
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AnalyzeCommand(%q).Paths = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/policy/ -run TestAnalyzeCommand -v`
Expected: 컴파일 실패 — `undefined: AnalyzeCommand`

- [ ] **Step 3: 최소 구현 작성**

`internal/policy/command.go`:

```go
package policy

import (
	"path/filepath"
	"strings"
)

type CmdRisk int

const (
	CmdSafe CmdRisk = iota
	CmdDotenvxGet
	CmdDotenvxDecrypt
	CmdRedactBypass
	CmdPrivateKeyEcho
)

func (r CmdRisk) String() string {
	switch r {
	case CmdSafe:
		return "CmdSafe"
	case CmdDotenvxGet:
		return "CmdDotenvxGet"
	case CmdDotenvxDecrypt:
		return "CmdDotenvxDecrypt"
	case CmdRedactBypass:
		return "CmdRedactBypass"
	case CmdPrivateKeyEcho:
		return "CmdPrivateKeyEcho"
	}
	return "CmdRisk(?)"
}

type CmdFinding struct {
	Risk  CmdRisk
	Paths []string // 이 명령이 읽으려는 .env 계열 경로
}

// readerCommands 는 파일 내용을 표준출력으로 흘리거나 복사하는 명령이다.
// rm, touch, ls 처럼 내용을 드러내지 않는 명령은 넣지 않는다.
var readerCommands = map[string]bool{
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"bat": true, "grep": true, "egrep": true, "fgrep": true, "rg": true,
	"ag": true, "ack": true, "sed": true, "awk": true, "cut": true,
	"tr": true, "sort": true, "uniq": true, "nl": true,
	"cp": true, "mv": true, "install": true,
	"base64": true, "xxd": true, "od": true, "strings": true, "dd": true,
	"source": true, ".": true, "open": true, "pbcopy": true,
}

// segmentSeparators 는 셸에서 새 명령이 시작되는 지점이다.
var segmentSeparators = []string{"&&", "||", ";", "|", "\n"}

func splitSegments(cmd string) []string {
	segs := []string{cmd}
	for _, sep := range segmentSeparators {
		var next []string
		for _, s := range segs {
			next = append(next, strings.Split(s, sep)...)
		}
		segs = next
	}
	return segs
}

// AnalyzeCommand 는 Bash 명령 문자열을 훑어 위험 패턴과 읽으려는 경로를 찾는다.
//
// 문자열 매칭이므로 d=dotenvx; $d get X 같은 우회는 잡지 못한다.
// 이 도구의 목표는 우발적 노출 차단이며 의도적 탈취 차단이 아니다.
func AnalyzeCommand(cmd string) CmdFinding {
	f := CmdFinding{Risk: CmdSafe}

	for _, seg := range splitSegments(cmd) {
		fields := strings.Fields(seg)
		if len(fields) == 0 {
			continue
		}

		// 위험 플래그는 세그먼트 어디에 있어도 잡는다.
		for i, tok := range fields {
			if tok == "--no-redact" {
				f.Risk = CmdRedactBypass
			}
			if tok == "--mask" && i+1 < len(fields) && fields[i+1] == "0" {
				f.Risk = CmdRedactBypass
			}
			if strings.Contains(tok, "DOTENV_PRIVATE_KEY") {
				f.Risk = CmdPrivateKeyEcho
			}
		}

		base := filepath.Base(fields[0])
		if base == "dotenvx" && len(fields) > 1 {
			switch fields[1] {
			case "get":
				f.Risk = CmdDotenvxGet
			case "decrypt":
				f.Risk = CmdDotenvxDecrypt
			}
		}

		// 리다이렉션(<)은 어떤 명령이든 파일 내용을 끌어온다.
		redirected := false
		for i, tok := range fields {
			if tok == "<" && i+1 < len(fields) {
				if k := ClassifyPath(fields[i+1]); k == PathEnvKeys || k == PathEnvFile {
					f.Paths = append(f.Paths, fields[i+1])
					redirected = true
				}
			}
		}

		if !readerCommands[base] || redirected {
			continue
		}
		for _, tok := range fields[1:] {
			if strings.HasPrefix(tok, "-") {
				continue
			}
			if k := ClassifyPath(tok); k == PathEnvKeys || k == PathEnvFile {
				f.Paths = append(f.Paths, tok)
			}
		}
	}
	return f
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/policy/ -v`
Expected: 모든 테스트 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/policy/command.go internal/policy/command_test.go
git commit -m "feat: Bash 명령 분류기

세그먼트로 쪼개 위험 플래그와 읽기 명령의 대상 경로를 찾는다.
readerCommands 에 없는 rm/touch/echo 는 내용을 드러내지 않으므로 제외."
```

---

### Task 4: 사용자 예외 목록

`~/.config/aeg/allow` 에 적힌 경로 이하는 판정을 건너뛴다. 전역 설치이므로 오탐이 곧 마찰이고, 사용자가 빠져나갈 문을 반드시 줘야 한다.

**Files:**
- Create: `internal/allowlist/allowlist.go`
- Test: `internal/allowlist/allowlist_test.go`

**Interfaces:**
- Consumes: (없음)
- Produces:
  - `func Path() string` — `~/.config/aeg/allow` 의 절대 경로
  - `func LoadFrom(path string) []string` — 읽기 실패 시 빈 슬라이스
  - `func Load() []string` — `LoadFrom(Path())`
  - `func Covers(roots []string, target string) bool`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/allowlist/allowlist_test.go`:

```go
package allowlist

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadFrom(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "allow")
	content := "# 주석은 무시\n" +
		"/Users/me/scratch\n" +
		"\n" +
		"   /Users/me/legacy   \n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadFrom(p)
	want := []string{"/Users/me/scratch", "/Users/me/legacy"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadFrom() = %v, want %v", got, want)
	}
}

func TestLoadFromMissingIsEmpty(t *testing.T) {
	if got := LoadFrom("/nonexistent/allow"); len(got) != 0 {
		t.Errorf("LoadFrom() = %v, want empty", got)
	}
}

func TestCovers(t *testing.T) {
	roots := []string{"/Users/me/scratch", "/Users/me/legacy"}
	tests := []struct {
		target string
		want   bool
	}{
		{"/Users/me/scratch/.env", true},
		{"/Users/me/scratch/deep/nested/.env", true},
		{"/Users/me/scratch", true},
		{"/Users/me/legacy/app/.env.keys", true},
		{"/Users/me/other/.env", false},
		{"/Users/me/scratchpad/.env", false}, // 접두어만 같은 형제 디렉터리
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := Covers(roots, tt.target); got != tt.want {
				t.Errorf("Covers(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/allowlist/ -v`
Expected: 컴파일 실패 — `undefined: LoadFrom`

- [ ] **Step 3: 최소 구현 작성**

`internal/allowlist/allowlist.go`:

```go
// Package allowlist 는 사용자가 판정에서 제외한 경로 목록을 다룬다.
package allowlist

import (
	"os"
	"path/filepath"
	"strings"
)

// Path 는 예외 목록 파일의 위치다.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "aeg", "allow")
}

// LoadFrom 은 한 줄에 하나씩 적힌 경로를 읽는다.
// 주석(#)과 빈 줄은 버린다. 읽을 수 없으면 빈 슬라이스를 반환한다.
func LoadFrom(path string) []string {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var roots []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		roots = append(roots, filepath.Clean(line))
	}
	return roots
}

// Load 는 기본 위치에서 읽는다.
func Load() []string { return LoadFrom(Path()) }

// Covers 는 target 이 roots 중 하나의 하위 경로인지 본다.
// 문자열 접두어 비교로는 /a/scratch 가 /a/scratchpad 를 삼키므로
// 경로 구분자를 붙여 비교한다.
func Covers(roots []string, target string) bool {
	if target == "" {
		return false
	}
	target = filepath.Clean(target)
	for _, r := range roots {
		if target == r {
			return true
		}
		if strings.HasPrefix(target, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/allowlist/ -v`
Expected: 모든 테스트 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/allowlist/
git commit -m "feat: 사용자 예외 목록

~/.config/aeg/allow 이하 경로는 판정을 건너뛴다. 경로 비교에
구분자를 붙여 /a/scratch 가 /a/scratchpad 를 삼키지 않게 했다."
```

---

### Task 5: 판정 엔진

Task 1~4 를 조립해 `Decide(ToolCall) Decision` 하나로 만든다. 여기가 이 프로젝트에서 가장 중요한 코드이고 테스트가 가장 촘촘해야 한다.

**Files:**
- Create: `internal/policy/types.go`
- Create: `internal/policy/engine.go`
- Test: `internal/policy/engine_test.go`

**Interfaces:**
- Consumes: `ClassifyEnvFile`(T1), `ClassifyPath`(T2), `AnalyzeCommand`(T3), `allowlist.Load`/`allowlist.Covers`(T4)
- Produces:
  - `type ToolKind int` — 상수 `ToolUnknown`, `ToolBash`, `ToolFileRead`, `ToolContentSearch`
  - `type ToolCall struct { Kind ToolKind; Command, Path, Pattern, Cwd string }`
  - `type Decision struct { Allow bool; Reason string }`
  - `type Engine struct { AllowRoots []string; ClassifyFile func(string) EnvFileKind }`
  - `func New() *Engine`
  - `func (e *Engine) Decide(tc ToolCall) Decision`

- [ ] **Step 1: 타입 정의 작성**

`internal/policy/types.go`:

```go
package policy

type ToolKind int

const (
	ToolUnknown ToolKind = iota
	ToolBash
	ToolFileRead      // Read
	ToolContentSearch // Grep — 매칭된 줄 내용을 반환하므로 반드시 막아야 한다
)

// ToolCall 은 에이전트 도구 호출을 도구 중립적으로 표현한 것이다.
type ToolCall struct {
	Kind    ToolKind
	Command string // ToolBash
	Path    string // ToolFileRead, ToolContentSearch
	Pattern string // ToolContentSearch
	Cwd     string // 상대 경로 해석 기준
}

// Decision 은 판정 결과다. 차단할 때 Reason 은 반드시 올바른 대안을 담는다.
type Decision struct {
	Allow  bool
	Reason string
}
```

- [ ] **Step 2: 실패하는 테스트 작성**

`internal/policy/engine_test.go`:

```go
package policy

import (
	"strings"
	"testing"
)

// 파일 내용 판별을 주입해 실제 파일 없이 테스트한다.
func engineWith(kinds map[string]EnvFileKind) *Engine {
	return &Engine{
		ClassifyFile: func(p string) EnvFileKind {
			if k, ok := kinds[p]; ok {
				return k
			}
			return EnvEmpty
		},
	}
}

func TestDecideFileRead(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{
		"/p/.env":     EnvPlaintext,
		"/p/enc/.env": EnvEncrypted,
	})
	tests := []struct {
		name      string
		path      string
		wantAllow bool
	}{
		{"개인키 파일은 차단", "/p/.env.keys", false},
		{"평문 .env 차단", "/p/.env", false},
		{"암호문 .env 허용", "/p/enc/.env", true},
		{"example 허용", "/p/.env.example", true},
		{"무관 파일 허용", "/p/main.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{Kind: ToolFileRead, Path: tt.path, Cwd: "/p"})
			if got.Allow != tt.wantAllow {
				t.Errorf("Allow = %v, want %v (reason: %s)", got.Allow, tt.wantAllow, got.Reason)
			}
			if !got.Allow && got.Reason == "" {
				t.Error("차단인데 Reason 이 비었다")
			}
		})
	}
}

func TestDecideGrepIsBlocked(t *testing.T) {
	// Grep 은 매칭된 줄 내용을 반환하므로 Read 와 같게 막아야 한다.
	e := engineWith(nil)
	got := e.Decide(ToolCall{
		Kind: ToolContentSearch, Path: "/p/.env.keys",
		Pattern: "DOTENV_PRIVATE_KEY", Cwd: "/p",
	})
	if got.Allow {
		t.Fatal("Grep 으로 .env.keys 를 읽는 것이 허용됐다")
	}
}

func TestDecideBash(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{
		"/p/.env": EnvPlaintext,
	})
	tests := []struct {
		name      string
		cmd       string
		wantAllow bool
	}{
		{"dotenvx get 차단", "dotenvx get DART_API_KEY", false},
		{"dotenvx decrypt 차단", "dotenvx decrypt --stdout", false},
		{"redact 우회 차단", "dotenvx run --no-redact -- python x.py", false},
		{"개인키 출력 차단", "echo $DOTENV_PRIVATE_KEY", false},
		{"cat .env.keys 차단", "cat .env.keys", false},
		{"평문 .env 읽기 차단", "cat .env", false},

		{"정상 run 허용", "dotenvx run -- python x.py", true},
		{"dotenvx set 허용", "dotenvx set K v", true},
		{"무관 명령 허용", "ls -la", true},
		{"example 읽기 허용", "cat .env.example", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{Kind: ToolBash, Command: tt.cmd, Cwd: "/p"})
			if got.Allow != tt.wantAllow {
				t.Errorf("Allow = %v, want %v (reason: %s)", got.Allow, tt.wantAllow, got.Reason)
			}
		})
	}
}

func TestDecideRelativePathResolvedAgainstCwd(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{"/proj/.env": EnvPlaintext})
	got := e.Decide(ToolCall{Kind: ToolFileRead, Path: ".env", Cwd: "/proj"})
	if got.Allow {
		t.Error("Cwd 기준 상대 경로 해석이 안 됐다")
	}
}

func TestDecideAllowlistSkips(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{"/scratch/.env": EnvPlaintext})
	e.AllowRoots = []string{"/scratch"}
	got := e.Decide(ToolCall{Kind: ToolFileRead, Path: "/scratch/.env", Cwd: "/scratch"})
	if !got.Allow {
		t.Error("예외 목록 경로인데 차단됐다")
	}
}

func TestDenyReasonContainsRemedy(t *testing.T) {
	// 거부만 하면 에이전트가 우회를 시도한다. 대안을 반드시 담는다.
	e := engineWith(map[string]EnvFileKind{"/p/.env": EnvPlaintext})
	cases := map[string]string{
		"dotenvx get K": "dotenvx run",
		"cat .env":      "aeg init",
	}
	for cmd, want := range cases {
		got := e.Decide(ToolCall{Kind: ToolBash, Command: cmd, Cwd: "/p"})
		if !strings.Contains(got.Reason, want) {
			t.Errorf("%q 의 Reason 에 %q 가 없다: %s", cmd, want, got.Reason)
		}
	}
}

func TestDecideUnknownToolAllows(t *testing.T) {
	e := engineWith(nil)
	if got := e.Decide(ToolCall{Kind: ToolUnknown}); !got.Allow {
		t.Error("알 수 없는 도구는 통과시켜야 한다 (fail-open)")
	}
}
```

- [ ] **Step 3: 테스트가 실패하는지 확인**

Run: `go test ./internal/policy/ -run TestDecide -v`
Expected: 컴파일 실패 — `undefined: Engine`

- [ ] **Step 4: 최소 구현 작성**

`internal/policy/engine.go`:

```go
package policy

import (
	"fmt"
	"path/filepath"

	"github.com/stuartkim/agentenvguard/internal/allowlist"
)

type Engine struct {
	// AllowRoots 이하 경로는 판정을 건너뛴다.
	AllowRoots []string
	// ClassifyFile 은 테스트에서 주입할 수 있도록 필드로 둔다.
	ClassifyFile func(path string) EnvFileKind
}

func New() *Engine {
	return &Engine{
		AllowRoots:   allowlist.Load(),
		ClassifyFile: ClassifyEnvFile,
	}
}

func (e *Engine) classify(p string) EnvFileKind {
	if e.ClassifyFile == nil {
		return ClassifyEnvFile(p)
	}
	return e.ClassifyFile(p)
}

func allow() Decision { return Decision{Allow: true} }

func deny(format string, a ...any) Decision {
	return Decision{Allow: false, Reason: fmt.Sprintf(format, a...)}
}

// Decide 는 도구 호출 하나를 판정한다. 순수 함수에 가깝고 파일 읽기 외에
// 부작용이 없다.
func (e *Engine) Decide(tc ToolCall) Decision {
	switch tc.Kind {
	case ToolFileRead, ToolContentSearch:
		return e.decidePath(tc.Path, tc.Cwd)
	case ToolBash:
		return e.decideBash(tc)
	default:
		return allow()
	}
}

func (e *Engine) abs(p, cwd string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(cwd, p)
}

func (e *Engine) decidePath(p, cwd string) Decision {
	full := e.abs(p, cwd)
	if allowlist.Covers(e.AllowRoots, full) {
		return allow()
	}
	switch ClassifyPath(full) {
	case PathEnvKeys:
		// 파일을 열지 않고 차단한다. 읽기 실패와 무관하게 판정이 명확하다.
		return deny(msgEnvKeys, full)
	case PathEnvExample:
		return allow()
	case PathEnvFile:
		if e.classify(full) == EnvPlaintext {
			return deny(msgPlaintextEnv, full)
		}
		return allow()
	default:
		return allow()
	}
}

func (e *Engine) decideBash(tc ToolCall) Decision {
	f := AnalyzeCommand(tc.Command)

	switch f.Risk {
	case CmdDotenvxGet:
		return deny(msgDotenvxGet)
	case CmdDotenvxDecrypt:
		return deny(msgDotenvxDecrypt)
	case CmdRedactBypass:
		return deny(msgRedactBypass)
	case CmdPrivateKeyEcho:
		return deny(msgPrivateKey)
	}

	for _, p := range f.Paths {
		if d := e.decidePath(p, tc.Cwd); !d.Allow {
			return d
		}
	}
	return allow()
}

const (
	msgEnvKeys = `%s 는 dotenvx 개인키 파일입니다. 읽으면 볼트 전체가 열립니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`

	msgPlaintextEnv = `%s 는 아직 평문입니다. 읽으면 시크릿이 대화 컨텍스트에 남습니다.

  aeg init

로 암호화한 뒤 다시 시도하세요. 암호화하면 이 파일은 그대로 읽을 수 있습니다.`

	msgDotenvxGet = `dotenvx get 은 평문 값을 그대로 출력해 대화 컨텍스트에 남깁니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`

	msgDotenvxDecrypt = `dotenvx decrypt 는 볼트 전체를 평문으로 되돌립니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`

	msgRedactBypass = `--no-redact 와 --mask 0 은 자식 프로세스 출력의 마스킹을 끕니다.
스크립트가 에러 메시지에 키를 뱉으면 그대로 노출됩니다.

플래그 없이 실행하세요:
  dotenvx run -- <실행할 명령>`

	msgPrivateKey = `DOTENV_PRIVATE_KEY 를 출력하면 볼트 전체가 열립니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`
)
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/policy/ -v`
Expected: 모든 테스트 PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/policy/types.go internal/policy/engine.go internal/policy/engine_test.go
git commit -m "feat: 판정 엔진

경로·명령 분류기와 암호화 판별기를 조립해 Decide() 하나로 만든다.
.env.keys 는 파일을 열지 않고 차단한다. 모든 거부 메시지는 대안을 담는다."
```

---

### Task 6: Claude Code 어댑터

PreToolUse 의 JSON 규약을 여기에만 가둔다. 나중에 다른 에이전트를 붙일 때 이 파일만 하나 더 생긴다.

**Files:**
- Create: `internal/adapter/adapter.go`
- Create: `internal/adapter/claudecode.go`
- Test: `internal/adapter/claudecode_test.go`

**Interfaces:**
- Consumes: `policy.ToolCall`, `policy.Decision` (T5)
- Produces:
  - `type Adapter interface { Parse(io.Reader) (policy.ToolCall, error); Emit(io.Writer, policy.Decision) error }`
  - `type ClaudeCode struct{}` — 위 인터페이스 구현

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/adapter/claudecode_test.go`:

```go
package adapter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stuartkim/agentenvguard/internal/policy"
)

func TestParseBash(t *testing.T) {
	in := `{
	  "session_id": "s1",
	  "cwd": "/Users/me/proj",
	  "hook_event_name": "PreToolUse",
	  "tool_name": "Bash",
	  "tool_input": {"command": "cat .env.keys"},
	  "tool_use_id": "toolu_01"
	}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolBash {
		t.Errorf("Kind = %v, want ToolBash", got.Kind)
	}
	if got.Command != "cat .env.keys" {
		t.Errorf("Command = %q", got.Command)
	}
	if got.Cwd != "/Users/me/proj" {
		t.Errorf("Cwd = %q", got.Cwd)
	}
}

func TestParseRead(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Read","tool_input":{"file_path":"/p/.env"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolFileRead || got.Path != "/p/.env" {
		t.Errorf("got %+v", got)
	}
}

func TestParseGrep(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Grep","tool_input":{"pattern":"DOTENV_PRIVATE_KEY","path":".env.keys"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolContentSearch {
		t.Errorf("Kind = %v, want ToolContentSearch", got.Kind)
	}
	if got.Path != ".env.keys" || got.Pattern != "DOTENV_PRIVATE_KEY" {
		t.Errorf("got %+v", got)
	}
}

func TestParseUnknownToolIsUnknownKind(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"WebFetch","tool_input":{"url":"x"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolUnknown {
		t.Errorf("Kind = %v, want ToolUnknown", got.Kind)
	}
}

func TestParseMalformedReturnsError(t *testing.T) {
	if _, err := (ClaudeCode{}).Parse(strings.NewReader("{not json")); err == nil {
		t.Error("깨진 JSON 인데 에러가 없다")
	}
}

func TestEmitDeny(t *testing.T) {
	var buf bytes.Buffer
	err := ClaudeCode{}.Emit(&buf, policy.Decision{Allow: false, Reason: "안 됩니다"})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q", out.HookSpecificOutput.HookEventName)
	}
	if out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q", out.HookSpecificOutput.PermissionDecision)
	}
	if out.HookSpecificOutput.PermissionDecisionReason != "안 됩니다" {
		t.Errorf("reason = %q", out.HookSpecificOutput.PermissionDecisionReason)
	}
}

func TestEmitAllow(t *testing.T) {
	var buf bytes.Buffer
	if err := (ClaudeCode{}).Emit(&buf, policy.Decision{Allow: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"allow"`) {
		t.Errorf("allow 가 아니다: %s", buf.String())
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/adapter/ -v`
Expected: 컴파일 실패 — `undefined: ClaudeCode`

- [ ] **Step 3: 최소 구현 작성**

`internal/adapter/adapter.go`:

```go
// Package adapter 는 에이전트별 훅 규약을 판정 엔진과 분리한다.
// 새 에이전트를 지원하려면 이 인터페이스 구현을 하나 더 만든다.
package adapter

import (
	"io"

	"github.com/stuartkim/agentenvguard/internal/policy"
)

type Adapter interface {
	Parse(r io.Reader) (policy.ToolCall, error)
	Emit(w io.Writer, d policy.Decision) error
}
```

`internal/adapter/claudecode.go`:

```go
package adapter

import (
	"encoding/json"
	"io"

	"github.com/stuartkim/agentenvguard/internal/policy"
)

// ClaudeCode 는 Claude Code 의 PreToolUse 훅 규약을 다룬다.
type ClaudeCode struct{}

type preToolUseInput struct {
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command  string `json:"command"`   // Bash
		FilePath string `json:"file_path"` // Read
		Pattern  string `json:"pattern"`   // Grep
		Path     string `json:"path"`      // Grep
	} `json:"tool_input"`
}

func (ClaudeCode) Parse(r io.Reader) (policy.ToolCall, error) {
	var in preToolUseInput
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return policy.ToolCall{}, err
	}
	tc := policy.ToolCall{Cwd: in.Cwd}
	switch in.ToolName {
	case "Bash":
		tc.Kind = policy.ToolBash
		tc.Command = in.ToolInput.Command
	case "Read":
		tc.Kind = policy.ToolFileRead
		tc.Path = in.ToolInput.FilePath
	case "Grep":
		tc.Kind = policy.ToolContentSearch
		tc.Path = in.ToolInput.Path
		tc.Pattern = in.ToolInput.Pattern
	default:
		tc.Kind = policy.ToolUnknown
	}
	return tc, nil
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

type preToolUseOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

func (ClaudeCode) Emit(w io.Writer, d policy.Decision) error {
	decision := "deny"
	if d.Allow {
		decision = "allow"
	}
	return json.NewEncoder(w).Encode(preToolUseOutput{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: d.Reason,
		},
	})
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./... -v`
Expected: 모든 패키지 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/adapter/
git commit -m "feat: Claude Code PreToolUse 어댑터

Bash/Read/Grep 의 tool_input 을 도구 중립 ToolCall 로 옮기고,
판정을 hookSpecificOutput JSON 으로 낸다."
```

---

### Task 7: `aeg hook` + main + 성능 벤치마크

여기서 처음으로 실행 가능한 바이너리가 나온다. fail-open 을 실제로 보장하는 곳이기도 하다.

**Files:**
- Create: `main.go`
- Create: `internal/cmd/hook.go`
- Test: `internal/cmd/hook_test.go`

**Interfaces:**
- Consumes: `adapter.ClaudeCode`(T6), `policy.New`(T5)
- Produces:
  - `func Hook(stdin io.Reader, stdout, stderr io.Writer, e *policy.Engine) int` — 항상 0 반환

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cmd/hook_test.go`:

```go
package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stuartkim/agentenvguard/internal/policy"
)

func testEngine(kinds map[string]policy.EnvFileKind) *policy.Engine {
	return &policy.Engine{
		ClassifyFile: func(p string) policy.EnvFileKind {
			if k, ok := kinds[p]; ok {
				return k
			}
			return policy.EnvEmpty
		},
	}
}

func TestHookDeniesEnvKeys(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}`
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader(in), &out, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0 (판정은 본문에 담는다)", code)
	}
	if !strings.Contains(out.String(), `"deny"`) {
		t.Errorf("차단되지 않았다: %s", out.String())
	}
}

func TestHookAllowsNormalCommand(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`
	var out, errOut bytes.Buffer
	if code := Hook(strings.NewReader(in), &out, &errOut, testEngine(nil)); code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("허용되지 않았다: %s", out.String())
	}
}

func TestHookFailsOpenOnMalformedInput(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader("{not json"), &out, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("파싱 실패 시 통과시켜야 한다: %s", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("stderr 에 경고를 남겨야 한다")
	}
}

func TestHookFailsOpenOnPanic(t *testing.T) {
	e := &policy.Engine{
		ClassifyFile: func(string) policy.EnvFileKind { panic("의도적 패닉") },
	}
	in := `{"cwd":"/p","tool_name":"Read","tool_input":{"file_path":"/p/.env"}}`
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader(in), &out, &errOut, e)
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("패닉 시 통과시켜야 한다: %s", out.String())
	}
}

func BenchmarkHook(b *testing.B) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}`
	e := testEngine(nil)
	for i := 0; i < b.N; i++ {
		var out, errOut bytes.Buffer
		Hook(strings.NewReader(in), &out, &errOut, e)
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/cmd/ -v`
Expected: 컴파일 실패 — `undefined: Hook`

- [ ] **Step 3: 최소 구현 작성**

`internal/cmd/hook.go`:

```go
// Package cmd 는 aeg 의 서브커맨드 구현을 담는다.
package cmd

import (
	"fmt"
	"io"

	"github.com/stuartkim/agentenvguard/internal/adapter"
	"github.com/stuartkim/agentenvguard/internal/policy"
)

// Hook 은 PreToolUse 훅 본체다.
//
// 항상 0을 반환한다. 종료코드를 판정 신호로 쓰면 의도적 차단과 버그로 인한
// 비정상 종료가 구분되지 않고, 판정기가 죽었을 때 에이전트의 모든 툴 호출이
// 막힌다. 판정은 stdout JSON 본문에만 담는다.
func Hook(stdin io.Reader, stdout, stderr io.Writer, e *policy.Engine) int {
	a := adapter.ClaudeCode{}
	dec := policy.Decision{Allow: true}

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(stderr, "aeg: 내부 오류로 통과시킴: %v\n", r)
				dec = policy.Decision{Allow: true}
			}
		}()
		tc, err := a.Parse(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "aeg: stdin 파싱 실패로 통과시킴: %v\n", err)
			return
		}
		dec = e.Decide(tc)
	}()

	if err := a.Emit(stdout, dec); err != nil {
		fmt.Fprintf(stderr, "aeg: 출력 실패: %v\n", err)
	}
	return 0
}
```

`main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/stuartkim/agentenvguard/internal/cmd"
	"github.com/stuartkim/agentenvguard/internal/policy"
)

const usage = `aeg — 코딩 에이전트가 시크릿 평문을 읽지 못하게 막는다.

사용법:
  aeg install        Claude Code 전역 훅을 설치한다 (노트북당 한 번)
  aeg scan [경로]     평문 .env 를 쓰는 프로젝트를 찾는다 (기본: $HOME)
  aeg init [경로]     프로젝트 하나를 dotenvx 로 마이그레이션한다
  aeg hook           훅이 내부적으로 호출한다. 직접 쓰지 않는다

시크릿 저장·주입·마스킹은 dotenvx 가 한다. aeg 는 그 위의 가드레일이다.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hook":
		os.Exit(cmd.Hook(os.Stdin, os.Stdout, os.Stderr, policy.New()))
	default:
		fmt.Fprintf(os.Stderr, "aeg: 알 수 없는 명령 %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./... -v`
Expected: 모든 패키지 PASS

- [ ] **Step 5: 성능 예산 확인**

Run: `go test ./internal/cmd/ -bench=BenchmarkHook -benchtime=1000x -run=XXX`
Expected: `ns/op` 이 10,000,000 (10ms) 보다 훨씬 작다. 예상 범위는 수 마이크로초.

프로세스 기동 시간을 포함한 실제 측정:

```bash
go build -o aeg .
echo '{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}' > /tmp/aeg-in.json
time (for i in $(seq 1 20); do ./aeg hook < /tmp/aeg-in.json > /dev/null; done)
```

Expected: 20회 합계가 1초 미만 (1회당 50ms 미만). 넘으면 원인을 찾아 기록한다.

- [ ] **Step 6: 수동 확인 — 차단 메시지가 실제로 읽히는지**

```bash
echo '{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"dotenvx get KEY"}}' | ./aeg hook
```

Expected: `permissionDecision":"deny"` 와 `dotenvx run --` 대안이 담긴 JSON 출력

- [ ] **Step 7: 커밋**

```bash
git add main.go internal/cmd/hook.go internal/cmd/hook_test.go
git commit -m "feat: aeg hook 과 진입점

항상 exit 0 으로 끝내고 판정을 JSON 본문에 담는다. 파싱 실패와
패닉은 recover 로 잡아 allow 로 떨어뜨린다."
```

---

### Task 8: `aeg install`

`~/.claude/settings.json` 에 훅을 병합한다. 사용자의 기존 설정을 절대 잃지 않는 것이 이 태스크의 전부다.

**Files:**
- Create: `internal/cmd/install.go`
- Modify: `main.go` — `install` 케이스 추가
- Test: `internal/cmd/install_test.go`

**Interfaces:**
- Consumes: (없음)
- Produces:
  - `func SettingsPath() string` — `~/.claude/settings.json`
  - `func MergeHook(raw []byte, binPath string) (out []byte, changed bool, err error)`
  - `func Install(settingsPath, binPath string, stdout io.Writer) error`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cmd/install_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func hookCommands(t *testing.T, raw []byte) []string {
	t.Helper()
	var s struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("결과가 JSON 이 아니다: %v\n%s", err, raw)
	}
	var cmds []string
	for _, entry := range s.Hooks.PreToolUse {
		for _, h := range entry.Hooks {
			cmds = append(cmds, h.Command)
		}
	}
	return cmds
}

func TestMergeHookIntoEmptySettings(t *testing.T) {
	out, changed, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}
	cmds := hookCommands(t, out)
	if len(cmds) != 1 || cmds[0] != "/usr/local/bin/aeg" {
		t.Errorf("commands = %v", cmds)
	}
}

func TestMergeHookIsIdempotent(t *testing.T) {
	once, _, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	twice, changed, err := MergeHook(once, "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("두 번째 실행에서 changed = true. 멱등이 아니다")
	}
	if cmds := hookCommands(t, twice); len(cmds) != 1 {
		t.Errorf("훅이 중복됐다: %v", cmds)
	}
}

func TestMergeHookPreservesExistingSettings(t *testing.T) {
	in := []byte(`{
	  "model": "opus",
	  "hooks": {
	    "PreToolUse": [
	      {"matcher": "Write", "hooks": [{"type": "command", "command": "/other/hook.sh"}]}
	    ],
	    "Stop": [
	      {"matcher": "*", "hooks": [{"type": "command", "command": "/stop.sh"}]}
	    ]
	  }
	}`)
	out, changed, err := MergeHook(in, "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}

	var s map[string]any
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatal(err)
	}
	if s["model"] != "opus" {
		t.Errorf("기존 최상위 키가 사라졌다: %v", s)
	}
	hooks := s["hooks"].(map[string]any)
	if _, ok := hooks["Stop"]; !ok {
		t.Error("다른 이벤트의 훅이 사라졌다")
	}
	cmds := hookCommands(t, out)
	if len(cmds) != 2 {
		t.Errorf("기존 PreToolUse 훅과 공존해야 한다: %v", cmds)
	}
}

func TestMergeHookRejectsInvalidJSON(t *testing.T) {
	if _, _, err := MergeHook([]byte("{not json"), "/usr/local/bin/aeg"); err == nil {
		t.Error("깨진 JSON 을 덮어쓰면 안 된다. 에러를 내야 한다")
	}
}

func TestInstallWritesBackupAndFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte(`{"model":"opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if cmds := hookCommands(t, mustRead(t, p)); len(cmds) != 1 {
		t.Errorf("설치되지 않았다: %v", cmds)
	}
	if _, err := os.Stat(p + ".aeg-backup"); err != nil {
		t.Error("백업 파일이 없다")
	}
}

func TestInstallCreatesMissingSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "settings.json")
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if cmds := hookCommands(t, mustRead(t, p)); len(cmds) != 1 {
		t.Errorf("생성되지 않았다: %v", cmds)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/cmd/ -run 'TestMergeHook|TestInstall' -v`
Expected: 컴파일 실패 — `undefined: MergeHook`

- [ ] **Step 3: 최소 구현 작성**

`internal/cmd/install.go`:

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// HookMatcher 는 판정 대상 도구다.
// Grep 은 매칭된 줄 내용을 반환하므로 반드시 포함해야 한다.
const HookMatcher = "Bash|Read|Grep"

func SettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// MergeHook 은 settings.json 내용에 aeg 훅을 병합한다.
// 이미 있으면 changed=false 로 아무것도 바꾸지 않는다.
//
// map[string]any 로 다루므로 알지 못하는 설정 키도 보존된다.
// 다만 JSON 재직렬화 과정에서 키 순서가 알파벳순으로 바뀐다.
func MergeHook(raw []byte, binPath string) ([]byte, bool, error) {
	root := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			return nil, false, fmt.Errorf("settings.json 이 올바른 JSON 이 아닙니다: %w", err)
		}
	}

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)

	// 이미 설치되어 있는지 본다.
	for _, entry := range pre {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := m["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if ok && hm["command"] == binPath {
				return raw, false, nil
			}
		}
	}

	pre = append(pre, map[string]any{
		"matcher": HookMatcher,
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": binPath,
			"args":    []any{"hook"},
		}},
	})
	hooks["PreToolUse"] = pre
	root["hooks"] = hooks

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

// Install 은 settings.json 을 읽어 훅을 병합하고 되쓴다.
// 덮어쓰기 전에 .aeg-backup 을 남긴다.
func Install(settingsPath, binPath string, stdout io.Writer) error {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		raw = []byte("{}")
	} else {
		if err := os.WriteFile(settingsPath+".aeg-backup", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패: %w", err)
		}
	}

	out, changed, err := MergeHook(raw, binPath)
	if err != nil {
		return fmt.Errorf("%w\n\n%s 를 직접 고친 뒤 다시 실행하세요. 덮어쓰지 않았습니다", err, settingsPath)
	}
	if !changed {
		fmt.Fprintf(stdout, "이미 설치되어 있습니다: %s\n", settingsPath)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "설치했습니다: %s\n대상 도구: %s\n\n다음으로 평문 .env 를 찾아보세요:\n  aeg scan\n",
		settingsPath, HookMatcher)
	return nil
}
```

`main.go` 의 switch 에 추가:

```go
	case "install":
		bin, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "aeg: 실행 파일 경로를 찾을 수 없습니다: %v\n", err)
			os.Exit(1)
		}
		if err := cmd.Install(cmd.SettingsPath(), bin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aeg: %v\n", err)
			os.Exit(1)
		}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./... -v`
Expected: 모든 패키지 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/cmd/install.go internal/cmd/install_test.go main.go
git commit -m "feat: aeg install

settings.json 을 map 으로 다뤄 모르는 키까지 보존하고, 같은 command 가
있으면 아무것도 바꾸지 않는다. 덮어쓰기 전 .aeg-backup 을 남긴다."
```

---

### Task 9: `aeg scan`

노트북 전체에서 평문 `.env` 를 찾는다. "노트북 전반"이라는 요구에 직접 답하는 명령이고 실질적 첫 실행 경험이다.

**Files:**
- Create: `internal/cmd/scan.go`
- Modify: `main.go` — `scan` 케이스 추가
- Test: `internal/cmd/scan_test.go`

**Interfaces:**
- Consumes: `policy.ClassifyPath`, `policy.ClassifyEnvFile`(T1,T2)
- Produces:
  - `type Finding struct { Path string }`
  - 테스트 헬퍼 `write(t *testing.T, path, content string)` — 같은 패키지의 Task 10 테스트도 이걸 쓴다
  - `func Scan(root string, maxDepth int) (findings []Finding, skipped int)`
  - `func RunScan(root string, maxDepth int, stdout io.Writer) error`
  - `const DefaultDepth = 6`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cmd/scan_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func paths(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestScanFindsPlaintextOnly(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "a", ".env")
	enc := filepath.Join(root, "b", ".env")
	example := filepath.Join(root, "c", ".env.example")
	write(t, plain, "SECRET=abc123\n")
	write(t, enc, "DOTENV_PUBLIC_KEY=\"03\"\nSECRET=\"encrypted:xx\"\n")
	write(t, example, "SECRET=\n")

	found, _ := Scan(root, DefaultDepth)
	got := paths(found)
	if !contains(got, plain) {
		t.Errorf("평문 .env 를 못 찾았다: %v", got)
	}
	if contains(got, enc) {
		t.Errorf("암호문 .env 를 잘못 보고했다: %v", got)
	}
	if contains(got, example) {
		t.Errorf(".env.example 을 잘못 보고했다: %v", got)
	}
}

func TestScanSkipsExcludedDirs(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, "node_modules", "pkg", ".env")
	write(t, hidden, "SECRET=abc\n")
	found, _ := Scan(root, DefaultDepth)
	if contains(paths(found), hidden) {
		t.Errorf("node_modules 를 훑었다: %v", paths(found))
	}
}

func TestScanRespectsDepthLimit(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", ".env")
	write(t, deep, "SECRET=abc\n")

	if found, _ := Scan(root, 2); contains(paths(found), deep) {
		t.Error("깊이 제한을 넘어 훑었다")
	}
	if found, _ := Scan(root, 10); !contains(paths(found), deep) {
		t.Error("깊이 제한 안인데 못 찾았다")
	}
}

func TestScanFindsEnvKeys(t *testing.T) {
	// .env.keys 는 정의상 평문 개인키다. 커밋 위험이 있으므로 보고한다.
	root := t.TempDir()
	keys := filepath.Join(root, "p", ".env.keys")
	write(t, keys, "DOTENV_PRIVATE_KEY=\"122\"\n")
	if found, _ := Scan(root, DefaultDepth); !contains(paths(found), keys) {
		t.Errorf(".env.keys 를 보고하지 않았다: %v", paths(found))
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/cmd/ -run TestScan -v`
Expected: 컴파일 실패 — `undefined: Scan`

- [ ] **Step 3: 최소 구현 작성**

`internal/cmd/scan.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/stuartkim/agentenvguard/internal/policy"
)

// DefaultDepth 는 root 로부터의 최대 탐색 깊이다.
// 홈 전체를 무제한으로 훑으면 너무 느리다.
const DefaultDepth = 6

// excludedDirs 는 훑지 않는 디렉터리 이름이다.
var excludedDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true,
	"dist": true, "build": true, "target": true,
	".venv": true, "venv": true, ".cache": true, ".Trash": true,
	"Library": true, // macOS. 거대하고 프로젝트가 없다
}

// excludedPathFragments 는 경로 일부로 판단하는 제외 대상이다.
// 이름만으로 거르면 Go 프로젝트의 정상 pkg/ 디렉터리까지 건너뛰게 된다.
var excludedPathFragments = []string{
	filepath.Join("go", "pkg", "mod"),
}

// slowDirs 는 네트워크 동기화 폴더다. 만나면 경고하고 계속한다.
var slowDirs = []string{"Google Drive", "Dropbox", "CloudStorage", "OneDrive"}

type Finding struct {
	Path string
}

// Scan 은 root 아래에서 평문 .env 와 .env.keys 를 찾는다.
// 권한이 없어 못 읽은 디렉터리 수를 함께 반환한다.
func Scan(root string, maxDepth int) ([]Finding, int) {
	root = filepath.Clean(root)
	rootDepth := strings.Count(root, string(filepath.Separator))

	var findings []Finding
	skipped := 0

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			skipped++
			return nil // 권한 없는 디렉터리는 건너뛰고 계속한다
		}
		if d.IsDir() {
			if path != root && excludedDirs[d.Name()] {
				return fs.SkipDir
			}
			for _, frag := range excludedPathFragments {
				if strings.Contains(path, frag) {
					return fs.SkipDir
				}
			}
			if strings.Count(path, string(filepath.Separator))-rootDepth >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}

		// 이름으로 먼저 거른다. 내용 읽기는 후보에만 한다.
		switch policy.ClassifyPath(path) {
		case policy.PathEnvKeys:
			findings = append(findings, Finding{Path: path})
		case policy.PathEnvFile:
			if policy.ClassifyEnvFile(path) == policy.EnvPlaintext {
				findings = append(findings, Finding{Path: path})
			}
		}
		return nil
	})
	return findings, skipped
}

func RunScan(root string, maxDepth int, stdout io.Writer) error {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = home
	}
	for _, s := range slowDirs {
		if strings.Contains(root, s) {
			fmt.Fprintf(stdout, "경고: %s 는 네트워크 동기화 폴더라 느릴 수 있습니다.\n\n", s)
			break
		}
	}

	fmt.Fprintf(stdout, "%s 아래를 훑는 중 (최대 깊이 %d)...\n\n", root, maxDepth)
	findings, skipped := Scan(root, maxDepth)

	if len(findings) == 0 {
		fmt.Fprintln(stdout, "평문 시크릿 파일을 찾지 못했습니다.")
	} else {
		fmt.Fprintf(stdout, "평문 시크릿 파일 %d개:\n\n", len(findings))
		for _, f := range findings {
			fmt.Fprintf(stdout, "  %s\n", f.Path)
		}
		fmt.Fprintf(stdout, "\n각 프로젝트에서 다음을 실행하세요:\n  aeg init <프로젝트 경로>\n")
	}
	if skipped > 0 {
		fmt.Fprintf(stdout, "\n권한이 없어 건너뛴 디렉터리: %d개\n", skipped)
	}
	fmt.Fprintf(stdout, "\n느리면 범위를 좁히세요:  aeg scan ~/projects\n")
	return nil
}
```

`main.go` 의 switch 에 추가:

```go
	case "scan":
		root := ""
		if len(os.Args) > 2 {
			root = os.Args[2]
		}
		if err := cmd.RunScan(root, cmd.DefaultDepth, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aeg: %v\n", err)
			os.Exit(1)
		}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./... -v`
Expected: 모든 패키지 PASS

- [ ] **Step 5: 실제 동작 확인**

```bash
go build -o aeg . && ./aeg scan ~/projects
```

Expected: 평문 `.env` 목록 또는 "찾지 못했습니다". 수 초 내에 끝난다.

- [ ] **Step 6: 커밋**

```bash
git add internal/cmd/scan.go internal/cmd/scan_test.go main.go
git commit -m "feat: aeg scan

이름으로 먼저 거르고 내용은 후보만 읽는다. 깊이 제한 6,
node_modules/.git/Library 등 제외. 권한 오류는 세어서 보고."
```

---

### Task 10: `aeg init`

프로젝트 하나를 dotenvx 로 마이그레이션한다. 가장 중요한 단계는 **git 히스토리 경고**다. 암호화했다는 이유로 사용자를 안심시키면 안 된다.

**Files:**
- Create: `internal/cmd/initcmd.go`
- Modify: `main.go` — `init` 케이스 추가
- Test: `internal/cmd/initcmd_test.go`

**Interfaces:**
- Consumes: `policy.ClassifyEnvFile`(T1), 테스트 헬퍼 `write` (Task 9 의 `internal/cmd/scan_test.go` 에 정의. 같은 패키지라 그대로 쓴다. Task 9 를 건너뛰었다면 이 헬퍼를 먼저 만들어야 컴파일된다)
- Produces:
  - `type Runner func(dir, name string, args ...string) ([]byte, error)`
  - `func ExecRunner(dir, name string, args ...string) ([]byte, error)`
  - `func EnsureGitignore(dir, entry string) (added bool, err error)`
  - `func GitignoreHas(content, entry string) bool`
  - `func Init(dir string, run Runner, stdout io.Writer) error`

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cmd/initcmd_test.go`:

```go
package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitignoreHas(t *testing.T) {
	content := "node_modules/\n.env\n# 주석\n  .env.keys  \n"
	if !GitignoreHas(content, ".env.keys") {
		t.Error(".env.keys 를 못 찾았다 (공백 포함 줄)")
	}
	if !GitignoreHas(content, ".env") {
		t.Error(".env 를 못 찾았다")
	}
	if GitignoreHas(content, ".env.local") {
		t.Error("없는 항목을 있다고 했다")
	}
}

func TestEnsureGitignoreCreatesFile(t *testing.T) {
	dir := t.TempDir()
	added, err := EnsureGitignore(dir, ".env.keys")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("added = false, want true")
	}
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), ".env.keys") {
		t.Errorf(".gitignore 내용: %s", b)
	}
}

func TestEnsureGitignoreIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureGitignore(dir, ".env.keys"); err != nil {
		t.Fatal(err)
	}
	added, err := EnsureGitignore(dir, ".env.keys")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("두 번째 호출에서 added = true. 중복 추가된다")
	}
}

// fakeRunner 는 외부 명령을 흉내낸다.
func fakeRunner(t *testing.T, dotenvxFound bool, gitLogOutput string, calls *[]string) Runner {
	t.Helper()
	return func(dir, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		switch {
		case name == "dotenvx" && len(args) > 0 && args[0] == "--version":
			if !dotenvxFound {
				return nil, fmt.Errorf("실행 파일 없음")
			}
			return []byte("1.0.0"), nil
		case name == "dotenvx" && len(args) > 0 && args[0] == "encrypt":
			return []byte("encrypted"), nil
		case name == "git":
			return []byte(gitLogOutput), nil
		}
		return nil, nil
	}
}

func TestInitFailsWithoutDotenvx(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	err := Init(dir, fakeRunner(t, false, "", &calls), &out)
	if err == nil {
		t.Fatal("dotenvx 가 없는데 성공했다")
	}
	if !strings.Contains(err.Error(), "dotenvx") {
		t.Errorf("설치 안내가 없다: %v", err)
	}
}

func TestInitEncryptsAndAddsGitignore(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "dotenvx encrypt") {
		t.Errorf("encrypt 를 부르지 않았다: %v", calls)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || !strings.Contains(string(b), ".env.keys") {
		t.Errorf(".env.keys 가 gitignore 에 없다: %s / %v", b, err)
	}
}

func TestInitWarnsOnGitHistory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	// git log 가 커밋을 반환하면 과거에 평문이 커밋된 것이다.
	if err := Init(dir, fakeRunner(t, true, "abc123 초기 커밋\n", &calls), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "로테이션") {
		t.Errorf("히스토리 경고와 로테이션 권고가 없다:\n%s", out.String())
	}
}

func TestInitNoWarnWhenHistoryClean(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "로테이션") {
		t.Errorf("히스토리가 깨끗한데 경고했다:\n%s", out.String())
	}
}

func TestInitSkipsAlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"),
		"DOTENV_PUBLIC_KEY=\"03\"\nSECRET=\"encrypted:xx\"\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(calls, "\n"), "encrypt") {
		t.Error("이미 암호화된 파일을 다시 암호화했다")
	}
}

func TestInitAdvisesButDoesNotRemoveEnvFromGitignore(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	write(t, filepath.Join(dir, ".gitignore"), ".env\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !GitignoreHas(string(b), ".env") {
		t.Error(".gitignore 에서 .env 를 자동 제거했다. 안내만 해야 한다")
	}
	if !strings.Contains(out.String(), ".gitignore") {
		t.Errorf("안내 문구가 없다:\n%s", out.String())
	}
}
```

- [ ] **Step 2: 테스트가 실패하는지 확인**

Run: `go test ./internal/cmd/ -run TestInit -v`
Expected: 컴파일 실패 — `undefined: Init`

- [ ] **Step 3: 최소 구현 작성**

`internal/cmd/initcmd.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stuartkim/agentenvguard/internal/policy"
)

// Runner 는 외부 명령 실행을 추상화한다. 테스트에서 주입한다.
type Runner func(dir, name string, args ...string) ([]byte, error)

func ExecRunner(dir, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	return c.CombinedOutput()
}

func GitignoreHas(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

// EnsureGitignore 는 entry 가 없으면 추가한다. 이미 있으면 아무것도 하지 않는다.
func EnsureGitignore(dir, entry string) (bool, error) {
	p := filepath.Join(dir, ".gitignore")
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	content := string(b)
	if GitignoreHas(content, entry) {
		return false, nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Init 은 프로젝트 하나를 dotenvx 로 마이그레이션한다.
func Init(dir string, run Runner, stdout io.Writer) error {
	envPath := filepath.Join(dir, ".env")
	if _, err := os.Stat(envPath); err != nil {
		return fmt.Errorf("%s 에 .env 가 없습니다", dir)
	}

	if _, err := run(dir, "dotenvx", "--version"); err != nil {
		return fmt.Errorf(`dotenvx 가 설치되어 있지 않습니다.

  brew install dotenvx

또는 https://dotenvx.com 의 설치 방법을 따르세요`)
	}

	switch policy.ClassifyEnvFile(envPath) {
	case policy.EnvEncrypted:
		fmt.Fprintf(stdout, "이미 암호화되어 있습니다: %s\n", envPath)
	case policy.EnvEmpty:
		fmt.Fprintf(stdout, "%s 에 값이 없습니다. 암호화를 건너뜁니다.\n", envPath)
	default:
		out, err := run(dir, "dotenvx", "encrypt")
		if err != nil {
			return fmt.Errorf("dotenvx encrypt 실패: %w\n%s", err, out)
		}
		fmt.Fprintf(stdout, "암호화했습니다: %s\n", envPath)
	}

	added, err := EnsureGitignore(dir, ".env.keys")
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintln(stdout, ".gitignore 에 .env.keys 를 추가했습니다.")
	}

	// .env 는 자동으로 건드리지 않는다. 안내만 한다.
	if b, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
		if GitignoreHas(string(b), ".env") {
			fmt.Fprintln(stdout, `
안내: .gitignore 에 .env 가 있습니다. 암호화된 .env 는 커밋해도 안전하므로
그 줄을 빼면 팀과 공유할 수 있습니다. 자동으로 빼지 않았습니다 — 커밋하지
않아도 aeg 와 dotenvx 는 정상 동작합니다.`)
		}
	}

	// 가장 중요한 단계. 암호화해도 히스토리의 평문은 남는다.
	if out, err := run(dir, "git", "log", "--all", "--oneline", "--", ".env"); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			fmt.Fprintf(stdout, `
경고: 평문 .env 가 과거에 커밋된 적이 있습니다.

%s
암호화해도 git 히스토리의 평문은 그대로 남습니다. 해당 키를 발급처에서
로테이션하세요. 암호화했다는 이유로 안심하면 안 됩니다.
`, strings.TrimSpace(string(out)))
		}
	}

	fmt.Fprintf(stdout, "\n이제 스크립트를 이렇게 실행하세요:\n  dotenvx run -- <실행할 명령>\n")
	return nil
}
```

`main.go` 의 switch 에 추가:

```go
	case "init":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		if err := cmd.Init(dir, cmd.ExecRunner, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aeg: %v\n", err)
			os.Exit(1)
		}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./... -v`
Expected: 모든 패키지 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/cmd/initcmd.go internal/cmd/initcmd_test.go main.go
git commit -m "feat: aeg init

dotenvx encrypt 로 마이그레이션하고 .env.keys 를 gitignore 에 넣는다.
.gitignore 의 .env 는 자동 제거하지 않고 안내만 한다.
평문 .env 가 커밋된 적 있으면 키 로테이션을 경고한다."
```

---

### Task 11: 실제 검증 + README + LICENSE

여기서 처음으로 도구를 자기 자신에게 걸어 실제로 동작하는지 확인한다. 단위 테스트가 다 통과해도 실제 Claude Code 훅으로 동작하지 않으면 아무 의미가 없다.

**Files:**
- Create: `README.md`, `LICENSE`
- Test: 수동 통합 검증

**Interfaces:**
- Consumes: 전체
- Produces: (없음)

- [ ] **Step 1: dotenvx 설치**

```bash
command -v dotenvx || brew install dotenvx
dotenvx --version
```

- [ ] **Step 2: 임시 프로젝트로 전체 흐름 검증**

```bash
go build -o aeg .
TMP=$(mktemp -d)
cd "$TMP" && git init -q
printf 'DART_API_KEY=realsecret123\n' > .env
~/projects/agentenvguard/aeg init "$TMP"
cat .env          # 값이 encrypted: 로 바뀌어 있어야 한다
cat .gitignore    # .env.keys 가 있어야 한다
```

Expected: `.env` 의 값이 `encrypted:` 로 시작하고, `.env.keys` 가 생성되고 gitignore 에 등록됨

- [ ] **Step 3: 판정기가 실제로 막는지 확인**

```bash
cd ~/projects/agentenvguard
J() { printf '{"cwd":"%s","tool_name":"%s","tool_input":%s}' "$1" "$2" "$3"; }

# 차단되어야 하는 것
J "$TMP" Bash '{"command":"cat .env.keys"}'      | ./aeg hook
J "$TMP" Grep '{"pattern":"KEY","path":".env.keys"}' | ./aeg hook
J "$TMP" Bash '{"command":"dotenvx get DART_API_KEY"}' | ./aeg hook

# 허용되어야 하는 것 (.env 는 이제 암호문)
J "$TMP" Read '{"file_path":"'"$TMP"'/.env"}'    | ./aeg hook
J "$TMP" Bash '{"command":"dotenvx run -- python x.py"}' | ./aeg hook
```

Expected: 앞 세 개는 `"permissionDecision":"deny"`, 뒤 두 개는 `"allow"`

- [ ] **Step 4: 실제 Claude Code 에 설치해 확인**

```bash
sudo cp aeg /usr/local/bin/aeg   # 또는 PATH 상의 다른 위치
aeg install
cat ~/.claude/settings.json      # 훅이 병합됐는지 확인
aeg install                      # 두 번째 실행: "이미 설치되어 있습니다"
```

그다음 **새 Claude Code 세션**에서 `cat .env.keys` 를 시켜본다.

Expected: 차단되고 `dotenvx run --` 대안 메시지가 보인다.

이 단계가 실패하면 되돌린다:

```bash
cp ~/.claude/settings.json.aeg-backup ~/.claude/settings.json
```

- [ ] **Step 5: README 작성**

`README.md`:

````markdown
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

## 설치

```bash
brew install dotenvx          # 볼트는 dotenvx 가 맡는다
go install github.com/stuartkim/agentenvguard@latest
aeg install                   # 노트북당 한 번
```

## 사용

```bash
aeg scan                      # 평문 .env 를 쓰는 프로젝트를 전부 찾는다
aeg init ~/projects/myapp     # 하나를 dotenvx 로 마이그레이션한다
dotenvx run -- python app.py  # 이제부터 스크립트는 이렇게 실행한다
```

`aeg scan` 은 기본으로 `$HOME` 아래를 깊이 6까지 훑는다.
`node_modules`, `.git`, `Library` 등은 건너뛴다. 느리면 범위를 좁힌다.

## 예외 처리

오탐이 나면 `~/.config/aeg/allow` 에 경로를 한 줄씩 적는다.
그 경로 이하는 판정을 건너뛴다.

## 라이선스

MIT
````

- [ ] **Step 6: LICENSE 작성**

```bash
YEAR=$(date +%Y)
cat > LICENSE <<EOF
MIT License

Copyright (c) $YEAR Stuart Kim

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
EOF
```

- [ ] **Step 7: 전체 검증**

```bash
go vet ./...
go test ./... -v
gofmt -l .        # 출력이 비어 있어야 한다
```

Expected: vet 무출력, 모든 테스트 PASS, gofmt 무출력

- [ ] **Step 8: 커밋**

```bash
git add README.md LICENSE
git commit -m "docs: README 와 MIT 라이선스

무엇을 막고 무엇을 못 막는지 README 에 명시한다. 우발적 노출 차단이
목표이고 의도적 탈취는 막지 못한다는 것을 흐리지 않는다."
```

- [ ] **Step 9: 공개 전 남은 항목을 이슈로 기록**

공개 배포는 이 계획의 범위 밖이다. 다만 다음을 잊지 않도록 적어둔다.

1. **메시지 영어화.** 현재 거부 메시지와 CLI 출력이 한국어다. 공개 배포하려면
   영어가 기본이어야 한다. `internal/policy/engine.go` 의 `msg*` 상수와
   `internal/cmd/*.go` 의 출력 문구가 대상이다.
2. **`aeg` 이름 최종 확인.** 설계 시점에 PATH·Homebrew 는 비어 있었다.
   공개 직전 npm·crates.io·GitHub 도 확인한다.
3. **크로스 컴파일 배포.** `GOOS`/`GOARCH` 조합으로 바이너리를 만들고
   Homebrew tap 또는 `curl | sh` 채널을 준비한다. Windows 경로 처리
   (`filepath.Separator`)는 이미 고려되어 있으나 실제 테스트가 필요하다.
4. **다른 에이전트 어댑터.** `adapter.Adapter` 인터페이스가 이미 분리되어
   있으므로 Cursor·Codex 지원은 구현 하나를 추가하는 일이다.
