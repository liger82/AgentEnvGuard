package allowlist

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadFrom(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "allow")
	content := "# 주석은 무시\n" +
		"/Users/me/scratch\n" +
		"\n" +
		"   /Users/me/legacy   \n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadFrom(p)
	want := []string{"/Users/me/scratch", "/Users/me/legacy"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadFrom() = %v, want %v", got, want)
	}
}

func TestLoadFromMissingIsEmpty(t *testing.T) {
	if got := LoadFrom("/nonexistent/allow"); len(got) != 0 {
		t.Errorf("LoadFrom() = %v, want empty", got)
	}
}

func TestCovers(t *testing.T) {
	roots := []string{"/Users/me/scratch", "/Users/me/legacy"}
	tests := []struct {
		target string
		want   bool
	}{
		{"/Users/me/scratch/.env", true},
		{"/Users/me/scratch/deep/nested/.env", true},
		{"/Users/me/scratch", true},
		{"/Users/me/legacy/app/.env.keys", true},
		{"/Users/me/other/.env", false},
		{"/Users/me/scratchpad/.env", false}, // 접두어만 같은 형제 디렉터리
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := Covers(roots, tt.target); got != tt.want {
				t.Errorf("Covers(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}
