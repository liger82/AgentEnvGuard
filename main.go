package main

import (
	"fmt"
	"os"

	"github.com/liger82/AgentEnvGuard/internal/cmd"
	"github.com/liger82/AgentEnvGuard/internal/policy"
)

const usage = `aeg — 코딩 에이전트가 시크릿 평문을 읽지 못하게 막는다.

사용법:
  aeg install        Claude Code 전역 훅을 설치한다 (노트북당 한 번)
  aeg scan [경로]     평문 .env 를 쓰는 프로젝트를 찾는다 (기본: $HOME)
  aeg init [경로]     프로젝트 하나를 dotenvx 로 마이그레이션한다
  aeg hook           훅이 내부적으로 호출한다. 직접 쓰지 않는다

시크릿 저장·주입·마스킹은 dotenvx 가 한다. aeg 는 그 위의 가드레일이다.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hook":
		os.Exit(cmd.Hook(os.Stdin, os.Stdout, os.Stderr, policy.New()))
	case "install":
		bin, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "aeg: 실행 파일 경로를 찾을 수 없습니다: %v\n", err)
			os.Exit(1)
		}
		if err := cmd.Install(cmd.SettingsPath(), bin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aeg: %v\n", err)
			os.Exit(1)
		}
	case "scan":
		root := ""
		if len(os.Args) > 2 {
			root = os.Args[2]
		}
		if err := cmd.RunScan(root, cmd.DefaultDepth, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aeg: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "aeg: 알 수 없는 명령 %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}
