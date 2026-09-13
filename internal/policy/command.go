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
			if strings.HasPrefix(tok, "--mask=") && strings.HasSuffix(tok, "=0") {
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
