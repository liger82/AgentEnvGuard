package policy

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/liger82/AgentEnvGuard/internal/allowlist"
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
	case ToolFileRead:
		return e.decidePath(tc.Path, tc.Cwd)
	case ToolContentSearch:
		return e.decideContentSearch(tc)
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
	return e.decidePathKind(full)
}

// decidePathKind 는 (allowlist 를 통과한) 경로 하나를 종류에 따라 판정한다.
// decidePath 와 decideContentSearch 가 공유한다.
func (e *Engine) decidePathKind(full string) Decision {
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

// isPrivateKeyPattern 은 Grep 패턴이 dotenvx 개인키를 노린 것인지 본다.
// 대소문자를 가리지 않는다.
func isPrivateKeyPattern(pattern string) bool {
	return strings.Contains(strings.ToUpper(pattern), "DOTENV_PRIVATE_KEY")
}

// decideContentSearch 는 Grep 을 판정한다. Grep 은 path 가 비어 있거나
// 디렉터리여도(재귀 검색) 동작하므로, 경로 판정만으로는 개인키를 노리는
// 패턴을 잡을 수 없다. path 판정에 더해 패턴 자체도 본다.
//
// 무해한 검색(예: "func main")을 path 없이 실행하는 것은 Grep 의 주된
// 용도이므로 막지 않는다 — 막는 것은 어디까지나 개인키를 노리는 패턴이다.
func (e *Engine) decideContentSearch(tc ToolCall) Decision {
	full := e.abs(tc.Path, tc.Cwd)
	if allowlist.Covers(e.AllowRoots, full) {
		return allow()
	}
	if isPrivateKeyPattern(tc.Pattern) {
		return deny(msgGrepPrivateKey)
	}
	return e.decidePathKind(full)
}

func (e *Engine) decideBash(tc ToolCall) Decision {
	f := AnalyzeCommand(tc.Command)

	switch f.Risk {
	case CmdDotenvxGet:
		return deny(msgDotenvxGet)
	case CmdDotenvxDecrypt:
		return deny(msgDotenvxDecrypt)
	case CmdDotenvxKeypair:
		return deny(msgDotenvxKeypair)
	case CmdEnvDump:
		return deny(msgEnvDump)
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

	msgDotenvxKeypair = `dotenvx keypair 는 DOTENV_PRIVATE_KEY 를 그대로 출력합니다. 읽으면 볼트 전체가 열립니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`

	msgEnvDump = `dotenvx run 은 복호화한 평문 값을 그대로 주입합니다. 자식 명령이 printenv, env
처럼 환경변수를 출력하면 시크릿이 대화 컨텍스트에 남습니다.

실행할 명령 자체가 환경변수를 출력하지 않게 하세요:
  dotenvx run -- <실행할 명령>`

	msgPrivateKey = `DOTENV_PRIVATE_KEY 를 출력하면 볼트 전체가 열립니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`

	msgGrepPrivateKey = `이 검색 패턴은 DOTENV_PRIVATE_KEY 를 찾습니다. 경로가 없거나
디렉터리여도 매칭된 줄 내용에 개인키가 그대로 노출됩니다.

값을 직접 볼 필요 없이 다음으로 실행하세요:
  dotenvx run -- <실행할 명령>`
)
