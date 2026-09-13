package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/liger82/AgentEnvGuard/internal/policy"
)

// Runner 는 외부 명령 실행을 추상화한다. 테스트에서 주입한다.
type Runner func(dir, name string, args ...string) ([]byte, error)

func ExecRunner(dir, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	return c.CombinedOutput()
}

func GitignoreHas(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

// EnsureGitignore 는 entry 가 없으면 추가한다. 이미 있으면 아무것도 하지 않는다.
func EnsureGitignore(dir, entry string) (bool, error) {
	p := filepath.Join(dir, ".gitignore")
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	content := string(b)
	if GitignoreHas(content, entry) {
		return false, nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Init 은 프로젝트 하나를 dotenvx 로 마이그레이션한다.
func Init(dir string, run Runner, stdout io.Writer) error {
	envPath := filepath.Join(dir, ".env")
	if _, err := os.Stat(envPath); err != nil {
		return fmt.Errorf("%s 에 .env 가 없습니다", dir)
	}

	if _, err := run(dir, "dotenvx", "--version"); err != nil {
		return fmt.Errorf(`dotenvx 가 설치되어 있지 않습니다.

  brew install dotenvx

또는 https://dotenvx.com 의 설치 방법을 따르세요`)
	}

	switch policy.ClassifyEnvFile(envPath) {
	case policy.EnvEncrypted:
		fmt.Fprintf(stdout, "이미 암호화되어 있습니다: %s\n", envPath)
	case policy.EnvEmpty:
		fmt.Fprintf(stdout, "%s 에 값이 없습니다. 암호화를 건너뜁니다.\n", envPath)
	default:
		out, err := run(dir, "dotenvx", "encrypt")
		if err != nil {
			return fmt.Errorf("dotenvx encrypt 실패: %w\n%s", err, out)
		}
		fmt.Fprintf(stdout, "암호화했습니다: %s\n", envPath)
	}

	added, err := EnsureGitignore(dir, ".env.keys")
	if err != nil {
		return err
	}
	if added {
		fmt.Fprintln(stdout, ".gitignore 에 .env.keys 를 추가했습니다.")
	}

	// .env 는 자동으로 건드리지 않는다. 안내만 한다.
	if b, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
		if GitignoreHas(string(b), ".env") {
			fmt.Fprintln(stdout, `
안내: .gitignore 에 .env 가 있습니다. 암호화된 .env 는 커밋해도 안전하므로
그 줄을 빼면 팀과 공유할 수 있습니다. 자동으로 빼지 않았습니다 — 커밋하지
않아도 aeg 와 dotenvx 는 정상 동작합니다.`)
		}
	}

	// 가장 중요한 단계. 암호화해도 히스토리의 평문은 남는다.
	if out, err := run(dir, "git", "log", "--all", "--oneline", "--", ".env"); err == nil {
		if strings.TrimSpace(string(out)) != "" {
			fmt.Fprintf(stdout, `
경고: 평문 .env 가 과거에 커밋된 적이 있습니다.

%s
암호화해도 git 히스토리의 평문은 그대로 남습니다. 해당 키를 발급처에서
로테이션하세요. 암호화했다는 이유로 안심하면 안 됩니다.
`, strings.TrimSpace(string(out)))
		}
	} else {
		fmt.Fprintf(stdout, `
알림: git 히스토리를 확인할 수 없습니다 (git 이 없거나 git 저장소가 아님).
프로젝트가 버전 관리 중이라면, 평문 .env 가 과거에 커밋된 적 있는지
수동으로 확인하세요. 있다면 해당 키를 발급처에서 로테이션하세요.
`)
	}

	fmt.Fprintf(stdout, "\n이제 스크립트를 이렇게 실행하세요:\n  dotenvx run -- <실행할 명령>\n")
	return nil
}
