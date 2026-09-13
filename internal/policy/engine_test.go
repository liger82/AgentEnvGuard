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

// Grep 의 path 는 선택 사항이다 — 비워두면 작업 디렉터리를 재귀 검색하고,
// 디렉터리를 줘도 마찬가지다. 경로 판정만으로는 이 경우를 잡을 수 없으므로
// 패턴 자체에 DOTENV_PRIVATE_KEY 가 있는지 본다.
func TestDecideGrepPatternWithoutPath(t *testing.T) {
	e := engineWith(nil)
	tests := []struct {
		name      string
		path      string
		pattern   string
		wantAllow bool
	}{
		{"path 없이 개인키 패턴 차단", "", "DOTENV_PRIVATE_KEY", false},
		{"디렉터리 path 에 개인키 패턴 차단", ".", "DOTENV_PRIVATE_KEY", false},
		{"대소문자 무관하게 차단", "", "dotenv_private_key", false},
		{"무해한 패턴은 path 없이도 허용", "", "func main", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{
				Kind: ToolContentSearch, Path: tt.path,
				Pattern: tt.pattern, Cwd: "/p",
			})
			if got.Allow != tt.wantAllow {
				t.Errorf("Allow = %v, want %v (reason: %s)", got.Allow, tt.wantAllow, got.Reason)
			}
			if !got.Allow {
				if got.Reason == "" {
					t.Error("차단인데 Reason 이 비었다")
				}
				if !strings.Contains(got.Reason, "dotenvx run") {
					t.Errorf("Reason 에 dotenvx run 대안이 없다: %s", got.Reason)
				}
			}
		})
	}
}

// path 가 비어 있어도 Read 는 (Grep 과 달리) 재귀 검색을 하지 않으므로
// 기존 fail-open 동작이 그대로 유지되어야 한다.
func TestDecideFileReadEmptyPathAllows(t *testing.T) {
	e := engineWith(nil)
	got := e.Decide(ToolCall{Kind: ToolFileRead, Path: "", Cwd: "/p"})
	if !got.Allow {
		t.Errorf("path 없는 Read 는 허용돼야 한다 (reason: %s)", got.Reason)
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
		{"큰따옴표 .env.keys 차단", `cat ".env.keys"`, false},
		{"작은따옴표 .env.keys 차단", `cat '.env.keys'`, false},
		{"큰따옴표 평문 .env 차단", `cat ".env"`, false},
		{"작은따옴표 평문 .env 차단", `cat '.env'`, false},
		{"따옴표 절대경로 평문 .env 차단", `cat "/p/.env"`, false},
		{"따옴표 절대경로 .env.keys 차단", `cat '/p/.env.keys'`, false},

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
