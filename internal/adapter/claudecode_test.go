package adapter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

func TestParseBash(t *testing.T) {
	in := `{
	  "session_id": "s1",
	  "cwd": "/Users/me/proj",
	  "hook_event_name": "PreToolUse",
	  "tool_name": "Bash",
	  "tool_input": {"command": "cat .env.keys"},
	  "tool_use_id": "toolu_01"
	}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolBash {
		t.Errorf("Kind = %v, want ToolBash", got.Kind)
	}
	if got.Command != "cat .env.keys" {
		t.Errorf("Command = %q", got.Command)
	}
	if got.Cwd != "/Users/me/proj" {
		t.Errorf("Cwd = %q", got.Cwd)
	}
}

func TestParseRead(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Read","tool_input":{"file_path":"/p/.env"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolFileRead || got.Path != "/p/.env" {
		t.Errorf("got %+v", got)
	}
}

func TestParseGrep(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Grep","tool_input":{"pattern":"DOTENV_PRIVATE_KEY","path":".env.keys"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolContentSearch {
		t.Errorf("Kind = %v, want ToolContentSearch", got.Kind)
	}
	if got.Path != ".env.keys" || got.Pattern != "DOTENV_PRIVATE_KEY" {
		t.Errorf("got %+v", got)
	}
}

func TestParseGrepGlob(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Grep","tool_input":{"pattern":"KEY","glob":".env.keys"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Glob != ".env.keys" {
		t.Errorf("Glob = %q, want .env.keys", got.Glob)
	}
}

func TestParseUnknownToolIsUnknownKind(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"WebFetch","tool_input":{"url":"x"}}`
	got, err := ClaudeCode{}.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != policy.ToolUnknown {
		t.Errorf("Kind = %v, want ToolUnknown", got.Kind)
	}
}

func TestParseMalformedReturnsError(t *testing.T) {
	if _, err := (ClaudeCode{}).Parse(strings.NewReader("{not json")); err == nil {
		t.Error("깨진 JSON 인데 에러가 없다")
	}
}

func TestEmitDeny(t *testing.T) {
	var buf bytes.Buffer
	err := ClaudeCode{}.Emit(&buf, policy.Decision{Allow: false, Reason: "안 됩니다"})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q", out.HookSpecificOutput.HookEventName)
	}
	if out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q", out.HookSpecificOutput.PermissionDecision)
	}
	if out.HookSpecificOutput.PermissionDecisionReason != "안 됩니다" {
		t.Errorf("reason = %q", out.HookSpecificOutput.PermissionDecisionReason)
	}
}

// TestEmitAllowWritesNothing 은 허용 판정에서 아무것도 출력하지 않는지 본다.
// permissionDecision: "allow" 는 Claude Code 의 권한 확인 프롬프트를 건너뛰게
// 하므로, 내보내면 aeg 설치만으로 모든 Bash 호출이 자동 승인된다.
func TestEmitAllowWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	if err := (ClaudeCode{}).Emit(&buf, policy.Decision{Allow: true}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("허용인데 출력이 있다: %s", buf.String())
	}
}
