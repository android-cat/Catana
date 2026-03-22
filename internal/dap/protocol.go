// Package dap はDebug Adapter Protocolクライアントを提供する
package dap

import (
	"encoding/json"
)

// ProtocolMessage はDAP基本メッセージ
type ProtocolMessage struct {
	Seq  int    `json:"seq"`
	Type string `json:"type"` // "request", "response", "event"
}

// Request はDAPリクエスト
type Request struct {
	ProtocolMessage
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Response はDAPレスポンス
type Response struct {
	ProtocolMessage
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// Event はDAPイベント
type Event struct {
	ProtocolMessage
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// ─── 初期化 ───

// InitializeArguments は初期化引数
type InitializeArguments struct {
	ClientID                     string `json:"clientID,omitempty"`
	ClientName                   string `json:"clientName,omitempty"`
	AdapterID                    string `json:"adapterID"`
	Locale                       string `json:"locale,omitempty"`
	LinesStartAt1                bool   `json:"linesStartAt1"`
	ColumnsStartAt1              bool   `json:"columnsStartAt1"`
	PathFormat                   string `json:"pathFormat,omitempty"`
	SupportsVariableType         bool   `json:"supportsVariableType,omitempty"`
	SupportsVariablePaging       bool   `json:"supportsVariablePaging,omitempty"`
	SupportsRunInTerminalRequest bool   `json:"supportsRunInTerminalRequest,omitempty"`
}

// Capabilities はDAP機能情報（InitializeResponseのエイリアス）
type Capabilities = InitializeResponse

// InitializeResponse は初期化レスポンス
type InitializeResponse struct {
	SupportsConfigurationDoneRequest bool `json:"supportsConfigurationDoneRequest,omitempty"`
	SupportsFunctionBreakpoints      bool `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints   bool `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers        bool `json:"supportsEvaluateForHovers,omitempty"`
	SupportsStepBack                 bool `json:"supportsStepBack,omitempty"`
	SupportsSetVariable              bool `json:"supportsSetVariable,omitempty"`
	SupportsRestartFrame             bool `json:"supportsRestartFrame,omitempty"`
}

// LaunchRequestArguments はプログラム起動引数
type LaunchRequestArguments struct {
	Program     string            `json:"program"`
	Args        []string          `json:"args,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	StopOnEntry bool              `json:"stopOnEntry,omitempty"`
	NoDebug     bool              `json:"noDebug,omitempty"`
}

// LaunchArguments はプログラム起動引数（LaunchRequestArgumentsの別名）
type LaunchArguments = LaunchRequestArguments

// AttachArguments はプロセスアタッチ引数
type AttachArguments struct {
	ProcessID int `json:"processId,omitempty"`
	Port      int `json:"port,omitempty"`
}

// ─── ブレークポイント ───

// Source はソース情報
type Source struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// SourceBreakpoint はソースブレークポイント
type SourceBreakpoint struct {
	Line      int    `json:"line"`
	Column    int    `json:"column,omitempty"`
	Condition string `json:"condition,omitempty"`
}

// Breakpoint はブレークポイント情報
type Breakpoint struct {
	ID       int     `json:"id,omitempty"`
	Verified bool    `json:"verified"`
	Line     int     `json:"line,omitempty"`
	Source   *Source `json:"source,omitempty"`
}

// SetBreakpointsArguments はブレークポイント設定引数
type SetBreakpointsArguments struct {
	Source      Source             `json:"source"`
	Breakpoints []SourceBreakpoint `json:"breakpoints"`
}

// SetBreakpointsResponse はブレークポイント設定レスポンス
type SetBreakpointsResponse struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// ─── 実行制御 ───

// ContinueArguments は実行継続引数
type ContinueArguments struct {
	ThreadID int `json:"threadId"`
}

// StepArguments はステップ実行引数
type StepArguments struct {
	ThreadID int `json:"threadId"`
}

// ─── スレッド/スタック ───

// Thread はスレッド情報
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ThreadsResponse はスレッドリスト
type ThreadsResponse struct {
	Threads []Thread `json:"threads"`
}

// StackTraceArguments はスタックトレース引数
type StackTraceArguments struct {
	ThreadID   int `json:"threadId"`
	StartFrame int `json:"startFrame,omitempty"`
	Levels     int `json:"levels,omitempty"`
}

// StackFrame はスタックフレーム
type StackFrame struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Source *Source `json:"source,omitempty"`
	Line   int     `json:"line"`
	Column int     `json:"column"`
}

// StackTraceResponse はスタックトレースレスポンス
type StackTraceResponse struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames,omitempty"`
}

// ─── 変数 ───

// ScopesArguments はスコープ引数
type ScopesArguments struct {
	FrameID int `json:"frameId"`
}

// Scope はスコープ情報
type Scope struct {
	Name               string `json:"name"`
	VariablesReference int    `json:"variablesReference"`
	Expensive          bool   `json:"expensive,omitempty"`
}

// ScopesResponse はスコープレスポンス
type ScopesResponse struct {
	Scopes []Scope `json:"scopes"`
}

// VariablesArguments は変数引数
type VariablesArguments struct {
	VariablesReference int `json:"variablesReference"`
}

// Variable は変数情報
type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference,omitempty"`
}

// VariablesResponse は変数レスポンス
type VariablesResponse struct {
	Variables []Variable `json:"variables"`
}

// ─── Watch/Evaluate ───

// EvaluateArguments は式評価引数
type EvaluateArguments struct {
	Expression string `json:"expression"`
	FrameID    int    `json:"frameId,omitempty"`
	Context    string `json:"context,omitempty"` // "watch", "repl", "hover"
}

// EvaluateResponse は式評価レスポンス
type EvaluateResponse struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	VariablesReference int    `json:"variablesReference,omitempty"`
}

// ─── イベント ───

// StoppedEventBody は停止イベントのボディ
type StoppedEventBody struct {
	Reason            string `json:"reason"` // "breakpoint", "step", "exception", "pause"
	ThreadID          int    `json:"threadId,omitempty"`
	AllThreadsStopped bool   `json:"allThreadsStopped,omitempty"`
	Description       string `json:"description,omitempty"`
	Text              string `json:"text,omitempty"`
}

// OutputEventBody は出力イベントのボディ
type OutputEventBody struct {
	Category string `json:"category,omitempty"` // "console", "stdout", "stderr"
	Output   string `json:"output"`
}

// TerminatedEventBody は終了イベントのボディ
type TerminatedEventBody struct {
	Restart bool `json:"restart,omitempty"`
}

// BreakpointEventBody はブレークポイント変更イベントのボディ
type BreakpointEventBody struct {
	Reason     string     `json:"reason"` // "changed", "new", "removed"
	Breakpoint Breakpoint `json:"breakpoint"`
}
