package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

// errWriter 는 Write 가 항상 에러를 반환하는 스텁이다 — Emit 실패 경로를 본다.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("쓰기 실패")
}

// panicWriter 는 Write 가 패닉하는 스텁이다 — recover 경계가 Emit 까지
// 덮는지 본다. Emit 오늘 구현은 패닉하지 않지만, 그것은 구현의 우연이지
// 구조적 보장이 아니므로 recover 경계 자체를 검증한다.
type panicWriter struct{}

func (panicWriter) Write(p []byte) (int, error) {
	panic("emit 중 의도적 패닉")
}

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
	if out.Len() != 0 {
		t.Errorf("허용인데 출력이 있다 (allow 를 내면 권한 프롬프트가 사라진다): %s", out.String())
	}
}

func TestHookFailsOpenOnMalformedInput(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Hook(strings.NewReader("{not json"), &out, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if out.Len() != 0 {
		t.Errorf("파싱 실패 시 판정 없이 통과시켜야 한다: %s", out.String())
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
	if out.Len() != 0 {
		t.Errorf("패닉 시 판정 없이 통과시켜야 한다: %s", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("stderr 에 경고를 남겨야 한다")
	}
}

func TestHookFailsOpenOnEmitError(t *testing.T) {
	// 허용은 아무것도 쓰지 않으므로 쓰기 실패를 보려면 차단 입력이 필요하다.
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}`
	var errOut bytes.Buffer
	code := Hook(strings.NewReader(in), errWriter{}, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0", code)
	}
	if errOut.Len() == 0 {
		t.Error("stderr 에 경고를 남겨야 한다")
	}
}

func TestHookFailsOpenOnEmitPanic(t *testing.T) {
	in := `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"cat .env.keys"}}`
	var errOut bytes.Buffer
	code := Hook(strings.NewReader(in), panicWriter{}, &errOut, testEngine(nil))
	if code != 0 {
		t.Errorf("종료코드 = %d, want 0 (Emit 패닉도 최상위 recover 로 잡혀야 한다)", code)
	}
	if errOut.Len() == 0 {
		t.Error("stderr 에 경고를 남겨야 한다")
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
