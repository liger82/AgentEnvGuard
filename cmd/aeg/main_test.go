package main

import (
	"bytes"
	"strings"
	"testing"
)

// 사용법 오류는 1로 끝나야 한다. 2는 Claude Code 훅에서 "차단" 신호라서,
// 인자 없이 잘못 등록되면 모든 도구 호출이 막힌다.
func TestRunUsageErrorsExitOne(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"인자 없음", nil},
		{"알 수 없는 명령", []string{"bogus"}},
		{"scan 플래그 오류", []string{"scan", "--unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(tt.args, strings.NewReader(""), &out, &errOut); code != 1 {
				t.Errorf("종료코드 = %d, want 1", code)
			}
		})
	}
}
