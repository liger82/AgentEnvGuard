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
