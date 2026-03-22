package ai

import (
	"fmt"
	"strconv"
	"strings"
)

// DiffBlock はAI応答から抽出されたdiffブロック
type DiffBlock struct {
	FilePath string     // 対象ファイルパス
	Hunks    []DiffHunk // 変更ハンク
	RawText  string     // 元のdiffテキスト
}

// DiffHunk はdiffの1つのハンク（変更ブロック）
type DiffHunk struct {
	OldStart int        // 変更前開始行（1始まり）
	OldCount int        // 変更前行数
	NewStart int        // 変更後開始行（1始まり）
	NewCount int        // 変更後行数
	Lines    []DiffLine // ハンク内の行
}

// DiffLine はdiffの1行
type DiffLine struct {
	Type    DiffLineType // 行タイプ
	Content string       // 行内容（プレフィックス除去後）
}

// DiffLineType はdiff行の種別
type DiffLineType int

const (
	DiffContext DiffLineType = iota // コンテキスト（変更なし）
	DiffAdded                       // 追加行
	DiffDeleted                     // 削除行
)

// ExtractDiffBlocks はAI応答テキストからdiffブロックを抽出する
func ExtractDiffBlocks(text string) []DiffBlock {
	var blocks []DiffBlock
	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		// ```diff ブロックの開始を検出
		if strings.HasPrefix(line, "```diff") {
			i++
			var diffLines []string
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "```" {
					break
				}
				diffLines = append(diffLines, lines[i])
				i++
			}
			if len(diffLines) > 0 {
				rawText := strings.Join(diffLines, "\n")
				block := parseDiffBlock(diffLines)
				block.RawText = rawText
				blocks = append(blocks, block)
			}
		}
		i++
	}
	return blocks
}

// parseDiffBlock はdiffテキスト行をDiffBlockにパースする
func parseDiffBlock(lines []string) DiffBlock {
	block := DiffBlock{}
	i := 0

	// --- / +++ ヘッダーからファイルパスを抽出
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "--- ") {
			// ファイルパスの取得（--- a/path や --- path 形式）
			path := strings.TrimPrefix(line, "--- ")
			path = strings.TrimPrefix(path, "a/")
			block.FilePath = path
		} else if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			block.FilePath = path
			i++
			break
		} else if strings.HasPrefix(line, "@@") {
			// ヘッダーなしで直接ハンクが始まるパターン
			break
		} else if !strings.HasPrefix(line, "diff ") {
			// ヘッダーではない行が来たらハンクとして処理開始
			break
		}
		i++
	}

	// ハンクのパース
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "@@") {
			hunk := parseHunkHeader(line)
			i++
			// ハンク内の行を収集
			for i < len(lines) {
				l := lines[i]
				if strings.HasPrefix(l, "@@") || strings.HasPrefix(l, "diff ") {
					break
				}
				switch {
				case strings.HasPrefix(l, "+"):
					hunk.Lines = append(hunk.Lines, DiffLine{Type: DiffAdded, Content: l[1:]})
				case strings.HasPrefix(l, "-"):
					hunk.Lines = append(hunk.Lines, DiffLine{Type: DiffDeleted, Content: l[1:]})
				default:
					// コンテキスト行（先頭のスペースを除去）
					content := l
					if len(content) > 0 && content[0] == ' ' {
						content = content[1:]
					}
					hunk.Lines = append(hunk.Lines, DiffLine{Type: DiffContext, Content: content})
				}
				i++
			}
			block.Hunks = append(block.Hunks, hunk)
		} else {
			// @@なしの場合、全体を1つのハンクとして扱う
			hunk := DiffHunk{OldStart: 1, NewStart: 1}
			for i < len(lines) {
				l := lines[i]
				switch {
				case strings.HasPrefix(l, "+"):
					hunk.Lines = append(hunk.Lines, DiffLine{Type: DiffAdded, Content: l[1:]})
				case strings.HasPrefix(l, "-"):
					hunk.Lines = append(hunk.Lines, DiffLine{Type: DiffDeleted, Content: l[1:]})
				default:
					content := l
					if len(content) > 0 && content[0] == ' ' {
						content = content[1:]
					}
					hunk.Lines = append(hunk.Lines, DiffLine{Type: DiffContext, Content: content})
				}
				i++
			}
			if len(hunk.Lines) > 0 {
				block.Hunks = append(block.Hunks, hunk)
			}
		}
	}

	return block
}

