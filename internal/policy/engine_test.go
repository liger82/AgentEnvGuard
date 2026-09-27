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

// 패턴이 무해해도 path 가 .env.keys 면 경로만으로 차단해야 한다.
func TestDecideGrepBenignPatternOnEnvKeysPath(t *testing.T) {
	e := engineWith(nil)
	got := e.Decide(ToolCall{Kind: ToolContentSearch, Path: ".env.keys", Pattern: "HELLO", Cwd: "/p"})
	if got.Allow {
		t.Fatal("무해한 패턴으로 .env.keys 를 Grep 하는 것이 허용됐다")
	}
}

// Grep 의 glob 은 검색할 파일을 고른다. glob 이 .env.keys 를 가리키면 path 가
// 디렉터리여도 개인키 줄이 나온다.
func TestDecideGrepGlob(t *testing.T) {
	e := engineWith(nil)
	tests := []struct {
		name      string
		path      string
		glob      string
		wantAllow bool
	}{
		{"glob .env.keys 차단", "", ".env.keys", false},
		{"glob .env.keys* 차단", "", ".env.keys*", false},
		{"하위 경로 glob 차단", "", "**/.env.keys", false},
		{"디렉터리 path + glob 차단", "/p/sub", ".env.keys", false},
		{"무관한 glob 허용", "", "*.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{
				Kind: ToolContentSearch, Path: tt.path, Glob: tt.glob,
				Pattern: "HELLO", Cwd: "/p",
			})
			if got.Allow != tt.wantAllow {
				t.Errorf("Allow = %v, want %v (reason: %s)", got.Allow, tt.wantAllow, got.Reason)
			}
		})
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
		{"dotenvx keypair 차단", "dotenvx keypair", false},
		{"npx dotenvx get 차단", "npx @dotenvx/dotenvx get X", false},
		{"sudo dotenvx decrypt 차단", "sudo dotenvx decrypt --stdout", false},
		{"run -- printenv 차단", "dotenvx run -- printenv", false},
		{"run -- env 차단", "dotenvx run -- env", false},
		{"run -f -- printenv X 차단", "dotenvx run -f .env.production -- printenv X", false},
		{"개인키 출력 차단", "echo $DOTENV_PRIVATE_KEY", false},
		{"cat .env.keys 차단", "cat .env.keys", false},
		{"평문 .env 읽기 차단", "cat .env", false},
		{"큰따옴표 .env.keys 차단", `cat ".env.keys"`, false},
		{"작은따옴표 .env.keys 차단", `cat '.env.keys'`, false},
		{"큰따옴표 평문 .env 차단", `cat ".env"`, false},
		{"작은따옴표 평문 .env 차단", `cat '.env'`, false},
		{"따옴표 절대경로 평문 .env 차단", `cat "/p/.env"`, false},
		{"따옴표 절대경로 .env.keys 차단", `cat '/p/.env.keys'`, false},
		{"diff 로 평문 .env 읽기 차단", "diff .env .env.example", false},
		{"jq 로 평문 .env 읽기 차단", "jq . .env", false},
		{"공백 없는 리다이렉션 차단", "cat<.env", false},

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

// 백슬래시 이스케이프는 bash 규칙을 따라야 한다. \" 나 \' 를 따옴표 시작으로
// 잘못 보면 뒤따르는 명령 전체가 한 토큰으로 삼켜져 검사를 피한다.
func TestDecideBashBackslashEscapes(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{"/p/.env": EnvPlaintext})
	tests := []struct {
		name      string
		cmd       string
		wantAllow bool
	}{
		{"작은따옴표 안 이스케이프 관용구 뒤 .env.keys", `echo 'it'\''s'; cat .env.keys`, false},
		{"큰따옴표 안 이스케이프 따옴표 뒤 .env.keys", `echo "a \" b"; cat .env.keys`, false},
		{"따옴표 밖 이스케이프 작은따옴표 뒤 .env.keys", `echo it\'s; cat .env.keys`, false},
		{"따옴표 밖 이스케이프 큰따옴표 뒤 평문 .env", `echo \"; cat .env`, false},
		{"printf 큰따옴표 이스케이프 뒤 dotenvx get", `printf '%s\n' "don\"t" && dotenvx get X`, false},
		{"이스케이프 관용구 뒤 decrypt", `echo 'it'\''s' && dotenvx decrypt --stdout`, false},
		{"한 세그먼트 안 큰따옴표 이스케이프", `grep "a\"b" .env`, false},

		{"큰따옴표 이스케이프만 있으면 허용", `echo "a \" b"`, true},
		{"따옴표 밖 이스케이프만 있으면 허용", `echo it\'s`, true},
		{"이스케이프된 공백 경로는 한 토큰", `cat my\ notes.txt`, true},
		{"작은따옴표 안 백슬래시는 글자 그대로", `echo 'a\'`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{Kind: ToolBash, Command: tt.cmd, Cwd: "/p"})
			if got.Allow != tt.wantAllow {
				t.Errorf("%s: Allow = %v, want %v (reason: %s)", tt.cmd, got.Allow, tt.wantAllow, got.Reason)
			}
		})
	}
}

