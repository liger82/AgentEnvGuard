package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

// DefaultDepth 는 root 로부터의 최대 탐색 깊이다.
// 홈 전체를 무제한으로 훑으면 너무 느리다.
const DefaultDepth = 6

// excludedDirs 는 훑지 않는 디렉터리 이름이다.
var excludedDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true,
	"dist": true, "build": true, "target": true,
	".venv": true, "venv": true, ".cache": true, ".Trash": true,
	"Library": true, // macOS. 거대하고 프로젝트가 없다
}

// excludedPathSegments 는 연속된 경로 세그먼트로 판단하는 제외 대상이다.
// 이름만으로 거르면 Go 프로젝트의 정상 pkg/ 디렉터리까지 건너뛰게 된다.
var excludedPathSegments = [][]string{
	{"go", "pkg", "mod"},
}

// containsConsecutiveSegments 는 경로가 연속된 세그먼트를 포함하는지 확인한다.
// mongo/pkg/mod 같은 무관한 경로는 매치되지 않도록 한다.
func containsConsecutiveSegments(path string, segments []string) bool {
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) < len(segments) {
		return false
	}
	for i := 0; i <= len(parts)-len(segments); i++ {
		match := true
		for j, seg := range segments {
			if parts[i+j] != seg {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// slowDirs 는 네트워크 동기화 폴더다. 만나면 경고하고 계속한다.
var slowDirs = []string{"Google Drive", "Dropbox", "CloudStorage", "OneDrive"}

type Finding struct {
	Path string
}

// Scan 은 root 아래에서 평문 .env 와 .env.keys 를 찾는다.
// 권한이 없어 못 읽은 디렉터리 수를 함께 반환한다.
func Scan(root string, maxDepth int) ([]Finding, int) {
	root = filepath.Clean(root)
	rootDepth := strings.Count(root, string(filepath.Separator))

	var findings []Finding
	skipped := 0

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			skipped++
			return nil // 권한 없는 디렉터리는 건너뛰고 계속한다
		}
		if d.IsDir() {
			if path != root && excludedDirs[d.Name()] {
				return fs.SkipDir
			}
			for _, segs := range excludedPathSegments {
				if containsConsecutiveSegments(path, segs) {
					return fs.SkipDir
				}
			}
			if strings.Count(path, string(filepath.Separator))-rootDepth >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}

		// 이름으로 먼저 거른다. 내용 읽기는 후보에만 한다.
		switch policy.ClassifyPath(path) {
		case policy.PathEnvKeys:
			findings = append(findings, Finding{Path: path})
		case policy.PathEnvFile:
			if policy.ClassifyEnvFile(path) == policy.EnvPlaintext {
				findings = append(findings, Finding{Path: path})
			}
		}
		return nil
	})
	return findings, skipped
}

// ParseScanArgs 는 aeg scan 의 커맨드라인 인수를 파싱한다.
// --depth N 또는 --depth=N 플래그와 선택적 경로를 처리한다.
func ParseScanArgs(args []string) (root string, depth int, err error) {
	depth = DefaultDepth

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--depth" {
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("--depth 에 값이 필요합니다")
			}
			depthStr := args[i+1]
			depthVal, err := strconv.Atoi(depthStr)
			if err != nil {
				return "", 0, fmt.Errorf("--depth 값이 숫자가 아닙니다: %q", depthStr)
			}
			if depthVal <= 0 {
				return "", 0, fmt.Errorf("--depth 는 양수여야 합니다: %d", depthVal)
			}
			depth = depthVal
			i++ // skip next arg
		} else if strings.HasPrefix(arg, "--depth=") {
			depthStr := strings.TrimPrefix(arg, "--depth=")
			if depthStr == "" {
				return "", 0, fmt.Errorf("--depth= 에 값이 필요합니다")
			}
			depthVal, err := strconv.Atoi(depthStr)
			if err != nil {
				return "", 0, fmt.Errorf("--depth 값이 숫자가 아닙니다: %q", depthStr)
			}
			if depthVal <= 0 {
				return "", 0, fmt.Errorf("--depth 는 양수여야 합니다: %d", depthVal)
			}
			depth = depthVal
		} else if !strings.HasPrefix(arg, "-") {
			// positional argument
			if root != "" {
				return "", 0, fmt.Errorf("경로가 여러 개입니다: %q, %q", root, arg)
			}
			root = arg
		} else {
			return "", 0, fmt.Errorf("알 수 없는 플래그: %q", arg)
		}
	}

	return root, depth, nil
}

func RunScan(root string, maxDepth int, stdout io.Writer) error {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		root = home
	}
	for _, s := range slowDirs {
		if strings.Contains(root, s) {
			fmt.Fprintf(stdout, "경고: %s 는 네트워크 동기화 폴더라 느릴 수 있습니다.\n\n", s)
			break
		}
	}

	fmt.Fprintf(stdout, "%s 아래를 훑는 중 (최대 깊이 %d)...\n\n", root, maxDepth)
	findings, skipped := Scan(root, maxDepth)

	if len(findings) == 0 {
		fmt.Fprintln(stdout, "평문 시크릿 파일을 찾지 못했습니다.")
	} else {
		fmt.Fprintf(stdout, "평문 시크릿 파일 %d개:\n\n", len(findings))
		for _, f := range findings {
			fmt.Fprintf(stdout, "  %s\n", f.Path)
		}
		fmt.Fprintf(stdout, "\n각 프로젝트에서 다음을 실행하세요:\n  aeg init <프로젝트 경로>\n")
	}
	if skipped > 0 {
		fmt.Fprintf(stdout, "\n권한이 없어 건너뛴 디렉터리: %d개\n", skipped)
	}
	fmt.Fprintf(stdout, "\n느리면 범위를 좁히세요:  aeg scan ~/projects\n")
	return nil
}
