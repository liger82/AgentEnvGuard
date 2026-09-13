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

func TestParseDepthFlag(t *testing.T) {
	tests := []struct {
		args      []string
		wantRoot  string
		wantDepth int
		wantErr   bool
	}{
		// 유효한 경우
		{[]string{"--depth", "3", "/home"}, "/home", 3, false},
		{[]string{"/home", "--depth", "3"}, "/home", 3, false},
		{[]string{"--depth=3", "/home"}, "/home", 3, false},
		{[]string{"/home", "--depth=3"}, "/home", 3, false},
		{[]string{"--depth", "5"}, "", 5, false},
		{[]string{"--depth=7"}, "", 7, false},
		{[]string{"/home"}, "/home", DefaultDepth, false},
		{[]string{}, "", DefaultDepth, false},

		// 에러 경우
		{[]string{"--depth"}, "", 0, true},        // 값 없음
		{[]string{"--depth", "abc"}, "", 0, true}, // 숫자 아님
		{[]string{"--depth=abc"}, "", 0, true},    // 숫자 아님
		{[]string{"--depth", "0"}, "", 0, true},   // 0은 불가
		{[]string{"--depth", "-1"}, "", 0, true},  // 음수는 불가
		{[]string{"--unknown"}, "", 0, true},      // 알 수 없는 플래그
	}

	for i, tt := range tests {
		root, depth, err := ParseScanArgs(tt.args)
		if (err != nil) != tt.wantErr {
			t.Errorf("test %d: wantErr=%v, got err=%v", i, tt.wantErr, err)
		}
		if !tt.wantErr {
			if root != tt.wantRoot || depth != tt.wantDepth {
				t.Errorf("test %d: got (%q, %d), want (%q, %d)", i, root, depth, tt.wantRoot, tt.wantDepth)
			}
		}
	}
}

func TestSegmentAwareExclusion(t *testing.T) {
	root := t.TempDir()

	// 실제 go/pkg/mod 는 제외된다
	gopkgmod := filepath.Join(root, "go", "pkg", "mod", "example.com", ".env")
	write(t, gopkgmod, "SECRET=abc\n")

	// mongo/pkg/mod 같은 무관한 경로는 제외되지 않는다
	mongopkgmod := filepath.Join(root, "mongo", "pkg", "mod", "local", ".env")
	write(t, mongopkgmod, "SECRET=def\n")

	found, _ := Scan(root, DefaultDepth)
	foundPaths := paths(found)

	// go/pkg/mod 의 .env 는 스킵되어야 한다
	if contains(foundPaths, gopkgmod) {
		t.Errorf("go/pkg/mod 를 제외하지 않았다: %v", foundPaths)
	}

	// mongo/pkg/mod 의 .env 는 보고되어야 한다
	if !contains(foundPaths, mongopkgmod) {
		t.Errorf("mongo/pkg/mod 를 잘못 제외했다: %v", foundPaths)
	}
}