// parseHunkHeader は @@ -old,count +new,count @@ 形式をパースする
func parseHunkHeader(line string) DiffHunk {
	hunk := DiffHunk{OldStart: 1, OldCount: 0, NewStart: 1, NewCount: 0}
	// "@@ -1,5 +1,7 @@" のような形式
	line = strings.TrimPrefix(line, "@@")
	if idx := strings.Index(line[1:], "@@"); idx >= 0 {
		line = line[:idx+1]
	}
	line = strings.TrimSpace(line)

	parts := strings.Fields(line)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") {
			nums := strings.TrimPrefix(p, "-")
			hunk.OldStart, hunk.OldCount = parseLineRange(nums)
		} else if strings.HasPrefix(p, "+") {
			nums := strings.TrimPrefix(p, "+")
			hunk.NewStart, hunk.NewCount = parseLineRange(nums)
		}
	}
	return hunk
}

// parseLineRange は "start,count" または "start" 形式をパースする
func parseLineRange(s string) (int, int) {
	if idx := strings.Index(s, ","); idx >= 0 {
		start, _ := strconv.Atoi(s[:idx])
		count, _ := strconv.Atoi(s[idx+1:])
		if start == 0 {
			start = 1
		}
		return start, count
	}
	start, _ := strconv.Atoi(s)
	if start == 0 {
		start = 1
	}
	return start, 1
}

// ApplyDiffToText はdiffブロックをテキストに適用して結果を返す
func ApplyDiffToText(original string, diff *DiffBlock) (string, error) {
	lines := strings.Split(original, "\n")
	// 末尾の空行を保持
	trailingNewline := strings.HasSuffix(original, "\n")

	for _, hunk := range diff.Hunks {
		newLines, err := applyHunk(lines, &hunk)
		if err != nil {
			return "", err
		}
		lines = newLines
	}

	result := strings.Join(lines, "\n")
	if trailingNewline && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

// applyHunk は1つのハンクをテキスト行に適用する
func applyHunk(lines []string, hunk *DiffHunk) ([]string, error) {
	// 0-indexed の開始位置
	startIdx := hunk.OldStart - 1
	if startIdx < 0 {
		startIdx = 0
	}

	// コンテキスト行でマッチ位置を検索（ファジーマッチ）
	matchIdx := findHunkMatch(lines, hunk, startIdx)
	if matchIdx < 0 {
		return nil, fmt.Errorf("ハンクの適用位置が見つかりません（行 %d 付近）", hunk.OldStart)
	}

	// 適用: 削除行とコンテキスト行を消費しつつ、追加行とコンテキスト行で置換
	var result []string
	result = append(result, lines[:matchIdx]...)

	lineIdx := matchIdx
	for _, dl := range hunk.Lines {
		switch dl.Type {
		case DiffContext:
			// コンテキスト行は元のテキストを保持
			if lineIdx < len(lines) {
				result = append(result, lines[lineIdx])
				lineIdx++
			} else {
				result = append(result, dl.Content)
			}
		case DiffDeleted:
			// 削除行はスキップ
			if lineIdx < len(lines) {
				lineIdx++
			}
		case DiffAdded:
			// 追加行を挿入
			result = append(result, dl.Content)
		}
	}

	// 残りの行を追加
	if lineIdx < len(lines) {
		result = append(result, lines[lineIdx:]...)
	}

	return result, nil
}

// findHunkMatch はハンクのコンテキスト行が一致する位置を検索する
func findHunkMatch(lines []string, hunk *DiffHunk, hint int) int {
	// コンテキスト行と削除行を使ってマッチングする元テキストのパターンを構築
	var pattern []string
	for _, dl := range hunk.Lines {
		if dl.Type == DiffContext || dl.Type == DiffDeleted {
			pattern = append(pattern, dl.Content)
		}
	}

	if len(pattern) == 0 {
		// パターンがない場合（全て追加行）はヒント位置を使用
		if hint > len(lines) {
			hint = len(lines)
		}
		return hint
	}

	// ヒント位置から前後を探索
	maxRange := 50
	for offset := 0; offset <= maxRange; offset++ {
		for _, dir := range []int{0, 1} {
			idx := hint
			if dir == 0 {
				idx = hint + offset
			} else {
				idx = hint - offset
			}
			if idx < 0 || idx+len(pattern) > len(lines) {
				continue
			}
			if matchAt(lines, idx, pattern) {
				return idx
			}
		}
	}

	return -1
}

// matchAt は指定位置でパターンが一致するか確認する
func matchAt(lines []string, pos int, pattern []string) bool {
	for i, p := range pattern {
		if pos+i >= len(lines) {
			return false
		}
		if strings.TrimRight(lines[pos+i], " \t") != strings.TrimRight(p, " \t") {
			return false
		}
	}
	return true
}
