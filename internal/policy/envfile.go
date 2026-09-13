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
