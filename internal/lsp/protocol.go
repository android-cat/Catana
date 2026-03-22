// Package lsp はLanguage Server Protocolクライアントを提供する
package lsp

import (
	"encoding/json"
)

// JSON-RPC 2.0 メッセージ型

// RequestMessage はJSON-RPCリクエストメッセージ
type RequestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ResponseMessage はJSON-RPCレスポンスメッセージ
type ResponseMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// NotificationMessage はJSON-RPC通知メッセージ
type NotificationMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ResponseError はJSON-RPCエラー
type ResponseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// LSP エラーコード
const (
	ParseError           = -32700
	InvalidRequest       = -32600
	MethodNotFound       = -32601
	InvalidParams        = -32602
	InternalError        = -32603
	ServerNotInitialized = -32002
	RequestCancelled     = -32800
)

// ─── 初期化関連 ───

// InitializeParams は初期化リクエストのパラメータ
type InitializeParams struct {
	ProcessID    int                `json:"processId"`
	RootURI      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
}

// ClientCapabilities はクライアントの機能
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
}

// TextDocumentClientCapabilities はテキストドキュメント関連の機能
type TextDocumentClientCapabilities struct {
	Completion      *CompletionClientCapabilities   `json:"completion,omitempty"`
	Definition      *DefinitionClientCapabilities   `json:"definition,omitempty"`
	References      *ReferencesClientCapabilities   `json:"references,omitempty"`
	Rename          *RenameClientCapabilities       `json:"rename,omitempty"`
	PublishDiag     *PublishDiagnosticsCapabilities `json:"publishDiagnostics,omitempty"`
	CodeAction      *CodeActionClientCapabilities   `json:"codeAction,omitempty"`
	Formatting      *FormattingClientCapabilities   `json:"formatting,omitempty"`
	Synchronization *TextDocSyncClientCapabilities  `json:"synchronization,omitempty"`
}

type CompletionClientCapabilities struct {
	CompletionItem *CompletionItemCapabilities `json:"completionItem,omitempty"`
}

type CompletionItemCapabilities struct {
	SnippetSupport bool `json:"snippetSupport,omitempty"`
}

type DefinitionClientCapabilities struct{}
type ReferencesClientCapabilities struct{}

type RenameClientCapabilities struct {
	PrepareSupport bool `json:"prepareSupport,omitempty"`
}

type PublishDiagnosticsCapabilities struct{}
type CodeActionClientCapabilities struct{}
type FormattingClientCapabilities struct{}

type TextDocSyncClientCapabilities struct {
	DidSave bool `json:"didSave,omitempty"`
}

// InitializeResult は初期化レスポンス
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities はサーバーの機能
type ServerCapabilities struct {
	TextDocumentSync           int                `json:"textDocumentSync,omitempty"` // 0=None, 1=Full, 2=Incremental
	CompletionProvider         *CompletionOptions `json:"completionProvider,omitempty"`
	DefinitionProvider         bool               `json:"definitionProvider,omitempty"`
	ReferencesProvider         bool               `json:"referencesProvider,omitempty"`
	RenameProvider             bool               `json:"renameProvider,omitempty"`
	DocumentFormattingProvider bool               `json:"documentFormattingProvider,omitempty"`
	CodeActionProvider         bool               `json:"codeActionProvider,omitempty"`
}

// CompletionOptions は補完のオプション
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// ─── テキストドキュメント関連 ───

// TextDocumentIdentifier はドキュメント識別子
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier はバージョン付きドキュメント識別子
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// TextDocumentItem はドキュメントの内容を含む識別子
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// Position はドキュメント内の位置（0始まり）
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range はドキュメント内の範囲
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location はドキュメント内の場所
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextEdit はテキスト編集操作
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// ─── ドキュメント同期 ───

// DidOpenTextDocumentParams はドキュメントを開いた通知のパラメータ
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidCloseTextDocumentParams はドキュメントを閉じた通知のパラメータ
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidChangeTextDocumentParams はドキュメント変更通知のパラメータ
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent は変更イベント（Full Sync用）
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"` // ドキュメント全体のテキスト
}

// DidSaveTextDocumentParams は保存通知のパラメータ
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// ─── 補完 ───

// CompletionParams は補完リクエストのパラメータ
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CompletionList は補完候補リスト
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// CompletionItem は補完候補
type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
	FilterText    string `json:"filterText,omitempty"`
	SortText      string `json:"sortText,omitempty"`
}

// 補完アイテムの種別
const (
	CompletionKindText          = 1
	CompletionKindMethod        = 2
	CompletionKindFunction      = 3
	CompletionKindConstructor   = 4
	CompletionKindField         = 5
	CompletionKindVariable      = 6
	CompletionKindClass         = 7
	CompletionKindInterface     = 8
	CompletionKindModule        = 9
	CompletionKindProperty      = 10
	CompletionKindKeyword       = 14
	CompletionKindSnippet       = 15
	CompletionKindFile          = 17
	CompletionKindFolder        = 19
	CompletionKindConstant      = 21
	CompletionKindStruct        = 22
	CompletionKindTypeParameter = 25
)

// ─── 定義・参照 ───

// DefinitionParams は定義ジャンプのパラメータ
type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ReferenceParams は参照検索のパラメータ
type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

// ReferenceContext は参照コンテキスト
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// ─── リネーム ───

// RenameParams はリネームのパラメータ
type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

// WorkspaceEdit はワークスペース全体の編集
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// ─── 診断 ───

// PublishDiagnosticsParams は診断通知のパラメータ
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnostic は診断情報（エラー・警告など）
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"` // 1=Error, 2=Warning, 3=Information, 4=Hint
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// 診断の重大度
const (
	SeverityError       = 1
	SeverityWarning     = 2
	SeverityInformation = 3
	SeverityHint        = 4
)

// ─── コードアクション ───

// CodeActionParams はコードアクションのパラメータ
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext はコードアクションのコンテキスト
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// CodeAction はコードアクション
type CodeAction struct {
	Title string         `json:"title"`
	Kind  string         `json:"kind,omitempty"`
	Edit  *WorkspaceEdit `json:"edit,omitempty"`
}

// ─── フォーマット ───

// DocumentFormattingParams はドキュメントフォーマットのパラメータ
type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Options      FormattingOptions      `json:"options"`
}

// FormattingOptions はフォーマットオプション
type FormattingOptions struct {
	TabSize      int  `json:"tabSize"`
	InsertSpaces bool `json:"insertSpaces"`
}

// ─── ユーティリティ ───

// FilePathToURI はファイルパスをfile://URIに変換する
func FilePathToURI(path string) string {
	return "file://" + path
}

// URIToFilePath はfile://URIをファイルパスに変換する
func URIToFilePath(uri string) string {
	const prefix = "file://"
	if len(uri) > len(prefix) && uri[:len(prefix)] == prefix {
		return uri[len(prefix):]
	}
	return uri
}
