package main

import (
	"fmt"
	"io"
	"os"

	"github.com/liger82/AgentEnvGuard/internal/cmd"
	"github.com/liger82/AgentEnvGuard/internal/policy"
)

const usage = `aeg — 코딩 에이전트가 시크릿 평문을 읽지 못하게 막는다.

사용법:
  aeg install        Claude Code 전역 훅을 설치한다 (노트북당 한 번)
  aeg scan [경로]     평문 .env 를 쓰는 프로젝트를 찾는다 (기본: $HOME)
    --depth N        탐색 깊이를 조정한다 (기본: 6)
  aeg init [경로]     프로젝트 하나를 dotenvx 로 마이그레이션한다
  aeg hook           훅이 내부적으로 호출한다. 직접 쓰지 않는다

시크릿 저장·암호화·주입은 dotenvx 가 한다. aeg 는 그 위의 가드레일이다.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run 은 서브커맨드를 실행하고 종료코드를 반환한다.
//
// 사용법 오류도 1로 끝낸다. 2는 Claude Code 훅에서 "차단" 신호이므로,
// 훅이 인자 없이 잘못 등록됐을 때 모든 도구 호출이 막히는 일을 피한다.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	switch args[0] {
	case "hook":
		return cmd.Hook(stdin, stdout, stderr, policy.New())
	case "install":
		bin, err := os.Executable()
		if err != nil {
			fmt.Fprintf(stderr, "aeg: 실행 파일 경로를 찾을 수 없습니다: %v\n", err)
			return 1
		}
		if err := cmd.Install(cmd.SettingsPath(), bin, stdout); err != nil {
			fmt.Fprintf(stderr, "aeg: %v\n", err)
			return 1
		}
	case "scan":
		root, depth, err := cmd.ParseScanArgs(args[1:])
		if err != nil {
			fmt.Fprintf(stderr, "aeg: %v\n", err)
			return 1
		}
		if err := cmd.RunScan(root, depth, stdout); err != nil {
			fmt.Fprintf(stderr, "aeg: %v\n", err)
			return 1
		}
	case "init":
		dir := "."
		if len(args) > 1 {
			dir = args[1]
		}
		if err := cmd.Init(dir, cmd.ExecRunner, stdout); err != nil {
			fmt.Fprintf(stderr, "aeg: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "aeg: 알 수 없는 명령 %q\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return 1
	}
	return 0
}
