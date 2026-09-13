package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func paths(fs []Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestScanFindsPlaintextOnly(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "a", ".env")
	enc := filepath.Join(root, "b", ".env")
	example := filepath.Join(root, "c", ".env.example")
	write(t, plain, "SECRET=abc123\n")
	write(t, enc, "DOTENV_PUBLIC_KEY=\"03\"\nSECRET=\"encrypted:xx\"\n")
	write(t, example, "SECRET=\n")

	found, _ := Scan(root, DefaultDepth)
	got := paths(found)
	if !contains(got, plain) {
		t.Errorf("평문 .env 를 못 찾았다: %v", got)
	}
	if contains(got, enc) {
		t.Errorf("암호문 .env 를 잘못 보고했다: %v", got)
	}
	if contains(got, example) {
		t.Errorf(".env.example 을 잘못 보고했다: %v", got)
	}
}

func TestScanSkipsExcludedDirs(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, "node_modules", "pkg", ".env")
	write(t, hidden, "SECRET=abc\n")
	found, _ := Scan(root, DefaultDepth)
	if contains(paths(found), hidden) {
		t.Errorf("node_modules 를 훑었다: %v", paths(found))
	}
}

func TestScanRespectsDepthLimit(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", ".env")
	write(t, deep, "SECRET=abc\n")

	if found, _ := Scan(root, 2); contains(paths(found), deep) {
		t.Error("깊이 제한을 넘어 훑었다")
	}
	if found, _ := Scan(root, 10); !contains(paths(found), deep) {
		t.Error("깊이 제한 안인데 못 찾았다")
	}
}

func TestScanFindsEnvKeys(t *testing.T) {
	// .env.keys 는 정의상 평문 개인키다. 커밋 위험이 있으므로 보고한다.
	root := t.TempDir()
	keys := filepath.Join(root, "p", ".env.keys")
	write(t, keys, "DOTENV_PRIVATE_KEY=\"122\"\n")
	if found, _ := Scan(root, DefaultDepth); !contains(paths(found), keys) {
		t.Errorf(".env.keys 를 보고하지 않았다: %v", paths(found))
	}
}
