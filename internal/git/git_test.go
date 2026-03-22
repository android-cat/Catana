package git

import (
	"fmt"
	"testing"
	"time"
)

func TestParseStatusOutput_基本(t *testing.T) {
	output := " M modified.go\n?? untracked.go\nA  staged_new.go\nD  deleted.go\n"
	entries := parseStatusOutput(output)

	if len(entries) != 4 {
		t.Fatalf("エントリ数 = %d, 期待 4", len(entries))
	}

	if entries[0].Path != "modified.go" || entries[0].Status != StatusModified {
		t.Errorf("1件目: Path=%q, Status=%d", entries[0].Path, entries[0].Status)
	}
	if entries[1].Path != "untracked.go" || entries[1].Status != StatusUntracked {
		t.Errorf("2件目: Path=%q, Status=%d", entries[1].Path, entries[1].Status)
	}
	if entries[2].Path != "staged_new.go" || entries[2].Staged != StatusStagedNew {
		t.Errorf("3件目: Path=%q, Staged=%d", entries[2].Path, entries[2].Staged)
	}
	if entries[3].Path != "deleted.go" || entries[3].Staged != StatusStagedDeleted {
		t.Errorf("4件目: Path=%q, Staged=%d", entries[3].Path, entries[3].Staged)
	}
}

func TestParseStatusOutput_空出力(t *testing.T) {
	entries := parseStatusOutput("")
	if entries != nil {
		t.Fatalf("空出力でエントリが返された: %v", entries)
	}
}

func TestParseStatusOutput_リネーム(t *testing.T) {
	output := "R  old.go -> new.go\n"
	entries := parseStatusOutput(output)
	if len(entries) != 1 {
		t.Fatalf("エントリ数 = %d, 期待 1", len(entries))
	}
	if entries[0].Path != "new.go" {
		t.Errorf("Path = %q, 期待 %q", entries[0].Path, "new.go")
	}
	if entries[0].OldPath != "old.go" {
		t.Errorf("OldPath = %q, 期待 %q", entries[0].OldPath, "old.go")
	}
}

func TestParseStatusOutput_コンフリクト(t *testing.T) {
	output := "UU conflict.go\n"
	entries := parseStatusOutput(output)
	if len(entries) != 1 {
		t.Fatalf("エントリ数 = %d, 期待 1", len(entries))
	}
	if !entries[0].IsConflict {
		t.Error("IsConflict = false, 期待 true")
	}
	if entries[0].Status != StatusConflicted {
		t.Errorf("Status = %d, 期待 %d", entries[0].Status, StatusConflicted)
	}
}

func TestCharToStatus(t *testing.T) {
	tests := []struct {
		c      byte
		staged bool
		want   FileStatus
	}{
		{'M', false, StatusModified},
		{'M', true, StatusStaged},
		{'A', false, StatusAdded},
		{'A', true, StatusStagedNew},
		{'D', false, StatusDeleted},
		{'D', true, StatusStagedDeleted},
		{'R', false, StatusRenamed},
		{'R', true, StatusRenamed},
		{'?', false, StatusUntracked},
		{'U', false, StatusConflicted},
		{' ', false, StatusUnmodified},
	}
	for _, tt := range tests {
		got := charToStatus(tt.c, tt.staged)
		if got != tt.want {
			t.Errorf("charToStatus(%q, %v) = %d, 期待 %d", tt.c, tt.staged, got, tt.want)
		}
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input     string
		wantStart int
		wantCount int
	}{
		{"1,5", 1, 5},
		{"10,20", 10, 20},
		{"5", 5, 1},
	}
	for _, tt := range tests {
		start, count := parseRange(tt.input)
		if start != tt.wantStart || count != tt.wantCount {
			t.Errorf("parseRange(%q) = (%d,%d), 期待 (%d,%d)",
				tt.input, start, count, tt.wantStart, tt.wantCount)
		}
	}
}

