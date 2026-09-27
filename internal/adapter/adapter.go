// Package adapter 는 에이전트별 훅 규약을 판정 엔진과 분리한다.
// 새 에이전트를 지원하려면 이 인터페이스 구현을 하나 더 만든다.
package adapter

import (
	"io"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

type Adapter interface {
	Parse(r io.Reader) (policy.ToolCall, error)
	Emit(w io.Writer, d policy.Decision) error
}
