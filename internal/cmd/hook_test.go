package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

func testEngine(kinds map[string]policy.EnvFileKind) *policy.Engine {
	return &policy.Engine{
		ClassifyFile: func(p string) policy.EnvFileKind {
			if k, ok := kinds[p]; ok {
				return k
			}
			return policy.EnvEmpty
		},
	}
}

func TestHookDeniesEnvKeys(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}`
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader(in), &out, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0 (판정은 본문에 담는다)", code)
	}
	if !strings.Contains(out.String(), `"deny"`) {
		t.Errorf("차단되지 않았다: %s", out.String())
	}
}

func TestHookAllowsNormalCommand(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`
	var out, errOut bytes.Buffer
	if code := Hook(strings.NewReader(in), &out, &errOut, testEngine(nil)); code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("허용되지 않았다: %s", out.String())
	}
}

func TestHookFailsOpenOnMalformedInput(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader("{not json"), &out, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("파싱 실패 시 통과시켜야 한다: %s", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("stderr 에 경고를 남겨야 한다")
	}
}

func TestHookFailsOpenOnPanic(t *testing.T) {
	e := &policy.Engine{
		ClassifyFile: func(string) policy.EnvFileKind { panic("의도적 패닉") },
	}
	in := `{"cwd":"/p","tool_name":"Read","tool_input":{"file_path":"/p/.env"}}`
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader(in), &out, &errOut, e)
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Errorf("패닉 시 통과시켜야 한다: %s", out.String())
	}
}

func BenchmarkHook(b *testing.B) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}`
	e := testEngine(nil)
	for i := 0; i < b.N; i++ {
		var out, errOut bytes.Buffer
		Hook(strings.NewReader(in), &out, &errOut, e)
	}
}
