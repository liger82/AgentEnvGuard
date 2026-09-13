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
	CmdDotenvxKeypair
	CmdEnvDump
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
	case CmdDotenvxKeypair:
		return "CmdDotenvxKeypair"
	case CmdEnvDump:
		return "CmdEnvDump"
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
	"diff": true, "vimdiff": true, "cmp": true, "tac": true, "rev": true,
	"paste": true, "column": true, "jq": true, "yq": true,
}

// segment 는 셸 명령 하나(구분자 사이)를 토큰으로 자른 결과다.
type segment struct {
	// words 는 리다이렉션을 뺀 명령과 인자다.
	words []string
	// inputs 는 입력 리다이렉션(<, <>)의 대상 — 명령이 내용을 읽는 파일이다.
	inputs []string
	// others 는 나머지 리다이렉션 대상이다. 출력 파일(>, >>, &>), fd 복제
	// (>&1), heredoc 구분자(<<), here-string(<<<) 은 읽기가 아니다.
	others []string
}

// redirKind 는 직전에 읽은 리다이렉션 연산자가 다음 토큰을 어디로 보낼지다.
type redirKind int

const (
	redirNone redirKind = iota
	redirInput
	redirOther
)

// splitSegments 는 명령 문자열을 셸 명령 단위(세그먼트)로 나누고, 각
// 세그먼트를 토큰으로 자른다.
//
// 따옴표를 최소한으로 이해한다. 짝이 맞는 "..." 와 '...' 는 한 토큰으로 묶고
// 따옴표 자체는 벗긴다 — cat ".env.keys" 가 cat .env.keys 와 같게 보이도록.
// 따옴표 안의 공백과 구분자(&&, ||, ;, |, 줄바꿈)는 토큰이나 세그먼트를
// 나누지 않는다. 백슬래시는 bash 규칙을 따른다 — 따옴표 밖에서는 다음 바이트를
// 이스케이프하고, 큰따옴표 안에서는 " \ $ ` 와 줄바꿈만, 작은따옴표 안에서는
// 아무것도 이스케이프하지 않는다. 변수 확장까지 흉내 내지는 않는다.
//
// 따옴표 밖의 리다이렉션 연산자(<, >, >>, <<<, 2>, &>, >&, 2>&1 등)는 붙여
// 써도 앞뒤 토큰과 떼어내고, 그 대상 토큰은 words 가 아니라 inputs/others 로
// 보낸다.
func splitSegments(cmd string) []segment {
	var (
		segs    []segment
		seg     segment
		cur     strings.Builder
		inTok   bool
		literal bool // 현재 토큰에 따옴표나 이스케이프가 쓰였다 — fd 번호가 아니다
		quote   byte
		pending redirKind
	)
	flushTok := func() {
		if !inTok {
			return
		}
		switch pending {
		case redirInput:
			seg.inputs = append(seg.inputs, cur.String())
		case redirOther:
			seg.others = append(seg.others, cur.String())
		default:
			seg.words = append(seg.words, cur.String())
		}
		pending = redirNone
		cur.Reset()
		inTok, literal = false, false
	}
	flushSeg := func() {
		flushTok()
		pending = redirNone
		if len(seg.words)+len(seg.inputs)+len(seg.others) > 0 {
			segs = append(segs, seg)
			seg = segment{}
		}
	}
	peek := func(i int) byte {
		if i < len(cmd) {
			return cmd[i]
		}
		return 0
	}

	// 바이트 단위로 훑는다. 구분자는 모두 ASCII 이고 UTF-8 의 다바이트 문자는
	// ASCII 바이트와 겹치지 않으므로 한글 인자도 안전하다.
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if quote == '\'' {
			// 작은따옴표 안에서는 백슬래시도 글자 그대로다. ' 를 이스케이프할 방법이 없다.
			if c == quote {
				quote = 0
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		if quote == '"' {
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && peek(i+1) == '\n':
				i++ // 줄 이음: 백슬래시와 줄바꿈 둘 다 사라진다
			case c == '\\' && i+1 < len(cmd) && strings.IndexByte("\"\\$`", cmd[i+1]) >= 0:
				// 큰따옴표 안에서는 " \ $ ` 만 이스케이프된다.
				cur.WriteByte(cmd[i+1])
				i++
			default:
				cur.WriteByte(c)
			}
			continue
		}
		switch {
		case c == '\\':
			// 따옴표 밖의 백슬래시는 다음 바이트를 글자 그대로 만든다.
			// 맨 끝에 홀로 남은 백슬래시는 그대로 둔다.
			switch {
			case i+1 >= len(cmd):
				cur.WriteByte(c)
			case cmd[i+1] == '\n':
				i++ // 줄 이음
				continue
			default:
				cur.WriteByte(cmd[i+1])
				i++
			}
			inTok, literal = true, true
		case c == '\'' || c == '"':
			quote = c
			inTok, literal = true, true
		case c == ' ' || c == '\t':
			flushTok()
		case c == '\n' || c == ';' || c == '|':
			flushSeg()
		case c == '<' || c == '>' || (c == '&' && peek(i+1) == '>'):
			// 2> 의 2 처럼 연산자에 붙은 숫자만으로 된 토큰은 fd 번호다.
			if inTok && !literal && isDigits(cur.String()) {
				cur.Reset()
				inTok = false
			} else {
				flushTok()
			}
			op := string(c)
			switch c {
			case '&': // &>, &>>
				op += ">"
				i++
				if peek(i+1) == '>' {
					op += ">"
					i++
				}
			case '>': // >>, >&, >|
				if n := peek(i + 1); n == '>' || n == '&' || n == '|' {
					op += string(n)
					i++
				}
			case '<': // <<, <<-, <<<, <&, <>
				switch n := peek(i + 1); n {
				case '<':
					op += "<"
					i++
					if m := peek(i + 1); m == '<' || m == '-' {
						op += string(m)
						i++
					}
				case '&', '>':
					op += string(n)
					i++
				}
			}
			if op == "<" || op == "<>" {
				pending = redirInput
			} else {
				pending = redirOther
			}
		case c == '&':
			flushSeg()
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	flushSeg()
	return segs
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isDotenvxToken 은 토큰이 dotenvx 실행 파일을 가리키는지 본다.
// dotenvx, /opt/homebrew/bin/dotenvx, @dotenvx/dotenvx, dotenvx@latest,
// @dotenvx/dotenvx@1.2.3 을 모두 dotenvx 로 본다.
func isDotenvxToken(tok string) bool {
	base := filepath.Base(tok)
	if at := strings.Index(base, "@"); at > 0 {
		base = base[:at]
	}
	return base == "dotenvx"
}

// runChild 는 dotenvx run 의 인자에서 -- 뒤의 자식 명령을 꺼낸다.
func runChild(args []string) []string {
	for i, tok := range args {
		if tok == "--" {
			return args[i+1:]
		}
	}
	return nil
}

// isEnvDump 는 명령이 환경변수를 출력하는지 본다.
//
// dotenvx run 은 기본으로 복호화한 평문 값을 그대로 주입한다(--redact,
// --mask 는 선택). 따라서 dotenvx run -- printenv 는 볼트 전체를 평문으로
// 출력한다. printenv 는 인자가 있어도(printenv NAME) 값을 출력한다.
// env 는 뒤에 실행할 명령이 오면 출력하지 않으므로 옵션·VAR=값 만 있을 때만
// 덤프로 본다. set, export, declare 는 위치 인자가 없을 때만 덤프로 본다.
func isEnvDump(child []string) bool {
	if len(child) == 0 {
		return false
	}
	rest := child[1:]
	switch filepath.Base(child[0]) {
	case "printenv":
		return true
	case "env":
		for _, tok := range rest {
			if !strings.HasPrefix(tok, "-") && !strings.Contains(tok, "=") {
				return false
			}
		}
		return true
	case "set":
		return len(rest) == 0
	case "export", "declare":
		for _, tok := range rest {
			if !strings.HasPrefix(tok, "-") {
				return false
			}
		}
		return true
	}
	return false
}

// AnalyzeCommand 는 Bash 명령 문자열을 훑어 위험 패턴과 읽으려는 경로를 찾는다.
//
// 문자열 매칭이므로 d=dotenvx; $d get X 같은 우회는 잡지 못한다.
// 이 도구의 목표는 우발적 노출 차단이며 의도적 탈취 차단이 아니다.
func AnalyzeCommand(cmd string) CmdFinding {
	f := CmdFinding{Risk: CmdSafe}

	for _, seg := range splitSegments(cmd) {
		for _, list := range [][]string{seg.words, seg.inputs, seg.others} {
			for _, tok := range list {
				if strings.Contains(tok, "DOTENV_PRIVATE_KEY") {
					f.Risk = CmdPrivateKeyEcho
				}
			}
		}

		fields := seg.words

		// npx, bunx, pnpm exec, sudo, env VAR=1 같은 래퍼 뒤에 와도 잡도록
		// dotenvx 토큰을 세그먼트 어디서든 찾는다.
		for i, tok := range fields {
			if !isDotenvxToken(tok) || i+1 >= len(fields) {
				continue
			}
			switch fields[i+1] {
			case "get":
				f.Risk = CmdDotenvxGet
			case "decrypt":
				f.Risk = CmdDotenvxDecrypt
			case "keypair":
				// dotenvx keypair 는 DOTENV_PRIVATE_KEY 를 그대로 출력한다.
				f.Risk = CmdDotenvxKeypair
			case "run":
				if isEnvDump(runChild(fields[i+2:])) {
					f.Risk = CmdEnvDump
				}
			}
		}

		// 입력 리다이렉션(<)은 어떤 명령이든 파일 내용을 끌어온다.
		// 출력 리다이렉션 대상은 읽기가 아니므로 보지 않는다.
		redirected := false
		for _, tok := range seg.inputs {
			if k := ClassifyPath(tok); k == PathEnvKeys || k == PathEnvFile {
				f.Paths = append(f.Paths, tok)
				redirected = true
			}
		}

		if len(fields) == 0 || !readerCommands[filepath.Base(fields[0])] || redirected {
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
