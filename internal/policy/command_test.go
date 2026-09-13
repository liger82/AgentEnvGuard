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
		{"dotenvx keypair", "dotenvx keypair", CmdDotenvxKeypair},
		{"dotenvx keypair 형식 지정", "dotenvx keypair -f .env.production --format shell", CmdDotenvxKeypair},
		{"run -- printenv", "dotenvx run -- printenv", CmdEnvDump},
		{"run -- printenv NAME", "dotenvx run -- printenv HELLO", CmdEnvDump},
		{"run -- env", "dotenvx run -- env", CmdEnvDump},
		{"run -- 절대경로 env", "dotenvx run -- /usr/bin/env", CmdEnvDump},
		{"run -- env 할당만", "dotenvx run -- env FOO=1", CmdEnvDump},
		{"run -f 뒤 printenv", "dotenvx run -f .env.production -- printenv X", CmdEnvDump},
		{"run -- set", "dotenvx run -- set", CmdEnvDump},
		{"run -- export -p", "dotenvx run -- export -p", CmdEnvDump},
		{"run -- declare -x", "dotenvx run -- declare -x", CmdEnvDump},
		{"run -- declare -p", "dotenvx run -- declare -p", CmdEnvDump},
		{"run -- env 로 명령 실행은 허용", "dotenvx run -- env FOO=1 python app.py", CmdSafe},
		{"run --redact 는 안전한 옵션", "dotenvx run --redact -- python x.py", CmdSafe},
		{"run --mask 는 안전한 옵션", "dotenvx run --mask 0 -- node a.js", CmdSafe},
		{"run -- 뒤가 아닌 printenv 인자", "dotenvx run -- python printenv.py", CmdSafe},
		{"private key echo", "echo $DOTENV_PRIVATE_KEY", CmdPrivateKeyEcho},
		{"private key printenv", "printenv DOTENV_PRIVATE_KEY", CmdPrivateKeyEcho},

		{"npx scoped get", "npx @dotenvx/dotenvx get HELLO", CmdDotenvxGet},
		{"npx 버전 지정 get", "npx @dotenvx/dotenvx@1.2.3 get HELLO", CmdDotenvxGet},
		{"npx decrypt", "npx dotenvx decrypt --stdout", CmdDotenvxDecrypt},
		{"npx -y keypair", "npx -y dotenvx@latest keypair", CmdDotenvxKeypair},
		{"bunx get", "bunx @dotenvx/dotenvx get K", CmdDotenvxGet},
		{"pnpm exec decrypt", "pnpm exec dotenvx decrypt", CmdDotenvxDecrypt},
		{"pnpm dlx get", "pnpm dlx @dotenvx/dotenvx get K", CmdDotenvxGet},
		{"sudo get", "sudo dotenvx get K", CmdDotenvxGet},
		{"env 할당 뒤 get", "env FOO=1 dotenvx get K", CmdDotenvxGet},
		{"npx run -- printenv", "npx @dotenvx/dotenvx run -- printenv", CmdEnvDump},
		{"npx 정상 run", "npx @dotenvx/dotenvx run -- node a.js", CmdSafe},

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

		{"큰따옴표 .env.keys", `cat ".env.keys"`, []string{".env.keys"}},
		{"작은따옴표 .env.keys", `cat '.env.keys'`, []string{".env.keys"}},
		{"큰따옴표 평문 .env", `head ".env"`, []string{".env"}},
		{"작은따옴표 절대경로", `cat '/Users/me/p/.env'`, []string{"/Users/me/p/.env"}},
		{"공백 있는 따옴표 경로는 한 토큰", `cat "/Users/me/my proj/.env.keys"`, []string{"/Users/me/my proj/.env.keys"}},
		{"따옴표 리다이렉션 대상", `base64 < ".env.keys"`, []string{".env.keys"}},
		{"따옴표 안의 구분자는 세그먼트를 나누지 않는다", `grep "a|b" .env`, []string{".env"}},
		{"따옴표 안의 명령 이름은 명령이 아니다", `git commit -m "cat .env"`, nil},

		{"diff", "diff .env .env.example", []string{".env"}},
		{"tac", "tac .env", []string{".env"}},
		{"rev", "rev .env.keys", []string{".env.keys"}},
		{"paste", "paste .env", []string{".env"}},
		{"column", "column -t -s= .env", []string{".env"}},
		{"jq", "jq . .env", []string{".env"}},
		{"yq", "yq e . .env.local", []string{".env.local"}},
		{"vimdiff", "vimdiff .env .env.bak", []string{".env", ".env.bak"}},
		{"cmp", "cmp .env.keys /tmp/k", []string{".env.keys"}},
		{"공백 없는 리다이렉션", "cat<.env", []string{".env"}},
		{"공백 없는 리다이렉션 대상", "base64 <.env.keys", []string{".env.keys"}},
		{"홈 경로는 그대로 돌려준다", "cat ~/.env", []string{"~/.env"}},

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
