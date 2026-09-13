// Package cmd 는 aeg 의 서브커맨드 구현을 담는다.
package cmd

import (
	"fmt"
	"io"

	"github.com/liger82/AgentEnvGuard/internal/adapter"
	"github.com/liger82/AgentEnvGuard/internal/policy"
)

// Hook 은 PreToolUse 훅 본체다.
//
// 항상 0을 반환한다. 종료코드를 판정 신호로 쓰면 의도적 차단과 버그로 인한
// 비정상 종료가 구분되지 않고, 판정기가 죽었을 때 에이전트의 모든 툴 호출이
// 막힌다. 판정은 stdout JSON 본문에만 담는다.
func Hook(stdin io.Reader, stdout, stderr io.Writer, e *policy.Engine) int {
	a := adapter.ClaudeCode{}
	dec := policy.Decision{Allow: true}

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(stderr, "aeg: 내부 오류로 통과시킴: %v\n", r)
				dec = policy.Decision{Allow: true}
			}
		}()
		tc, err := a.Parse(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "aeg: stdin 파싱 실패로 통과시킴: %v\n", err)
			return
		}
		dec = e.Decide(tc)
	}()

	if err := a.Emit(stdout, dec); err != nil {
		fmt.Fprintf(stderr, "aeg: 출력 실패: %v\n", err)
	}
	return 0
}
