package ai

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_各アクション(t *testing.T) {
	tests := []struct {
		action   ActionType
		contains string
	}{
		{ActionChat, "コードに関する質問"},
		{ActionRefactor, "リファクタリング"},
		{ActionExplain, "解説"},
		{ActionGenTest, "テスト"},
		{ActionGenDoc, "ドキュメント"},
	}
	for _, tt := range tests {
		result := BuildSystemPrompt(tt.action)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("BuildSystemPrompt(%d) に %q が含まれない: %q", tt.action, tt.contains, result)
		}
		// 全てのプロンプトが基本プレフィックスを含むこと
		if !strings.HasPrefix(result, "あなたはCatanaコードエディタ") {
			t.Errorf("BuildSystemPrompt(%d) が基本プレフィックスを含まない", tt.action)
		}
	}
}

func TestBuildContextPrompt_nil(t *testing.T) {
	result := BuildContextPrompt(nil)
	if result != "" {
		t.Errorf("BuildContextPrompt(nil) = %q, 期待 空文字", result)
	}
}

func TestBuildContextPrompt_空コンテキスト(t *testing.T) {
	ctx := &Context{}
	result := BuildContextPrompt(ctx)
	if result != "" {
		t.Errorf("空コンテキストで非空文字列が返された: %q", result)
	}
}

func TestBuildContextPrompt_アクティブファイル(t *testing.T) {
	ctx := &Context{
		OpenFiles: []FileContext{
			{Path: "main.go", Language: "go", Content: "package main", Active: true},
		},
	}
	result := BuildContextPrompt(ctx)
	if !strings.Contains(result, "main.go") {
		t.Error("アクティブファイルのパスが含まれない")
	}
	if !strings.Contains(result, "package main") {
		t.Error("アクティブファイルの内容が含まれない")
	}
}

func TestBuildContextPrompt_選択範囲(t *testing.T) {
	ctx := &Context{
		Selection: &SelectionContext{
			FilePath:  "main.go",
			Language:  "go",
			Text:      "selected code",
			StartLine: 5,
			EndLine:   10,
		},
	}
	result := BuildContextPrompt(ctx)
	if !strings.Contains(result, "selected code") {
		t.Error("選択テキストが含まれない")
	}
	if !strings.Contains(result, "選択されたコード") {
		t.Error("選択範囲ヘッダーが含まれない")
	}
}

func TestBuildContextPrompt_Git差分(t *testing.T) {
	ctx := &Context{
		GitDiff: "+added line\n-removed line",
	}
	result := BuildContextPrompt(ctx)
	if !strings.Contains(result, "Git差分") {
		t.Error("Git差分ヘッダーが含まれない")
	}
	if !strings.Contains(result, "+added line") {
		t.Error("Git差分内容が含まれない")
	}
}

func TestBuildContextPrompt_LSPシンボル(t *testing.T) {
	ctx := &Context{
		Symbols: &SymbolContext{
			HoverInfo:  "func main()",
			CursorLine: 10,
			CursorCol:  5,
			Symbols: []SymbolInfo{
				{Name: "main", Kind: "function", Line: 0},
			},
			Diagnostics: []string{"未使用のインポート"},
		},
	}
	result := BuildContextPrompt(ctx)
	if !strings.Contains(result, "func main()") {
		t.Error("ホバー情報が含まれない")
	}
	if !strings.Contains(result, "未使用のインポート") {
		t.Error("診断情報が含まれない")
	}
}

func TestBuildContextPrompt_参照ファイル最大3(t *testing.T) {
	files := make([]FileContext, 5)
	for i := range files {
		files[i] = FileContext{Path: "file" + string(rune('0'+i)) + ".go", Language: "go", Content: "pkg", Active: false}
	}
	ctx := &Context{OpenFiles: files}
	result := BuildContextPrompt(ctx)
	// 最大3ファイルまで
	count := strings.Count(result, "参照ファイル")
	if count > 3 {
		t.Errorf("参照ファイル数 = %d, 最大 3", count)
	}
}

func TestTruncateContent(t *testing.T) {
	short := "hello"
	if got := truncateContent(short, 100); got != short {
		t.Errorf("短いテキストが変更された: %q", got)
	}

	long := strings.Repeat("a", 200)
	result := truncateContent(long, 50)
	if len(result) > 200 {
		t.Error("切り詰めが機能していない")
	}
	if !strings.Contains(result, "省略") {
		t.Error("切り詰めマーカーが含まれない")
	}
}

func TestActionType定数(t *testing.T) {
	if ActionChat != 0 {
		t.Errorf("ActionChat = %d, 期待 0", ActionChat)
	}
	if ActionRefactor != 1 {
		t.Errorf("ActionRefactor = %d, 期待 1", ActionRefactor)
	}
	if ActionExplain != 2 {
		t.Errorf("ActionExplain = %d, 期待 2", ActionExplain)
	}
	if ActionGenTest != 3 {
		t.Errorf("ActionGenTest = %d, 期待 3", ActionGenTest)
	}
	if ActionGenDoc != 4 {
		t.Errorf("ActionGenDoc = %d, 期待 4", ActionGenDoc)
	}
}
