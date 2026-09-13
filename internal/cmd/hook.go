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
//
// recover 는 Parse, Decide, Emit 전체를 감싼다 — 최상위에서 한 번만 잡는다.
// Emit 자체가 패닉해도(오늘은 없지만, 구조적으로 보장되지 않는다) 그 경계
// 밖으로 전파되어 프로세스를 비정상 종료시키는 일이 없어야 하기 때문이다.
// 패닉이 Emit 이전에 났다면 아직 아무것도 출력되지 않았으므로, recover 직후
// allow 판정을 한 번 더 내보낸다 — 이 재시도 역시 패닉할 수 있어 별도로
// 감싼다.
func Hook(stdin io.Reader, stdout, stderr io.Writer, e *policy.Engine) int {
	a := adapter.ClaudeCode{}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "aeg: 내부 오류로 통과시킴: %v\n", r)
			emitAllowSafely(a, stdout, stderr)
		}
	}()

	dec := policy.Decision{Allow: true}
	tc, err := a.Parse(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "aeg: stdin 파싱 실패로 통과시킴: %v\n", err)
	} else {
		dec = e.Decide(tc)
	}

	if err := a.Emit(stdout, dec); err != nil {
		fmt.Fprintf(stderr, "aeg: 출력 실패: %v\n", err)
	}
	return 0
}

// emitAllowSafely 는 recover 직후 allow 판정을 내보낸다. Emit 자체가 다시
// 패닉하는 경우(예: stdout 이 깨진 파이프)까지 대비해 한 번 더 감싼다 —
// 이 시도가 실패해도 Hook 은 여전히 0을 반환해야 한다.
func emitAllowSafely(a adapter.ClaudeCode, stdout, stderr io.Writer) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "aeg: 출력 중에도 실패, 포기: %v\n", r)
		}
	}()
	if err := a.Emit(stdout, policy.Decision{Allow: true}); err != nil {
		fmt.Fprintf(stderr, "aeg: 출력 실패: %v\n", err)
	}
}
