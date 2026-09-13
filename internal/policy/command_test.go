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
		{"no-redact", "dotenvx run --no-redact -- python x.py", CmdRedactBypass},
		{"mask 0", "dotenvx run --mask 0 -- node a.js", CmdRedactBypass},
		{"mask=0", "dotenvx run --mask=0 -- node a.js", CmdRedactBypass},
		{"mask=1", "dotenvx run --mask=1 -- node a.js", CmdSafe},
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

		{"큰따옴표 .env.keys", `cat ".env.keys"`, []string{".env.keys"}},
		{"작은따옴표 .env.keys", `cat '.env.keys'`, []string{".env.keys"}},
		{"큰따옴표 평문 .env", `head ".env"`, []string{".env"}},
		{"작은따옴표 절대경로", `cat '/Users/me/p/.env'`, []string{"/Users/me/p/.env"}},
		{"공백 있는 따옴표 경로는 한 토큰", `cat "/Users/me/my proj/.env.keys"`, []string{"/Users/me/my proj/.env.keys"}},
		{"따옴표 리다이렉션 대상", `base64 < ".env.keys"`, []string{".env.keys"}},
		{"따옴표 안의 구분자는 세그먼트를 나누지 않는다", `grep "a|b" .env`, []string{".env"}},
		{"따옴표 안의 명령 이름은 명령이 아니다", `git commit -m "cat .env"`, nil},

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
