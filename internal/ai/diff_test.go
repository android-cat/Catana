package ai

import (
	"strings"
	"testing"
)

func TestExtractDiffBlocks_単一ブロック(t *testing.T) {
	text := "変更内容:\n```diff\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n package main\n+import \"fmt\"\n func main() {\n }\n```\n"

	blocks := ExtractDiffBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("ブロック数 = %d, 期待 1", len(blocks))
	}
	if blocks[0].FilePath != "main.go" {
		t.Errorf("FilePath = %q, 期待 %q", blocks[0].FilePath, "main.go")
	}
	if len(blocks[0].Hunks) != 1 {
		t.Fatalf("ハンク数 = %d, 期待 1", len(blocks[0].Hunks))
	}
}

func TestExtractDiffBlocks_複数ブロック(t *testing.T) {
	text := "```diff\n--- a/file1.go\n+++ b/file1.go\n@@ -1,2 +1,2 @@\n-old\n+new\n```\nその他の説明\n```diff\n--- a/file2.go\n+++ b/file2.go\n@@ -1,2 +1,2 @@\n-old2\n+new2\n```\n"

	blocks := ExtractDiffBlocks(text)
	if len(blocks) != 2 {
		t.Fatalf("ブロック数 = %d, 期待 2", len(blocks))
	}
	if blocks[0].FilePath != "file1.go" {
		t.Errorf("1つ目のFilePath = %q, 期待 %q", blocks[0].FilePath, "file1.go")
	}
	if blocks[1].FilePath != "file2.go" {
		t.Errorf("2つ目のFilePath = %q, 期待 %q", blocks[1].FilePath, "file2.go")
	}
}

func TestExtractDiffBlocks_diffブロックなし(t *testing.T) {
	text := "これはdiffブロックを含みません。\n普通のテキストです。\n"
	blocks := ExtractDiffBlocks(text)
	if len(blocks) != 0 {
		t.Fatalf("ブロック数 = %d, 期待 0", len(blocks))
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line     string
		oldStart int
		oldCount int
		newStart int
		newCount int
	}{
		{"@@ -1,5 +1,7 @@", 1, 5, 1, 7},
		{"@@ -10,3 +12,5 @@ func main()", 10, 3, 12, 5},
		{"@@ -1 +1 @@", 1, 1, 1, 1},
	}
	for _, tt := range tests {
		hunk := parseHunkHeader(tt.line)
		if hunk.OldStart != tt.oldStart || hunk.OldCount != tt.oldCount {
			t.Errorf("parseHunkHeader(%q) old = (%d,%d), 期待 (%d,%d)",
				tt.line, hunk.OldStart, hunk.OldCount, tt.oldStart, tt.oldCount)
		}
		if hunk.NewStart != tt.newStart || hunk.NewCount != tt.newCount {
			t.Errorf("parseHunkHeader(%q) new = (%d,%d), 期待 (%d,%d)",
				tt.line, hunk.NewStart, hunk.NewCount, tt.newStart, tt.newCount)
		}
	}
}

func TestParseLineRange(t *testing.T) {
	tests := []struct {
		input     string
		wantStart int
		wantCount int
	}{
		{"1,5", 1, 5},
		{"10,3", 10, 3},
		{"5", 5, 1},
		{"0,3", 1, 3}, // 0は1に補正
	}
	for _, tt := range tests {
		start, count := parseLineRange(tt.input)
		if start != tt.wantStart || count != tt.wantCount {
			t.Errorf("parseLineRange(%q) = (%d,%d), 期待 (%d,%d)",
				tt.input, start, count, tt.wantStart, tt.wantCount)
		}
	}
}

func TestParseDiffBlock_ヘッダー付き(t *testing.T) {
	lines := []string{
		"--- a/hello.go",
		"+++ b/hello.go",
		"@@ -1,3 +1,4 @@",
		" package main",
		"+import \"fmt\"",
		" func main() {",
		" }",
	}
	block := parseDiffBlock(lines)
	if block.FilePath != "hello.go" {
		t.Errorf("FilePath = %q, 期待 %q", block.FilePath, "hello.go")
	}
	if len(block.Hunks) != 1 {
		t.Fatalf("ハンク数 = %d, 期待 1", len(block.Hunks))
	}
	hunk := block.Hunks[0]
	if hunk.OldStart != 1 || hunk.OldCount != 3 {
		t.Errorf("Old = (%d,%d), 期待 (1,3)", hunk.OldStart, hunk.OldCount)
	}

	// 行の内訳を確認
	added := 0
	context := 0
	for _, l := range hunk.Lines {
		switch l.Type {
		case DiffAdded:
			added++
		case DiffContext:
			context++
		}
	}
	if added != 1 {
		t.Errorf("追加行数 = %d, 期待 1", added)
	}
	if context != 3 {
		t.Errorf("コンテキスト行数 = %d, 期待 3", context)
	}
}

