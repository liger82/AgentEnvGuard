package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitignoreHas(t *testing.T) {
	content := "node_modules/\n.env\n# 주석\n  .env.keys  \n"
	if !GitignoreHas(content, ".env.keys") {
		t.Error(".env.keys 를 못 찾았다 (공백 포함 줄)")
	}
	if !GitignoreHas(content, ".env") {
		t.Error(".env 를 못 찾았다")
	}
	if GitignoreHas(content, ".env.local") {
		t.Error("없는 항목을 있다고 했다")
	}
}

func TestEnsureGitignoreCreatesFile(t *testing.T) {
	dir := t.TempDir()
	added, err := EnsureGitignore(dir, ".env.keys")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Error("added = false, want true")
	}
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), ".env.keys") {
		t.Errorf(".gitignore 내용: %s", b)
	}
}

func TestEnsureGitignoreIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureGitignore(dir, ".env.keys"); err != nil {
		t.Fatal(err)
	}
	added, err := EnsureGitignore(dir, ".env.keys")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("두 번째 호출에서 added = true. 중복 추가된다")
	}
}

// fakeRunner 는 외부 명령을 흉내낸다.
func fakeRunner(t *testing.T, dotenvxFound bool, gitLogOutput string, calls *[]string) Runner {
	t.Helper()
	return func(dir, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		switch {
		case name == "dotenvx" && len(args) > 0 && args[0] == "--version":
			if !dotenvxFound {
				return nil, fmt.Errorf("실행 파일 없음")
			}
			return []byte("1.0.0"), nil
		case name == "dotenvx" && len(args) > 0 && args[0] == "encrypt":
			return []byte("encrypted"), nil
		case name == "git":
			return []byte(gitLogOutput), nil
		}
		return nil, nil
	}
}

func TestInitFailsWithoutDotenvx(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	err := Init(dir, fakeRunner(t, false, "", &calls), &out)
	if err == nil {
		t.Fatal("dotenvx 가 없는데 성공했다")
	}
	if !strings.Contains(err.Error(), "dotenvx") {
		t.Errorf("설치 안내가 없다: %v", err)
	}
}

func TestInitEncryptsAndAddsGitignore(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "dotenvx encrypt") {
		t.Errorf("encrypt 를 부르지 않았다: %v", calls)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || !strings.Contains(string(b), ".env.keys") {
		t.Errorf(".env.keys 가 gitignore 에 없다: %s / %v", b, err)
	}
}

func TestInitWarnsOnGitHistory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	// git log 가 커밋을 반환하면 과거에 평문이 커밋된 것이다.
	if err := Init(dir, fakeRunner(t, true, "abc123 초기 커밋\n", &calls), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "로테이션") {
		t.Errorf("히스토리 경고와 로테이션 권고가 없다:\n%s", out.String())
	}
}

func TestInitNoWarnWhenHistoryClean(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "로테이션") {
		t.Errorf("히스토리가 깨끗한데 경고했다:\n%s", out.String())
	}
}

func TestInitSkipsAlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"),
		"DOTENV_PUBLIC_KEY=\"03\"\nSECRET=\"encrypted:xx\"\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(calls, "\n"), "encrypt") {
		t.Error("이미 암호화된 파일을 다시 암호화했다")
	}
}

func TestInitAdvisesButDoesNotRemoveEnvFromGitignore(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".env"), "SECRET=abc\n")
	write(t, filepath.Join(dir, ".gitignore"), ".env\n")
	var calls []string
	var out bytes.Buffer
	if err := Init(dir, fakeRunner(t, true, "", &calls), &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !GitignoreHas(string(b), ".env") {
		t.Error(".gitignore 에서 .env 를 자동 제거했다. 안내만 해야 한다")
	}
	if !strings.Contains(out.String(), ".gitignore") {
		t.Errorf("안내 문구가 없다:\n%s", out.String())
	}
}
