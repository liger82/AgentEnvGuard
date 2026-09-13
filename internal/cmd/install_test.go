package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func hookCommands(t *testing.T, raw []byte) []string {
	t.Helper()
	var s struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("결과가 JSON 이 아니다: %v\n%s", err, raw)
	}
	var cmds []string
	for _, entry := range s.Hooks.PreToolUse {
		for _, h := range entry.Hooks {
			cmds = append(cmds, h.Command)
		}
	}
	return cmds
}

// hookEntry 는 raw settings.json 에서 aeg 훅이 들어있는 PreToolUse 엔트리를 찾는다.
// args 와 matcher 검증에 쓴다.
func hookEntry(t *testing.T, raw []byte, binPath string) (matcher string, args []string) {
	t.Helper()
	var s struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("결과가 JSON 이 아니다: %v\n%s", err, raw)
	}
	for _, entry := range s.Hooks.PreToolUse {
		for _, h := range entry.Hooks {
			if h.Command == binPath {
				return entry.Matcher, h.Args
			}
		}
	}
	t.Fatalf("binPath %q 에 대한 훅 엔트리를 찾지 못했다: %s", binPath, raw)
	return "", nil
}

func TestMergeHookIntoEmptySettings(t *testing.T) {
	out, changed, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}
	cmds := hookCommands(t, out)
	if len(cmds) != 1 || cmds[0] != "/usr/local/bin/aeg" {
		t.Errorf("commands = %v", cmds)
	}
}

// TestMergeHookUsesExecFormNotShell 은 등록된 훅이 exec 형태
// ("command": 바이너리 경로, "args": ["hook"]) 인지 확인한다.
// 셸 형태("command": "aeg hook")로 등록되면 매 도구 호출마다 셸 기동
// 비용이 들어 10ms 예산을 못 지킨다.
func TestMergeHookUsesExecFormNotShell(t *testing.T) {
	out, _, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	_, args := hookEntry(t, out, "/usr/local/bin/aeg")
	if len(args) != 1 || args[0] != "hook" {
		t.Errorf("args = %v, want [\"hook\"]", args)
	}
}

// TestMergeHookMatcherCoversGrep 은 matcher 에서 Grep 이 빠지지 않았는지
// 확인한다. Grep 은 매칭된 줄의 "내용"을 반환하므로, matcher 에서 Grep 이
// 빠지면 Bash/Read 를 거치지 않고 Grep(pattern="DOTENV_PRIVATE_KEY") 만으로
// 비밀키가 그대로 새어나간다 — 정책 전체가 무의미해진다.
func TestMergeHookMatcherCoversGrep(t *testing.T) {
	out, _, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	matcher, _ := hookEntry(t, out, "/usr/local/bin/aeg")
	if matcher != "Bash|Read|Grep" {
		t.Errorf("matcher = %q, want %q", matcher, "Bash|Read|Grep")
	}
}

func TestMergeHookIsIdempotent(t *testing.T) {
	once, _, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	twice, changed, err := MergeHook(once, "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("두 번째 실행에서 changed = true. 멱등이 아니다")
	}
	if cmds := hookCommands(t, twice); len(cmds) != 1 {
		t.Errorf("훅이 중복됐다: %v", cmds)
	}
}

func TestMergeHookPreservesExistingSettings(t *testing.T) {
	in := []byte(`{
	  "model": "opus",
	  "hooks": {
	    "PreToolUse": [
	      {"matcher": "Write", "hooks": [{"type": "command", "command": "/other/hook.sh"}]}
	    ],
	    "Stop": [
	      {"matcher": "*", "hooks": [{"type": "command", "command": "/stop.sh"}]}
	    ]
	  }
	}`)
	out, changed, err := MergeHook(in, "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("changed = false, want true")
	}

	var s map[string]any
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatal(err)
	}
	if s["model"] != "opus" {
		t.Errorf("기존 최상위 키가 사라졌다: %v", s)
	}
	hooks := s["hooks"].(map[string]any)
	if _, ok := hooks["Stop"]; !ok {
		t.Error("다른 이벤트의 훅이 사라졌다")
	}
	cmds := hookCommands(t, out)
	if len(cmds) != 2 {
		t.Errorf("기존 PreToolUse 훅과 공존해야 한다: %v", cmds)
	}
}

func TestMergeHookRejectsInvalidJSON(t *testing.T) {
	if _, _, err := MergeHook([]byte("{not json"), "/usr/local/bin/aeg"); err == nil {
		t.Error("깨진 JSON 을 덮어쓰면 안 된다. 에러를 내야 한다")
	}
}

func TestInstallWritesBackupAndFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte(`{"model":"opus"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if cmds := hookCommands(t, mustRead(t, p)); len(cmds) != 1 {
		t.Errorf("설치되지 않았다: %v", cmds)
	}
	if _, err := os.Stat(p + ".aeg-backup"); err != nil {
		t.Error("백업 파일이 없다")
	}
}

func TestInstallCreatesMissingSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "settings.json")
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if cmds := hookCommands(t, mustRead(t, p)); len(cmds) != 1 {
		t.Errorf("생성되지 않았다: %v", cmds)
	}
}

// 백업은 설치 전 최초 상태를 보존해야 한다. 두 번째 설치가 이미 aeg 가
// 들어간 파일로 백업을 덮어쓰면 되돌릴 원본이 사라진다.
func TestInstallDoesNotOverwriteExistingBackup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	original := `{"model":"opus"}`
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	// 경로가 달라 실제로 변경이 일어나는 두 번째 설치
	if err := Install(p, "/opt/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, p+".aeg-backup")); got != original {
		t.Errorf("백업이 덮어써졌다:\n%s", got)
	}
}

func TestInstallNoChangeDoesNotWriteBackup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	once, _, err := MergeHook([]byte(`{}`), "/usr/local/bin/aeg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, once, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".aeg-backup"); !os.IsNotExist(err) {
		t.Errorf("바뀐 것이 없는데 백업을 만들었다: %v", err)
	}
}

func TestInstallInvalidJSONLeavesFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".aeg-backup", []byte(`{"model":"opus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err == nil {
		t.Fatal("깨진 JSON 인데 성공했다")
	}
	if got := string(mustRead(t, p+".aeg-backup")); got != `{"model":"opus"}` {
		t.Errorf("깨진 JSON 으로 백업을 덮어썼다: %s", got)
	}
	if got := string(mustRead(t, p)); got != "{not json" {
		t.Errorf("settings.json 을 덮어썼다: %s", got)
	}
}

func TestInstallNullSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte("null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	if cmds := hookCommands(t, mustRead(t, p)); len(cmds) != 1 {
		t.Errorf("null 파일에 설치되지 않았다: %v", cmds)
	}
}

func TestMergeHookRejectsNonObjectTopLevel(t *testing.T) {
	for _, in := range []string{`[]`, `"x"`, `42`} {
		if _, _, err := MergeHook([]byte(in), "/usr/local/bin/aeg"); err == nil {
			t.Errorf("최상위가 객체가 아닌 %s 에 에러가 없다", in)
		}
	}
}

func TestInstallPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Install(p, "/usr/local/bin/aeg", &out); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("파일 모드 = %v, want 0600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "settings.json" && e.Name() != "settings.json.aeg-backup" {
			t.Errorf("임시 파일이 남았다: %s", e.Name())
		}
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
