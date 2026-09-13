// Package allowlist 는 사용자가 판정에서 제외한 경로 목록을 다룬다.
package allowlist

import (
	"os"
	"path/filepath"
	"strings"
)

// Path 는 예외 목록 파일의 위치다.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "aeg", "allow")
}

// LoadFrom 은 한 줄에 하나씩 적힌 경로를 읽는다.
// 주석(#)과 빈 줄은 버린다. 읽을 수 없으면 빈 슬라이스를 반환한다.
func LoadFrom(path string) []string {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var roots []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		roots = append(roots, filepath.Clean(line))
	}
	return roots
}

// Load 는 기본 위치에서 읽는다.
func Load() []string { return LoadFrom(Path()) }

// Covers 는 target 이 roots 중 하나의 하위 경로인지 본다.
// 문자열 접두어 비교로는 /a/scratch 가 /a/scratchpad 를 삼키므로
// 경로 구분자를 붙여 비교한다.
func Covers(roots []string, target string) bool {
	if target == "" {
		return false
	}
	target = filepath.Clean(target)
	for _, r := range roots {
		if target == r {
			return true
		}
		if strings.HasPrefix(target, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
