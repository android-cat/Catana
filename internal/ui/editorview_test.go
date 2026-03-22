package ui

import "testing"

func TestExpandTabs(t *testing.T) {
	expanded, width := expandTabs("\tfoo", 0)
	if expanded != "    foo" {
		t.Fatalf("expandTabs() = %q, want %q", expanded, "    foo")
	}
	if width != 7 {
		t.Fatalf("expandTabs() width = %d, want 7", width)
	}
}

func TestDisplayColsForPrefix(t *testing.T) {
	if got := displayColsForPrefix("\tfoo", 1, 0); got != 4 {
		t.Fatalf("displayColsForPrefix(tab, 1) = %d, want 4", got)
	}
	if got := displayColsForPrefix("\tfoo", 2, 0); got != 5 {
		t.Fatalf("displayColsForPrefix(tab, 2) = %d, want 5", got)
	}
	if got := displayColsForPrefix("あa", 1, 0); got != 2 {
		t.Fatalf("displayColsForPrefix(fullwidth, 1) = %d, want 2", got)
	}
	if got := displayColsForPrefix("あa", 2, 0); got != 3 {
		t.Fatalf("displayColsForPrefix(mixed, 2) = %d, want 3", got)
	}
}

func TestColFromPixelX(t *testing.T) {
	ev := &EditorView{charWidthF: 10, fullWidthCharF: 20}

	// ASCII "abc" — 各文字は表示幅1 → 10px
	if got := ev.colFromPixelX("abc", 4, 0); got != 0 {
		t.Fatalf("colFromPixelX(ascii start) = %d, want 0", got)
	}
	if got := ev.colFromPixelX("abc", 14, 0); got != 1 {
		t.Fatalf("colFromPixelX(ascii second) = %d, want 1", got)
	}

	// 全角 "あa" — 'あ'は全角幅20px, 'a'は半角幅10px
	if got := ev.colFromPixelX("あa", 9, 0); got != 0 {
		t.Fatalf("colFromPixelX(before fullwidth midpoint) = %d, want 0", got)
	}
	if got := ev.colFromPixelX("あa", 11, 0); got != 1 {
		t.Fatalf("colFromPixelX(after fullwidth midpoint) = %d, want 1", got)
	}
	if got := ev.colFromPixelX("あa", 26, 0); got != 2 {
		t.Fatalf("colFromPixelX(end mixed) = %d, want 2", got)
	}
}

func TestPixelXForPrefix(t *testing.T) {
	ev := &EditorView{charWidthF: 10, fullWidthCharF: 18}

	// ASCII のみ: 3文字 = 30px
	if got := ev.pixelXForPrefix("abc", 3, 0); got != 30 {
		t.Fatalf("pixelXForPrefix(ascii 3) = %d, want 30", got)
	}

	// 全角1文字 = 18px（fullWidthCharF）
	if got := ev.pixelXForPrefix("あa", 1, 0); got != 18 {
		t.Fatalf("pixelXForPrefix(fullwidth 1) = %d, want 18", got)
	}

	// 全角1文字 + 半角1文字 = 18 + 10 = 28px
	if got := ev.pixelXForPrefix("あa", 2, 0); got != 28 {
		t.Fatalf("pixelXForPrefix(mixed 2) = %d, want 28", got)
	}
}
