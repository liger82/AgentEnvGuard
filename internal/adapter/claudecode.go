package adapter

import (
	"encoding/json"
	"io"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

// ClaudeCode 는 Claude Code 의 PreToolUse 훅 규약을 다룬다.
type ClaudeCode struct{}

type preToolUseInput struct {
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command  string `json:"command"`   // Bash
		FilePath string `json:"file_path"` // Read
		Pattern  string `json:"pattern"`   // Grep
		Path     string `json:"path"`      // Grep
		Glob     string `json:"glob"`      // Grep
	} `json:"tool_input"`
}

func (ClaudeCode) Parse(r io.Reader) (policy.ToolCall, error) {
	var in preToolUseInput
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return policy.ToolCall{}, err
	}
	tc := policy.ToolCall{Cwd: in.Cwd}
	switch in.ToolName {
	case "Bash":
		tc.Kind = policy.ToolBash
		tc.Command = in.ToolInput.Command
	case "Read":
		tc.Kind = policy.ToolFileRead
		tc.Path = in.ToolInput.FilePath
	case "Grep":
		tc.Kind = policy.ToolContentSearch
		tc.Path = in.ToolInput.Path
		tc.Pattern = in.ToolInput.Pattern
		tc.Glob = in.ToolInput.Glob
	default:
		tc.Kind = policy.ToolUnknown
	}
	return tc, nil
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

type preToolUseOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

// Emit 은 차단 판정만 JSON 으로 내보낸다. 허용이면 아무것도 쓰지 않는다.
//
// permissionDecision: "allow" 는 "판정 없음"이 아니라 Claude Code 의 권한
// 확인 프롬프트를 건너뛰는 적극적 승인이다. 이것을 내보내면 aeg 설치만으로
// rm -rf 같은 모든 Bash 호출이 자동 승인된다. 출력이 없으면 평소의 권한
// 흐름이 그대로 적용된다.
func (ClaudeCode) Emit(w io.Writer, d policy.Decision) error {
	if d.Allow {
		return nil
	}
	return json.NewEncoder(w).Encode(preToolUseOutput{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: d.Reason,
		},
	})
}
