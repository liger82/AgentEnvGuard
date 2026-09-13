package policy

import "testing"

func TestClassifyPath(t *testing.T) {
	tests := []struct {
		in   string
		want PathKind
	}{
		{".env.keys", PathEnvKeys},
		{"/Users/me/proj/.env.keys", PathEnvKeys},
		{".env.keys.bak", PathEnvKeys},
		{"./.env.keys", PathEnvKeys},

		{".env", PathEnvFile},
		{"/Users/me/proj/.env", PathEnvFile},
		{".env.local", PathEnvFile},
		{".env.production", PathEnvFile},

		{".env.example", PathEnvExample},
		{".env.template", PathEnvExample},
		{".env.sample", PathEnvExample},
		{"/Users/me/proj/.env.example", PathEnvExample},

		{"main.go", PathOther},
		{"README.md", PathOther},
		{"envelope.txt", PathOther},
		{"my.env.txt", PathOther},
		{"", PathOther},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ClassifyPath(tt.in); got != tt.want {
				t.Errorf("ClassifyPath(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
