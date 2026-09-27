// Package cmd 는 aeg 의 서브커맨드 구현을 담는다.
package cmd

import (
	"bytes"
	"fmt"
	"io"

	"github.com/liger82/AgentEnvGuard/internal/adapter"
	"github.com/liger82/AgentEnvGuard/internal/policy"
)

// Hook 은 PreToolUse 훅 본체다.
//
// 항상 0을 반환한다. 종료코드를 판정 신호로 쓰면 의도적 차단과 버그로 인한
// 비정상 종료가 구분되지 않고, 판정기가 죽었을 때 에이전트의 모든 툴 호출이
// 막힌다. 차단 판정만 stdout JSON 본문에 담는다.
//
// 허용·파싱 실패·알 수 없는 도구·패닉은 모두 stdout 에 아무것도 쓰지 않는다
// ("판정 없음"). 그러면 Claude Code 의 평소 권한 흐름이 그대로 적용된다.
// permissionDecision: "allow" 를 내면 권한 프롬프트가 사라지므로 절대 내지 않는다.
// 예외로, 파싱에 실패해도 원본에 .env.keys 가 있으면 차단한다.
//
// recover 는 Parse, Decide, Emit 전체를 감싼다 — 최상위에서 한 번만 잡는다.
// Emit 자체가 패닉해도(예: stdout 이 깨진 파이프) 그 경계 밖으로 전파되어
// 프로세스를 비정상 종료시키는 일이 없어야 하기 때문이다. recover 뒤에는
// 아무것도 더 쓰지 않는다 — 출력 없음이 곧 통과다.
func Hook(stdin io.Reader, stdout, stderr io.Writer, e *policy.Engine) int {
	a := adapter.ClaudeCode{}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "aeg: 내부 오류로 통과시킴: %v\n", r)
		}
	}()

	// 파싱 실패 시 원본을 다시 볼 수 있도록 먼저 전부 읽는다.
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "aeg: stdin 읽기 실패: %v\n", err)
	}

	var dec policy.Decision
	tc, err := a.Parse(bytes.NewReader(raw))
	if err != nil {
		dec = policy.DecideUnparsed(raw)
		if dec.Allow {
			fmt.Fprintf(stderr, "aeg: stdin 파싱 실패로 통과시킴: %v\n", err)
		}
	} else {
		dec = e.Decide(tc)
	}

	if err := a.Emit(stdout, dec); err != nil {
		fmt.Fprintf(stderr, "aeg: 출력 실패: %v\n", err)
	}
	return 0
}
