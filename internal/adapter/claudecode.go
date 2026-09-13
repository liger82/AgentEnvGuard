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

func (ClaudeCode) Emit(w io.Writer, d policy.Decision) error {
	decision := "deny"
	if d.Allow {
		decision = "allow"
	}
	return json.NewEncoder(w).Encode(preToolUseOutput{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: d.Reason,
		},
	})
}
