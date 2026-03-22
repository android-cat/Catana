package core

import (
	"testing"
	"unicode/utf8"
)

func TestNewRope(t *testing.T) {
	r := NewRope("Hello, World!")
	if r.Length() != 13 {
		t.Errorf("Length() = %d, want 13", r.Length())
	}
	if r.String() != "Hello, World!" {
		t.Errorf("String() = %q, want %q", r.String(), "Hello, World!")
	}
}

func TestEmptyRope(t *testing.T) {
	r := NewRope("")
	if r.Length() != 0 {
		t.Errorf("Length() = %d, want 0", r.Length())
	}
	if r.String() != "" {
		t.Errorf("String() = %q, want empty", r.String())
	}
}

func TestInsert(t *testing.T) {
	r := NewRope("Hello World")
	r.Insert(5, ",")
	if r.String() != "Hello, World" {
		t.Errorf("Insert: got %q, want %q", r.String(), "Hello, World")
	}

	// 先頭に挿入
	r.Insert(0, ">> ")
	if r.String() != ">> Hello, World" {
		t.Errorf("Insert at start: got %q", r.String())
	}

	// 末尾に挿入
	r.Insert(r.Length(), "!")
	if r.String() != ">> Hello, World!" {
		t.Errorf("Insert at end: got %q", r.String())
	}
}

func TestDelete(t *testing.T) {
	r := NewRope("Hello, World!")
	r.Delete(5, 7)
	if r.String() != "HelloWorld!" {
		t.Errorf("Delete: got %q, want %q", r.String(), "HelloWorld!")
	}

	// 先頭から削除
	r.Delete(0, 5)
	if r.String() != "World!" {
		t.Errorf("Delete from start: got %q", r.String())
	}
}

func TestLineCount(t *testing.T) {
	r := NewRope("line1\nline2\nline3")
	if r.LineCount() != 3 {
		t.Errorf("LineCount() = %d, want 3", r.LineCount())
	}

	r2 := NewRope("single line")
	if r2.LineCount() != 1 {
		t.Errorf("LineCount() = %d, want 1", r2.LineCount())
	}
}

func TestLine(t *testing.T) {
	r := NewRope("line1\nline2\nline3")
	tests := []struct {
		n    int
		want string
	}{
		{0, "line1"},
		{1, "line2"},
		{2, "line3"},
	}
	for _, tc := range tests {
		got := r.Line(tc.n)
		if got != tc.want {
			t.Errorf("Line(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestLineStart(t *testing.T) {
	r := NewRope("abc\ndef\nghi")
	if r.LineStart(0) != 0 {
		t.Errorf("LineStart(0) = %d, want 0", r.LineStart(0))
	}
	if r.LineStart(1) != 4 {
		t.Errorf("LineStart(1) = %d, want 4", r.LineStart(1))
	}
	if r.LineStart(2) != 8 {
		t.Errorf("LineStart(2) = %d, want 8", r.LineStart(2))
	}
}

func TestPosToLineCol(t *testing.T) {
	r := NewRope("abc\ndef\nghi")
	line, col := r.PosToLineCol(5)
	if line != 1 || col != 1 {
		t.Errorf("PosToLineCol(5) = (%d, %d), want (1, 1)", line, col)
	}
}

func TestLineColToPos(t *testing.T) {
	r := NewRope("abc\ndef\nghi")
	pos := r.LineColToPos(1, 1)
	if pos != 5 {
		t.Errorf("LineColToPos(1, 1) = %d, want 5", pos)
	}
}

func TestPosToLineColUTF8(t *testing.T) {
	r := NewRope("a界\nうえ")
	line, col := r.PosToLineCol(len("a界"))
	if line != 0 || col != 2 {
		t.Fatalf("PosToLineCol(len(\"a界\")) = (%d, %d), want (0, 2)", line, col)
	}

	line, col = r.PosToLineCol(len("a界\nう"))
	if line != 1 || col != 1 {
		t.Fatalf("PosToLineCol(len(\"a界\\nう\")) = (%d, %d), want (1, 1)", line, col)
	}
}

func TestLineColToPosUTF8(t *testing.T) {
	r := NewRope("a界\nうえ")
	pos := r.LineColToPos(0, 2)
	if pos != len("a界") {
		t.Fatalf("LineColToPos(0, 2) = %d, want %d", pos, len("a界"))
	}

	pos = r.LineColToPos(1, 1)
	if pos != len("a界\nう") {
		t.Fatalf("LineColToPos(1, 1) = %d, want %d", pos, len("a界\nう"))
	}

	pos = r.LineColToPos(1, 99)
	if pos != len("a界\nうえ") {
		t.Fatalf("LineColToPos(1, 99) = %d, want %d", pos, len("a界\nうえ"))
	}

	if got := utf8.RuneCountInString(r.Line(1)); got != 2 {
		t.Fatalf("RuneCountInString(Line(1)) = %d, want 2", got)
	}
}

func TestLargeText(t *testing.T) {
	// 大きなテキストでの動作を検証
	text := ""
	for i := 0; i < 1000; i++ {
		text += "This is a test line for rope data structure.\n"
	}
	r := NewRope(text)
	if r.String() != text {
		t.Error("Large text: String() does not match original")
	}
	if r.LineCount() != 1001 {
		t.Errorf("Large text: LineCount() = %d, want 1001", r.LineCount())
	}

	// 中央に挿入
	mid := r.Length() / 2
	r.Insert(mid, "INSERTED")
	if r.Length() != len(text)+8 {
		t.Errorf("Large text insert: Length() = %d, want %d", r.Length(), len(text)+8)
	}
}
