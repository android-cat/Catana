package core

import "strings"

// FoldRegion はコードの折畳領域を表す
type FoldRegion struct {
	StartLine int  // 折畳の開始行（0始まり）
	EndLine   int  // 折畳の終了行（0始まり）
	Folded    bool // 折畳中かどうか
	Indent    int  // 開始行のインデントレベル
}

// FoldState はバッファの折畳状態を管理する
type FoldState struct {
	Regions []FoldRegion
}

// NewFoldState は初期状態の折畳状態を返す
func NewFoldState() *FoldState {
	return &FoldState{}
}

// DetectFoldRegions はインデントベースで折畳可能領域を検出する
func (fs *FoldState) DetectFoldRegions(lineCount int, lineFunc func(int) string) {
	// 既存の折畳状態を保持するためのマップ
	foldedLines := make(map[int]bool)
	for _, r := range fs.Regions {
		if r.Folded {
			foldedLines[r.StartLine] = true
		}
	}

	fs.Regions = fs.Regions[:0]

	if lineCount <= 1 {
		return
	}

	// 各行のインデントレベルを計算
	indents := make([]int, lineCount)
	isBlank := make([]bool, lineCount)
	for i := 0; i < lineCount; i++ {
		line := lineFunc(i)
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) == 0 {
			isBlank[i] = true
			indents[i] = -1
		} else {
			indents[i] = indentLevel(line)
		}
	}

	// 空行のインデントを前後の行から推定
	for i := 0; i < lineCount; i++ {
		if isBlank[i] {
			// 次の非空行のインデントを使用
			for j := i + 1; j < lineCount; j++ {
				if !isBlank[j] {
					indents[i] = indents[j]
					break
				}
			}
		}
	}

	// 折畳領域検出: 現在行より深いインデントの連続が続く区間
	for i := 0; i < lineCount-1; i++ {
		if isBlank[i] || indents[i] < 0 {
			continue
		}
		startIndent := indents[i]

		// 次行以降でインデントが深い行が続く範囲を探す
		endLine := -1
		for j := i + 1; j < lineCount; j++ {
			if isBlank[j] {
				continue
			}
			if indents[j] > startIndent {
				endLine = j
			} else {
				break
			}
		}

		if endLine > i {
			region := FoldRegion{
				StartLine: i,
				EndLine:   endLine,
				Indent:    startIndent,
				Folded:    foldedLines[i],
			}
			fs.Regions = append(fs.Regions, region)
		}
	}
}

// ToggleFold は指定行の折畳状態をトグルする
func (fs *FoldState) ToggleFold(line int) {
	for i := range fs.Regions {
		if fs.Regions[i].StartLine == line {
			fs.Regions[i].Folded = !fs.Regions[i].Folded
			return
		}
	}
}

// IsFolded は指定行が折畳の開始行かどうかを返す
func (fs *FoldState) IsFolded(line int) bool {
	for _, r := range fs.Regions {
		if r.StartLine == line && r.Folded {
			return true
		}
	}
	return false
}

// IsHidden は指定行が折畳によって非表示かどうかを返す
func (fs *FoldState) IsHidden(line int) bool {
	for _, r := range fs.Regions {
		if r.Folded && line > r.StartLine && line <= r.EndLine {
			return true
		}
	}
	return false
}

// IsFoldable は指定行が折畳可能かどうかを返す
func (fs *FoldState) IsFoldable(line int) bool {
	for _, r := range fs.Regions {
		if r.StartLine == line {
			return true
		}
	}
	return false
}

// FoldedLineCount は折畳された行数を除いた表示行数を返す
func (fs *FoldState) FoldedLineCount(totalLines int) int {
	hidden := 0
	for i := 0; i < totalLines; i++ {
		if fs.IsHidden(i) {
			hidden++
		}
	}
	return totalLines - hidden
}

// VisibleLineIndex は表示行インデックスから実際の行番号に変換する
func (fs *FoldState) VisibleLineIndex(visibleIdx int, totalLines int) int {
	count := 0
	for i := 0; i < totalLines; i++ {
		if fs.IsHidden(i) {
			continue
		}
		if count == visibleIdx {
			return i
		}
		count++
	}
	return totalLines - 1
}

// indentLevel はタブ=4スペースとして行のインデントレベルを計算する
func indentLevel(line string) int {
	level := 0
	for _, c := range line {
		switch c {
		case ' ':
			level++
		case '\t':
			level += 4
		default:
			return level
		}
	}
	return level
}