// 리다이렉션 연산자는 붙여 써도 앞뒤 토큰과 분리해야 한다. 출력 리다이렉션의
// 대상은 읽기가 아니므로 그 자체로 차단 사유가 되지 않는다.
func TestDecideBashRedirections(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{"/p/.env": EnvPlaintext})
	tests := []struct {
		name      string
		cmd       string
		wantAllow bool
	}{
		{"붙여 쓴 > 앞의 .env.keys", "cat .env.keys>/tmp/x", false},
		{"붙여 쓴 >> 앞의 .env.keys", "cat .env.keys>>out", false},
		{"2> 와 함께", "cat .env.keys 2>/dev/null", false},
		{"&> 붙여 쓰기", "cat .env.keys&>log", false},
		{"2>&1 뒤 파이프", "cat .env 2>&1 | head", false},
		{"붙여 쓴 > 앞의 평문 .env", "cat .env>/tmp/x", false},

		{"출력 리다이렉션만", "echo hi>out.txt", true},
		{"출력 대상이 평문 .env", "echo hi > .env", true},
		{"붙여 쓴 출력 대상이 평문 .env", "echo hi>.env", true},
		{"cat 의 출력 대상이 평문 .env", "cat foo.txt > .env", true},
		{"cat 의 추가 출력 대상이 평문 .env", "cat foo.txt >> .env", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{Kind: ToolBash, Command: tt.cmd, Cwd: "/p"})
			if got.Allow != tt.wantAllow {
				t.Errorf("%s: Allow = %v, want %v (reason: %s)", tt.cmd, got.Allow, tt.wantAllow, got.Reason)
			}
		})
	}
}

// sudo, env VAR=1, nohup 같은 접두어 뒤의 읽기 명령도 읽기 명령으로 봐야 한다.
func TestDecideBashCommandPrefixes(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{"/p/.env": EnvPlaintext})
	tests := []struct {
		name      string
		cmd       string
		wantAllow bool
	}{
		{"sudo", "sudo cat .env.keys", false},
		{"sudo -u root", "sudo -u root cat .env.keys", false},
		{"sudo 옵션 여러 개", "sudo -E -g staff head .env.keys", false},
		{"env 할당", "env FOO=1 cat .env.keys", false},
		{"env 옵션과 할당", "env -i FOO=1 BAR=2 cat .env.keys", false},
		{"맨 앞 할당", "FOO=1 cat .env", false},
		{"nohup", "nohup cat .env.keys", false},
		{"time", "time cat .env.keys", false},
		{"command", "command cat .env.keys", false},
		{"builtin source", "builtin source .env", false},
		{"exec", "exec cat .env.keys", false},
		{"nice -n", "nice -n 10 cat .env.keys", false},
		{"doas", "doas cat .env.keys", false},
		{"중첩 접두어", "sudo env FOO=1 nohup cat .env.keys", false},

		{"sudo ls", "sudo ls", true},
		{"env 로 다른 명령 실행", "env FOO=1 python app.py", true},
		{"sudo 뒤 무관한 파일", "sudo cat /etc/hosts", true},
		{"접두어만", "sudo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{Kind: ToolBash, Command: tt.cmd, Cwd: "/p"})
			if got.Allow != tt.wantAllow {
				t.Errorf("%s: Allow = %v, want %v (reason: %s)", tt.cmd, got.Allow, tt.wantAllow, got.Reason)
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

// 셸이 확장하는 ~, $HOME, ${HOME} 은 Cwd 기준 상대 경로가 아니라 홈 경로다.
func TestDecideBashExpandsHome(t *testing.T) {
	e := engineWith(map[string]EnvFileKind{
		"/home/me/.env":   EnvPlaintext,
		"/home/me/p/.env": EnvPlaintext,
	})
	e.Home = "/home/me"
	tests := []struct {
		name string
		cmd  string
	}{
		{"물결표", "cat ~/.env"},
		{"$HOME", "cat $HOME/p/.env"},
		{"${HOME}", "head ${HOME}/.env"},
		{"따옴표 안 $HOME", `cat "$HOME/.env"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Decide(ToolCall{Kind: ToolBash, Command: tt.cmd, Cwd: "/p"})
			if got.Allow {
				t.Errorf("%q 가 허용됐다 — 홈 경로를 펼치지 않았다", tt.cmd)
			}
		})
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
		"dotenvx get K":           "dotenvx run",
		"dotenvx keypair":         "dotenvx run",
		"dotenvx run -- printenv": "dotenvx run",
		"cat .env":                "aeg init",
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
