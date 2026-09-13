package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// HookMatcher 는 판정 대상 도구다.
// Grep 은 매칭된 줄 내용을 반환하므로 반드시 포함해야 한다.
const HookMatcher = "Bash|Read|Grep"

func SettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// MergeHook 은 settings.json 내용에 aeg 훅을 병합한다.
// 이미 있으면 changed=false 로 아무것도 바꾸지 않는다.
//
// map[string]any 로 다루므로 알지 못하는 설정 키도 보존된다.
// 다만 JSON 재직렬화 과정에서 키 순서가 알파벳순으로 바뀐다.
func MergeHook(raw []byte, binPath string) ([]byte, bool, error) {
	root := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				return nil, false, fmt.Errorf("settings.json 최상위가 JSON 객체가 아닙니다 (%s)", typeErr.Value)
			}
			return nil, false, fmt.Errorf("settings.json 이 올바른 JSON 이 아닙니다: %w", err)
		}
	}
	// 파일 내용이 null 이면 Unmarshal 이 map 을 nil 로 만든다. 빈 객체로 본다.
	if root == nil {
		root = map[string]any{}
	}

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)

	// 이미 설치되어 있는지 본다.
	for _, entry := range pre {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := m["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if ok && hm["command"] == binPath {
				return raw, false, nil
			}
		}
	}

	pre = append(pre, map[string]any{
		"matcher": HookMatcher,
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": binPath,
			"args":    []any{"hook"},
		}},
	})
	hooks["PreToolUse"] = pre
	root["hooks"] = hooks

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

// Install 은 settings.json 을 읽어 훅을 병합하고 되쓴다.
//
// 실제로 내용이 바뀔 때만 쓰고, 그때 .aeg-backup 이 아직 없으면 원본을
// 백업한다. 설치 전 최초 상태가 되돌릴 가치가 있는 유일한 상태이므로 이미
// 있는 백업은 덮어쓰지 않는다. 쓰기는 같은 디렉터리의 임시 파일에 쓴 뒤
// rename 해 원자적으로 하고, 기존 파일의 모드를 유지한다.
func Install(settingsPath, binPath string, stdout io.Writer) error {
	existed := true
	mode := os.FileMode(0o644)
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		existed = false
		raw = []byte("{}")
	} else if fi, err := os.Stat(settingsPath); err == nil {
		mode = fi.Mode().Perm()
	}

	out, changed, err := MergeHook(raw, binPath)
	if err != nil {
		return fmt.Errorf("%w\n\n%s 를 직접 고친 뒤 다시 실행하세요. 덮어쓰지 않았습니다", err, settingsPath)
	}
	if !changed {
		fmt.Fprintf(stdout, "이미 설치되어 있습니다: %s\n", settingsPath)
		return nil
	}

	backup := settingsPath + ".aeg-backup"
	if existed {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			if err := os.WriteFile(backup, raw, 0o600); err != nil {
				return fmt.Errorf("백업 실패: %w", err)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(settingsPath, out, mode); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "설치했습니다: %s\n대상 도구: %s\n\n다음으로 평문 .env 를 찾아보세요:\n  aeg scan\n",
		settingsPath, HookMatcher)
	return nil
}

// writeFileAtomic 은 같은 디렉터리의 임시 파일에 쓴 뒤 rename 한다.
// 쓰는 도중 실패해도 기존 파일이 반쯤 잘린 채 남지 않는다.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".aeg-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 에 성공하면 이미 없으므로 무시된다

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