func TestParseDiffBlock_ヘッダーなし(t *testing.T) {
	// @@なしで直接差分行が始まるパターン
	lines := []string{
		"-old line",
		"+new line",
		" context",
	}
	block := parseDiffBlock(lines)
	if len(block.Hunks) != 1 {
		t.Fatalf("ハンク数 = %d, 期待 1", len(block.Hunks))
	}
	if len(block.Hunks[0].Lines) != 3 {
		t.Fatalf("行数 = %d, 期待 3", len(block.Hunks[0].Lines))
	}
}

func TestApplyDiffToText_基本追加(t *testing.T) {
	original := "line1\nline2\nline3"
	diff := &DiffBlock{
		Hunks: []DiffHunk{
			{
				OldStart: 1,
				OldCount: 3,
				NewStart: 1,
				NewCount: 4,
				Lines: []DiffLine{
					{Type: DiffContext, Content: "line1"},
					{Type: DiffAdded, Content: "inserted"},
					{Type: DiffContext, Content: "line2"},
					{Type: DiffContext, Content: "line3"},
				},
			},
		},
	}

	result, err := ApplyDiffToText(original, diff)
	if err != nil {
		t.Fatalf("ApplyDiffToText エラー: %v", err)
	}
	if !strings.Contains(result, "inserted") {
		t.Errorf("結果に 'inserted' が含まれない: %q", result)
	}
}

func TestApplyDiffToText_削除(t *testing.T) {
	original := "line1\nline2\nline3"
	diff := &DiffBlock{
		Hunks: []DiffHunk{
			{
				OldStart: 1,
				OldCount: 3,
				NewStart: 1,
				NewCount: 2,
				Lines: []DiffLine{
					{Type: DiffContext, Content: "line1"},
					{Type: DiffDeleted, Content: "line2"},
					{Type: DiffContext, Content: "line3"},
				},
			},
		},
	}

	result, err := ApplyDiffToText(original, diff)
	if err != nil {
		t.Fatalf("ApplyDiffToText エラー: %v", err)
	}
	if strings.Contains(result, "line2") {
		t.Errorf("結果に削除行 'line2' が残っている: %q", result)
	}
}

func TestApplyDiffToText_置換(t *testing.T) {
	original := "line1\nold\nline3"
	diff := &DiffBlock{
		Hunks: []DiffHunk{
			{
				OldStart: 1,
				OldCount: 3,
				NewStart: 1,
				NewCount: 3,
				Lines: []DiffLine{
					{Type: DiffContext, Content: "line1"},
					{Type: DiffDeleted, Content: "old"},
					{Type: DiffAdded, Content: "new"},
					{Type: DiffContext, Content: "line3"},
				},
			},
		},
	}

	result, err := ApplyDiffToText(original, diff)
	if err != nil {
		t.Fatalf("ApplyDiffToText エラー: %v", err)
	}
	if strings.Contains(result, "old") {
		t.Errorf("結果に 'old' が残っている: %q", result)
	}
	if !strings.Contains(result, "new") {
		t.Errorf("結果に 'new' が含まれない: %q", result)
	}
}

func TestMatchAt(t *testing.T) {
	lines := []string{"line1", "line2", "line3", "line4"}

	if !matchAt(lines, 0, []string{"line1", "line2"}) {
		t.Error("matchAt(0, [line1,line2]) = false, 期待 true")
	}
	if matchAt(lines, 0, []string{"line1", "line3"}) {
		t.Error("matchAt(0, [line1,line3]) = true, 期待 false")
	}
	// 範囲外
	if matchAt(lines, 3, []string{"line4", "line5"}) {
		t.Error("matchAt(3, [line4,line5]) = true, 期待 false (範囲外)")
	}
}

func TestMatchAt_末尾空白無視(t *testing.T) {
	lines := []string{"line1  ", "line2\t"}
	if !matchAt(lines, 0, []string{"line1", "line2"}) {
		t.Error("末尾空白を無視したマッチが失敗した")
	}
}

func TestDiffLineType定数(t *testing.T) {
	// 定数値の確認
	if DiffContext != 0 {
		t.Errorf("DiffContext = %d, 期待 0", DiffContext)
	}
	if DiffAdded != 1 {
		t.Errorf("DiffAdded = %d, 期待 1", DiffAdded)
	}
	if DiffDeleted != 2 {
		t.Errorf("DiffDeleted = %d, 期待 2", DiffDeleted)
	}
}
