package editor

import (
	"testing"
)

func TestPosToLineCol(t *testing.T) {
	tests := []struct {
		text     string
		pos      int
		wantLine int
		wantCol  int
	}{
		{"hello\nworld", 0, 0, 0},
		{"hello\nworld", 5, 0, 5},
		{"hello\nworld", 6, 1, 0},
		{"hello\nworld", 11, 1, 5},
		{"a\nb\nc", 2, 1, 0},
		{"a\nb\nc", 4, 2, 0},
		{"", 0, 0, 0},
		{"abc", -1, 0, 0},
	}
	for _, tt := range tests {
		line, col := posToLineCol(tt.text, tt.pos)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("posToLineCol(%q, %d) = (%d,%d), 期待 (%d,%d)",
				tt.text, tt.pos, line, col, tt.wantLine, tt.wantCol)
		}
	}
}

func TestPosToLineCol_UTF8(t *testing.T) {
	text := "日本語\nテスト"
	// "日本語" = 9バイト, 改行 = 1バイト → "テスト" は10バイト目から
	line, col := posToLineCol(text, 10)
	if line != 1 || col != 0 {
		t.Errorf("posToLineCol(日本語テスト, 10) = (%d,%d), 期待 (1,0)", line, col)
	}
}

func TestSearchState_PlainSearch(t *testing.T) {
	ss := NewSearchState()
	ss.Query = "hello"
	text := "hello world\nhello again\n"
	lines := []string{"hello world", "hello again", ""}
	lineCache := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}

	ss.SearchInBuffer(text, lineCache, len(lines))

	if len(ss.Matches) != 2 {
		t.Fatalf("マッチ数 = %d, 期待 2", len(ss.Matches))
	}
	if ss.Matches[0].Line != 0 || ss.Matches[0].Col != 0 {
		t.Errorf("1個目のマッチ位置 = (%d,%d), 期待 (0,0)", ss.Matches[0].Line, ss.Matches[0].Col)
	}
	if ss.Matches[1].Line != 1 || ss.Matches[1].Col != 0 {
		t.Errorf("2個目のマッチ位置 = (%d,%d), 期待 (1,0)", ss.Matches[1].Line, ss.Matches[1].Col)
	}
}

func TestSearchState_CaseInsensitive(t *testing.T) {
	ss := NewSearchState()
	ss.Query = "Hello"
	ss.CaseSensitive = false
	text := "HELLO world\nhello again\n"
	lines := []string{"HELLO world", "hello again", ""}
	lineCache := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}

	ss.SearchInBuffer(text, lineCache, len(lines))

	if len(ss.Matches) != 2 {
		t.Fatalf("大文字小文字無視のマッチ数 = %d, 期待 2", len(ss.Matches))
	}
}

func TestSearchState_CaseSensitive(t *testing.T) {
	ss := NewSearchState()
	ss.Query = "Hello"
	ss.CaseSensitive = true
	text := "HELLO world\nHello again\n"
	lines := []string{"HELLO world", "Hello again", ""}
	lineCache := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}

	ss.SearchInBuffer(text, lineCache, len(lines))

	if len(ss.Matches) != 1 {
		t.Fatalf("大文字小文字区別のマッチ数 = %d, 期待 1", len(ss.Matches))
	}
	if ss.Matches[0].Line != 1 {
		t.Errorf("マッチ行 = %d, 期待 1", ss.Matches[0].Line)
	}
}

func TestSearchState_Regex(t *testing.T) {
	ss := NewSearchState()
	ss.Query = `\d+`
	ss.IsRegex = true
	text := "line 123\nline 456\n"
	lines := []string{"line 123", "line 456", ""}
	lineCache := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}

	ss.SearchInBuffer(text, lineCache, len(lines))

	if len(ss.Matches) != 2 {
		t.Fatalf("正規表現マッチ数 = %d, 期待 2", len(ss.Matches))
	}
	if ss.Matches[0].Length != 3 {
		t.Errorf("1個目のマッチ長 = %d, 期待 3", ss.Matches[0].Length)
	}
}

func TestSearchState_EmptyQuery(t *testing.T) {
	ss := NewSearchState()
	ss.Query = ""
	text := "hello world"
	ss.SearchInBuffer(text, func(i int) string { return text }, 1)

	if len(ss.Matches) != 0 {
		t.Fatalf("空クエリのマッチ数 = %d, 期待 0", len(ss.Matches))
	}
}

func TestSearchState_NextPrevMatch(t *testing.T) {
	ss := NewSearchState()
	ss.Matches = []SearchMatch{
		{Line: 0, Col: 0},
		{Line: 1, Col: 0},
		{Line: 2, Col: 0},
	}
	ss.CurrentMatch = 0

	ss.NextMatch()
	if ss.CurrentMatch != 1 {
		t.Fatalf("NextMatch: CurrentMatch = %d, 期待 1", ss.CurrentMatch)
	}

	ss.NextMatch()
	if ss.CurrentMatch != 2 {
		t.Fatalf("NextMatch: CurrentMatch = %d, 期待 2", ss.CurrentMatch)
	}

	// 末尾から先頭へラップ
	ss.NextMatch()
	if ss.CurrentMatch != 0 {
		t.Fatalf("NextMatch wrap: CurrentMatch = %d, 期待 0", ss.CurrentMatch)
	}

	// PrevMatchで末尾へ
	ss.PrevMatch()
	if ss.CurrentMatch != 2 {
		t.Fatalf("PrevMatch wrap: CurrentMatch = %d, 期待 2", ss.CurrentMatch)
	}

	ss.PrevMatch()
	if ss.CurrentMatch != 1 {
		t.Fatalf("PrevMatch: CurrentMatch = %d, 期待 1", ss.CurrentMatch)
	}
}

func TestSearchState_NextPrevMatch_空(t *testing.T) {
	ss := NewSearchState()
	// マッチ0件でもパニックしないこと
	ss.NextMatch()
	ss.PrevMatch()
}

func TestSearchState_CurrentMatchInfo(t *testing.T) {
	ss := NewSearchState()
	if ss.CurrentMatchInfo() != nil {
		t.Fatal("マッチ無しで CurrentMatchInfo が nil でない")
	}

	ss.Matches = []SearchMatch{{Line: 5, Col: 3}}
	ss.CurrentMatch = 0
	info := ss.CurrentMatchInfo()
	if info == nil {
		t.Fatal("CurrentMatchInfo が nil")
	}
	if info.Line != 5 || info.Col != 3 {
		t.Errorf("CurrentMatchInfo = (%d,%d), 期待 (5,3)", info.Line, info.Col)
	}
}

func TestSearchState_InvalidRegex(t *testing.T) {
	ss := NewSearchState()
	ss.Query = "[invalid"
	ss.IsRegex = true
	text := "some text"
	ss.SearchInBuffer(text, func(i int) string { return text }, 1)

	// 不正な正規表現でもパニックせずマッチ0件
	if len(ss.Matches) != 0 {
		t.Fatalf("不正な正規表現のマッチ数 = %d, 期待 0", len(ss.Matches))
	}
}
