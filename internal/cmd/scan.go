package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

// excludedPathFragments 는 경로 일부로 판단하는 제외 대상이다.
// 이름만으로 거르면 Go 프로젝트의 정상 pkg/ 디렉터리까지 건너뛰게 된다.
var excludedPathFragments = []string{
	filepath.Join("go", "pkg", "mod"),
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
			for _, frag := range excludedPathFragments {
				if strings.Contains(path, frag) {
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
