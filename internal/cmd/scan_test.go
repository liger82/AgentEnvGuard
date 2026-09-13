package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

// .env.keys 는 dotenvx 를 제대로 쓰는 프로젝트에 항상 있으므로 평문 시크릿으로
// 보고하지 않는다. .gitignore 에 없을 때만 커밋 위험으로 따로 보고한다.
func TestScanEnvKeysOnlyWhenNotGitignored(t *testing.T) {
	root := t.TempDir()
	exposed := filepath.Join(root, "a", ".env.keys")
	ignored := filepath.Join(root, "b", ".env.keys")
	write(t, exposed, "DOTENV_PRIVATE_KEY=\"122\"\n")
	write(t, ignored, "DOTENV_PRIVATE_KEY=\"122\"\n")
	write(t, filepath.Join(root, "b", ".gitignore"), "node_modules/\n.env.keys\n")

	found, _ := Scan(root, DefaultDepth)
	kinds := map[string]FindingKind{}
	for _, f := range found {
		kinds[f.Path] = f.Kind
	}
	if k, ok := kinds[exposed]; !ok || k != FindingKeysNotIgnored {
		t.Errorf("gitignore 에 없는 .env.keys 를 FindingKeysNotIgnored 로 보고하지 않았다: %v", found)
	}
	if _, ok := kinds[ignored]; ok {
		t.Errorf("gitignore 된 .env.keys 를 보고했다: %v", found)
	}
}

func TestScanPlaintextVariantKind(t *testing.T) {
	root := t.TempDir()
	env := filepath.Join(root, "a", ".env")
	local := filepath.Join(root, "a", ".env.local")
	write(t, env, "SECRET=abc\n")
	write(t, local, "SECRET=abc\n")

	found, _ := Scan(root, DefaultDepth)
	kinds := map[string]FindingKind{}
	for _, f := range found {
		kinds[f.Path] = f.Kind
	}
	if kinds[env] != FindingPlaintextEnv {
		t.Errorf(".env 종류 = %v, want FindingPlaintextEnv", kinds[env])
	}
	if kinds[local] != FindingPlaintextVariant {
		t.Errorf(".env.local 종류 = %v, want FindingPlaintextVariant", kinds[local])
	}
}

// dotenvx 로 마이그레이션을 마친 프로젝트(암호문 .env + gitignore 된 .env.keys)는
// 아무것도 보고하지 않아야 한다.
func TestRunScanCleanAfterMigration(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "p", ".env"), "DOTENV_PUBLIC_KEY=\"03\"\nSECRET=\"encrypted:xx\"\n")
	write(t, filepath.Join(root, "p", ".env.keys"), "DOTENV_PRIVATE_KEY=\"122\"\n")
	write(t, filepath.Join(root, "p", ".gitignore"), ".env.keys\n")

	var out bytes.Buffer
	if err := RunScan(root, DefaultDepth, &out); err != nil {
		t.Fatal(err)
	}
	if found, _ := Scan(root, DefaultDepth); len(found) != 0 {
		t.Errorf("깨끗한 프로젝트인데 보고했다: %v", paths(found))
	}
	if strings.Contains(out.String(), "aeg init") || strings.Contains(out.String(), ".env.keys") {
		t.Errorf("깨끗한 프로젝트인데 조치를 안내했다:\n%s", out.String())
	}
}

func TestRunScanVariantHintIsDotenvxEncrypt(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "p", ".env.production"), "SECRET=abc\n")

	var out bytes.Buffer
	if err := RunScan(root, DefaultDepth, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dotenvx encrypt -f") {
		t.Errorf(".env.production 에 dotenvx encrypt -f 안내가 없다:\n%s", out.String())
	}
	if strings.Contains(out.String(), "aeg init") {
		t.Errorf("aeg init 은 .env.production 을 마이그레이션하지 않는데 안내했다:\n%s", out.String())
	}
}

func TestRunScanKeysNotIgnoredHint(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "p", ".env.keys"), "DOTENV_PRIVATE_KEY=\"122\"\n")

	var out bytes.Buffer
	if err := RunScan(root, DefaultDepth, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), ".gitignore") {
		t.Errorf(".gitignore 추가 안내가 없다:\n%s", out.String())
	}
	if strings.Contains(out.String(), "aeg init") {
		t.Errorf(".env.keys 에 aeg init 을 안내했다:\n%s", out.String())
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
