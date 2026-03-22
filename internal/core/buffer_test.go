package core

import "testing"

func TestBufferSetCursorLineColUTF8(t *testing.T) {
	b := NewBuffer("a界\nうえ")
	b.SetCursorLineCol(0, 2)
	if got := b.CursorPos(); got != len("a界") {
		t.Fatalf("CursorPos() = %d, want %d", got, len("a界"))
	}
	if got := b.CursorCol(); got != 2 {
		t.Fatalf("CursorCol() = %d, want 2", got)
	}

	b.SetCursorLineCol(1, 1)
	if got := b.CursorPos(); got != len("a界\nう") {
		t.Fatalf("CursorPos() = %d, want %d", got, len("a界\nう"))
	}
	if got := b.CursorCol(); got != 1 {
		t.Fatalf("CursorCol() = %d, want 1", got)
	}
}

func TestBufferMoveAndDeleteRunes(t *testing.T) {
	b := NewBuffer("a界b")
	b.SetCursorPos(len("a界"))
	b.MoveCursorLeft()
	if got := b.CursorPos(); got != len("a") {
		t.Fatalf("MoveCursorLeft() => %d, want %d", got, len("a"))
	}

	b.MoveCursorRight()
	if got := b.CursorPos(); got != len("a界") {
		t.Fatalf("MoveCursorRight() => %d, want %d", got, len("a界"))
	}

	b.DeleteBackward()
	if got := b.Text(); got != "ab" {
		t.Fatalf("DeleteBackward() => %q, want %q", got, "ab")
	}
	if got := b.CursorPos(); got != len("a") {
		t.Fatalf("CursorPos() after DeleteBackward = %d, want %d", got, len("a"))
	}

	b = NewBuffer("界b")
	b.DeleteForward()
	if got := b.Text(); got != "b" {
		t.Fatalf("DeleteForward() => %q, want %q", got, "b")
	}
	if got := b.CursorPos(); got != 0 {
		t.Fatalf("CursorPos() after DeleteForward = %d, want 0", got)
	}
}
