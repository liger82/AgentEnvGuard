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