func TestGitParseHunkHeader(t *testing.T) {
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
		if hunk == nil {
			t.Fatalf("parseHunkHeader(%q) が nil を返した", tt.line)
		}
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

func TestGitParseHunkHeader_不正入力(t *testing.T) {
	hunk := parseHunkHeader("not a hunk header")
	if hunk != nil {
		t.Error("不正な入力で nil が返されなかった")
	}
}

func TestParseLogOutput(t *testing.T) {
	output := "abc1234\x00John\x002024-01-15T10:30:00+09:00\x00初回コミット\ndef5678\x00Jane\x002024-01-16T12:00:00+09:00\x00機能追加\n"
	entries := parseLogOutput(output)

	if len(entries) != 2 {
		t.Fatalf("エントリ数 = %d, 期待 2", len(entries))
	}
	if entries[0].Hash != "abc1234" {
		t.Errorf("1件目 Hash = %q, 期待 %q", entries[0].Hash, "abc1234")
	}
	if entries[0].Author != "John" {
		t.Errorf("1件目 Author = %q, 期待 %q", entries[0].Author, "John")
	}
	if entries[0].Message != "初回コミット" {
		t.Errorf("1件目 Message = %q, 期待 %q", entries[0].Message, "初回コミット")
	}
	if entries[0].Date.Year() != 2024 {
		t.Errorf("1件目の年 = %d, 期待 2024", entries[0].Date.Year())
	}
}

func TestParseLogOutput_空出力(t *testing.T) {
	entries := parseLogOutput("")
	if entries != nil {
		t.Fatalf("空出力でエントリが返された: %v", entries)
	}
}

func TestParseStashOutput(t *testing.T) {
	output := "stash@{0}\x00WIP on main: abc1234 some work\nstash@{1}\x00feature backup\n"
	entries := parseStashOutput(output)

	if len(entries) != 2 {
		t.Fatalf("エントリ数 = %d, 期待 2", len(entries))
	}
	if entries[0].Index != 0 {
		t.Errorf("1件目 Index = %d, 期待 0", entries[0].Index)
	}
	if entries[0].Message != "WIP on main: abc1234 some work" {
		t.Errorf("1件目 Message = %q", entries[0].Message)
	}
	if entries[1].Index != 1 {
		t.Errorf("2件目 Index = %d, 期待 1", entries[1].Index)
	}
}

func TestParseStashOutput_空出力(t *testing.T) {
	entries := parseStashOutput("")
	if entries != nil {
		t.Fatalf("空出力でエントリが返された: %v", entries)
	}
}

func TestIsHex(t *testing.T) {
	hexChars := "0123456789abcdefABCDEF"
	for _, c := range hexChars {
		if !isHex(byte(c)) {
			t.Errorf("isHex(%q) = false, 期待 true", c)
		}
	}
	nonHexChars := "ghijGHIJ!@#"
	for _, c := range nonHexChars {
		if isHex(byte(c)) {
			t.Errorf("isHex(%q) = true, 期待 false", c)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status FileStatus
		want   string
	}{
		{StatusModified, "M"},
		{StatusAdded, "A"},
		{StatusStagedNew, "A"},
		{StatusDeleted, "D"},
		{StatusStagedDeleted, "D"},
		{StatusRenamed, "R"},
		{StatusUntracked, "U"},
		{StatusConflicted, "C"},
		{StatusStaged, "M"},
		{StatusUnmodified, ""},
	}
	for _, tt := range tests {
		got := StatusLabel(tt.status)
		if got != tt.want {
			t.Errorf("StatusLabel(%d) = %q, 期待 %q", tt.status, got, tt.want)
		}
	}
}

func TestFileEntry_FullPath(t *testing.T) {
	entry := FileEntry{Path: "src/main.go"}
	got := entry.FullPath("/workspace")
	if got != "/workspace/src/main.go" {
		t.Errorf("FullPath = %q, 期待 %q", got, "/workspace/src/main.go")
	}
}

func TestParseDiffOutput_基本(t *testing.T) {
	output := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n package main\n+import \"fmt\"\n func main() {\n }\n"
	diffs := parseDiffOutput(output)
	if len(diffs) != 1 {
		t.Fatalf("diff数 = %d, 期待 1", len(diffs))
	}
	if diffs[0].Path != "main.go" {
		t.Errorf("Path = %q, 期待 %q", diffs[0].Path, "main.go")
	}
	if len(diffs[0].Hunks) != 1 {
		t.Fatalf("ハンク数 = %d, 期待 1", len(diffs[0].Hunks))
	}

	hasAdded := false
	for _, line := range diffs[0].Hunks[0].Lines {
		if line.Type == DiffAdded {
			hasAdded = true
			break
		}
	}
	if !hasAdded {
		t.Error("追加行が見つからない")
	}
}

func TestParseDiffOutput_空出力(t *testing.T) {
	diffs := parseDiffOutput("")
	if diffs != nil {
		t.Fatalf("空出力でdiffが返された: %v", diffs)
	}
}

func TestParseBlameOutput(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	output := fmt.Sprintf("abc1234567890123456789012345678901234567890 1 1 1\nauthor John\nauthor-mail <john@example.com>\nauthor-time %d\nauthor-tz +0000\ncommitter John\n\tHello World\n", ts)
	lines := parseBlameOutput(output)
	if len(lines) != 1 {
		t.Fatalf("行数 = %d, 期待 1", len(lines))
	}
	if lines[0].Content != "Hello World" {
		t.Errorf("Content = %q, 期待 %q", lines[0].Content, "Hello World")
	}
	if lines[0].Author != "John" {
		t.Errorf("Author = %q, 期待 %q", lines[0].Author, "John")
	}
}

func TestParseBlameOutput_空出力(t *testing.T) {
	lines := parseBlameOutput("")
	if lines != nil {
		t.Fatalf("空出力で行が返された: %v", lines)
	}
}
