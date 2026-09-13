package policy

import (
	"fmt"
	"path/filepath"

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
