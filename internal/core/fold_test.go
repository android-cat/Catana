package core

import (
	"strings"
	"testing"
)

func TestNewFoldState(t *testing.T) {
	fs := NewFoldState()
	if fs == nil {
		t.Fatal("NewFoldState() が nil を返した")
	}
	if len(fs.Regions) != 0 {
		t.Fatalf("初期状態の Regions 長 = %d, 期待 0", len(fs.Regions))
	}
}

func TestDetectFoldRegions_インデントベース(t *testing.T) {
	// インデントで折畳可能領域が検出されることを確認
	text := "func main() {\n    line1\n    line2\n}\n"
	lines := strings.Split(text, "\n")
	fs := NewFoldState()
	fs.DetectFoldRegions(len(lines), func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	})

	if len(fs.Regions) == 0 {
		t.Fatal("折畳領域が検出されなかった")
	}
	// 0行目 ("func main() {") が折畳開始行であること
	if fs.Regions[0].StartLine != 0 {
		t.Fatalf("StartLine = %d, 期待 0", fs.Regions[0].StartLine)
	}
}

func TestDetectFoldRegions_空テキスト(t *testing.T) {
	fs := NewFoldState()
	fs.DetectFoldRegions(0, func(i int) string { return "" })
	if len(fs.Regions) != 0 {
		t.Fatalf("空テキストで Regions 長 = %d, 期待 0", len(fs.Regions))
	}
}

func TestDetectFoldRegions_1行(t *testing.T) {
	fs := NewFoldState()
	fs.DetectFoldRegions(1, func(i int) string { return "hello" })
	if len(fs.Regions) != 0 {
		t.Fatalf("1行テキストで Regions 長 = %d, 期待 0", len(fs.Regions))
	}
}

func TestDetectFoldRegions_折畳状態保持(t *testing.T) {
	text := "func main() {\n    line1\n    line2\n}\n"
	lines := strings.Split(text, "\n")
	lineFunc := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}

	fs := NewFoldState()
	fs.DetectFoldRegions(len(lines), lineFunc)
	// 折畳を有効にする
	fs.ToggleFold(0)
	if !fs.IsFolded(0) {
		t.Fatal("ToggleFold後に IsFolded(0) = false")
	}

	// 再検出しても折畳状態が保持されること
	fs.DetectFoldRegions(len(lines), lineFunc)
	if !fs.IsFolded(0) {
		t.Fatal("再検出後に折畳状態が失われた")
	}
}

func TestToggleFold(t *testing.T) {
	text := "if true {\n    x\n}\n"
	lines := strings.Split(text, "\n")
	fs := NewFoldState()
	fs.DetectFoldRegions(len(lines), func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	})

	if len(fs.Regions) == 0 {
		t.Fatal("折畳領域が検出されなかった")
	}

	start := fs.Regions[0].StartLine
	if fs.IsFolded(start) {
		t.Fatal("折畳前に IsFolded が true")
	}

	fs.ToggleFold(start)
	if !fs.IsFolded(start) {
		t.Fatal("ToggleFold後に IsFolded が false")
	}

	fs.ToggleFold(start)
	if fs.IsFolded(start) {
		t.Fatal("再度ToggleFold後に IsFolded が true")
	}
}

func TestIsFoldable(t *testing.T) {
	text := "func main() {\n    x\n}\nnot foldable\n"
	lines := strings.Split(text, "\n")
	fs := NewFoldState()
	fs.DetectFoldRegions(len(lines), func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	})

	if !fs.IsFoldable(0) {
		t.Fatal("行0が折畳可能でない")
	}
	// "not foldable" は折畳不可
	if fs.IsFoldable(3) {
		t.Fatal("行3が折畳可能と判定された")
	}
}

func TestIsHidden(t *testing.T) {
	text := "func main() {\n    line1\n    line2\n}\n"
	lines := strings.Split(text, "\n")
	fs := NewFoldState()
	fs.DetectFoldRegions(len(lines), func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	})

	fs.ToggleFold(0)

	// 開始行(0)は非表示ではない
	if fs.IsHidden(0) {
		t.Fatal("折畳の開始行が非表示と判定された")
	}
	// 内部行は非表示
	if !fs.IsHidden(1) {
		t.Fatal("折畳内の行1が非表示でない")
	}
	if !fs.IsHidden(2) {
		t.Fatal("折畳内の行2が非表示でない")
	}
}

func TestFoldedLineCount(t *testing.T) {
	text := "func main() {\n    line1\n    line2\n}\n"
	lines := strings.Split(text, "\n")
	total := len(lines)
	fs := NewFoldState()
	fs.DetectFoldRegions(total, func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	})

	// 折畳前は全行表示
	if got := fs.FoldedLineCount(total); got != total {
		t.Fatalf("折畳前 FoldedLineCount = %d, 期待 %d", got, total)
	}

	fs.ToggleFold(0)
	visible := fs.FoldedLineCount(total)
	if visible >= total {
		t.Fatalf("折畳後に表示行数が減っていない: %d", visible)
	}
}

func TestVisibleLineIndex(t *testing.T) {
	text := "line0\n    line1\n    line2\nline3\n"
	lines := strings.Split(text, "\n")
	total := len(lines)
	fs := NewFoldState()
	fs.DetectFoldRegions(total, func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	})

	fs.ToggleFold(0)

	// 表示行0は実行0
	if got := fs.VisibleLineIndex(0, total); got != 0 {
		t.Fatalf("VisibleLineIndex(0) = %d, 期待 0", got)
	}
}

func TestIndentLevel(t *testing.T) {
	tests := []struct {
		line string
		want int
	}{
		{"hello", 0},
		{"  hello", 2},
		{"    hello", 4},
		{"\thello", 4},
		{"\t\thello", 8},
		{"  \thello", 6}, // 2スペース + 1タブ(=4)
	}
	for _, tt := range tests {
		got := indentLevel(tt.line)
		if got != tt.want {
			t.Errorf("indentLevel(%q) = %d, 期待 %d", tt.line, got, tt.want)
		}
	}
}
