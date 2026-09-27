package policy

type ToolKind int

const (
	ToolUnknown ToolKind = iota
	ToolBash
	ToolFileRead      // Read
	ToolContentSearch // Grep — 매칭된 줄 내용을 반환하므로 반드시 막아야 한다
)

// ToolCall 은 에이전트 도구 호출을 도구 중립적으로 표현한 것이다.
type ToolCall struct {
	Kind    ToolKind
	Command string // ToolBash
	Path    string // ToolFileRead, ToolContentSearch
	Pattern string // ToolContentSearch
	Glob    string // ToolContentSearch — 검색할 파일을 고르는 glob
	Cwd     string // 상대 경로 해석 기준
}

// Decision 은 판정 결과다. 차단할 때 Reason 은 반드시 올바른 대안을 담는다.
type Decision struct {
	Allow  bool
	Reason string
}
