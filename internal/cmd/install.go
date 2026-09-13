package cmd

import (
	"encoding/json"
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
			return nil, false, fmt.Errorf("settings.json 이 올바른 JSON 이 아닙니다: %w", err)
		}
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
// 덮어쓰기 전에 .aeg-backup 을 남긴다.
func Install(settingsPath, binPath string, stdout io.Writer) error {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		raw = []byte("{}")
	} else {
		if err := os.WriteFile(settingsPath+".aeg-backup", raw, 0o600); err != nil {
			return fmt.Errorf("백업 실패: %w", err)
		}
	}

	out, changed, err := MergeHook(raw, binPath)
	if err != nil {
		return fmt.Errorf("%w\n\n%s 를 직접 고친 뒤 다시 실행하세요. 덮어쓰지 않았습니다", err, settingsPath)
	}
	if !changed {
		fmt.Fprintf(stdout, "이미 설치되어 있습니다: %s\n", settingsPath)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "설치했습니다: %s\n대상 도구: %s\n\n다음으로 평문 .env 를 찾아보세요:\n  aeg scan\n",
		settingsPath, HookMatcher)
	return nil
}
