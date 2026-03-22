package ai

import (
	"fmt"
	"strings"
)

// Context はAIに送信するコンテキスト情報を管理する
type Context struct {
	// 開いているファイルの情報
	OpenFiles []FileContext
	// 選択範囲
	Selection *SelectionContext
	// Gitの差分情報
	GitDiff string
	// アクティブファイルのパス
	ActiveFile string
	// カーソル位置のシンボル情報（LSP）
	Symbols *SymbolContext
}

// FileContext は1つのファイルのコンテキスト
type FileContext struct {
	Path     string // ファイルパス
	Language string // プログラミング言語
	Content  string // ファイル内容
	Active   bool   // 現在アクティブなファイルか
}

// SelectionContext は選択されたコードのコンテキスト
type SelectionContext struct {
	FilePath  string // ファイルパス
	Language  string // 言語
	Text      string // 選択テキスト
	StartLine int    // 開始行
	EndLine   int    // 終了行
}

// SymbolContext はカーソル位置のLSPシンボル情報
type SymbolContext struct {
	HoverInfo   string       // ホバー情報（型シグネチャ等）
	Symbols     []SymbolInfo // ドキュメント内のシンボル一覧
	Diagnostics []string     // 診断情報（エラー・警告）
	CursorLine  int          // カーソル行
	CursorCol   int          // カーソル列
}

// SymbolInfo はシンボルの基本情報
type SymbolInfo struct {
	Name   string // シンボル名
	Kind   string // 種別（function, method, class等）
	Detail string // 詳細（シグネチャ等）
	Line   int    // 定義行
}

// ActionType はAIアクションの種類
type ActionType int

const (
	ActionChat     ActionType = iota // 通常のチャット
	ActionRefactor                   // コードリファクタリング
	ActionExplain                    // コード解説
	ActionGenTest                    // テスト生成
	ActionGenDoc                     // ドキュメント生成
)

// BuildSystemPrompt はアクションに応じたシステムプロンプトを構築する
func BuildSystemPrompt(action ActionType) string {
	base := "あなたはCatanaコードエディタに統合されたAIアシスタントです。"

	switch action {
	case ActionRefactor:
		return base + "ユーザーが選択したコードのリファクタリングを提案してください。改善点を説明し、修正後のコードをdiff形式で提示してください。"
	case ActionExplain:
		return base + "ユーザーが選択したコードを詳しく解説してください。各部分の役割、設計パターン、注意点を日本語で説明してください。"
	case ActionGenTest:
		return base + "ユーザーが選択したコードのユニットテストを生成してください。テストケースはエッジケースも含めて網羅的に作成してください。"
	case ActionGenDoc:
		return base + "ユーザーが選択したコードのドキュメントコメントを生成してください。関数の説明、パラメータ、戻り値を含めてください。"
	default:
		return base + "コードに関する質問に回答し、コードの編集・生成・説明を行ってください。回答は日本語で、コードブロックにはdiff形式を使用してください。"
	}
}

// BuildContextPrompt はコンテキスト情報をプロンプトとして構築する
func BuildContextPrompt(ctx *Context) string {
	if ctx == nil {
		return ""
	}

	var parts []string

	// アクティブファイルの情報
	for _, f := range ctx.OpenFiles {
		if f.Active {
			parts = append(parts, fmt.Sprintf("## 現在のファイル: %s (%s)\n```%s\n%s\n```",
				f.Path, f.Language, f.Language, truncateContent(f.Content, 8000)))
			break
		}
	}

	// 選択範囲
	if ctx.Selection != nil && ctx.Selection.Text != "" {
		parts = append(parts, fmt.Sprintf("## 選択されたコード (%s 行%d-%d)\n```%s\n%s\n```",
			ctx.Selection.FilePath, ctx.Selection.StartLine+1, ctx.Selection.EndLine+1,
			ctx.Selection.Language, ctx.Selection.Text))
	}

	// Git差分
	if ctx.GitDiff != "" {
		parts = append(parts, fmt.Sprintf("## Git差分\n```diff\n%s\n```",
			truncateContent(ctx.GitDiff, 4000)))
	}

	// LSPシンボル情報
	if ctx.Symbols != nil {
		var symParts []string
		if ctx.Symbols.HoverInfo != "" {
			symParts = append(symParts, fmt.Sprintf("### カーソル位置の型情報 (行%d, 列%d)\n```\n%s\n```",
				ctx.Symbols.CursorLine+1, ctx.Symbols.CursorCol+1, ctx.Symbols.HoverInfo))
		}
		if len(ctx.Symbols.Symbols) > 0 {
			symParts = append(symParts, "### ドキュメント内のシンボル")
			for _, sym := range ctx.Symbols.Symbols {
				detail := ""
				if sym.Detail != "" {
					detail = " — " + sym.Detail
				}
				symParts = append(symParts, fmt.Sprintf("- %s `%s`%s (行%d)", sym.Kind, sym.Name, detail, sym.Line+1))
			}
		}
		if len(ctx.Symbols.Diagnostics) > 0 {
			symParts = append(symParts, "### 診断情報")
			for _, d := range ctx.Symbols.Diagnostics {
				symParts = append(symParts, "- "+d)
			}
		}
		if len(symParts) > 0 {
			parts = append(parts, "## LSPシンボル情報\n"+strings.Join(symParts, "\n"))
		}
	}

	// その他の開いているファイル（アクティブ以外）
	otherCount := 0
	for _, f := range ctx.OpenFiles {
		if !f.Active && otherCount < 3 { // 最大3ファイルまで
			parts = append(parts, fmt.Sprintf("## 参照ファイル: %s (%s)\n```%s\n%s\n```",
				f.Path, f.Language, f.Language, truncateContent(f.Content, 2000)))
			otherCount++
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "# コンテキスト情報\n\n" + strings.Join(parts, "\n\n")
}

// truncateContent はコンテンツを最大長まで切り詰める
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n... (省略)"
}
