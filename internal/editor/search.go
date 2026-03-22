package editor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SearchMatch はファイル内の一致箇所を表す
type SearchMatch struct {
	Line      int    // 0始まり
	Col       int    // ルーン列（0始まり）
	Length    int    // マッチの文字数（ルーン）
	ByteStart int    // テキスト内のバイトオフセット
	ByteEnd   int    // テキスト内のバイトオフセット
	LineText  string // 行テキスト（プレビュー用）
}

// WorkspaceMatch はワークスペース検索の結果を表す
type WorkspaceMatch struct {
	FilePath string
	Matches  []SearchMatch
}

// SearchState はエディタの検索状態を管理する
type SearchState struct {
	Query         string        // 現在の検索クエリ
	ReplaceText   string        // 置換テキスト
	IsRegex       bool          // 正規表現モード
	CaseSensitive bool          // 大文字小文字を区別
	Active        bool          // 検索バーが表示中か
	Matches       []SearchMatch // 現在のファイル内マッチ結果
	CurrentMatch  int           // 現在ハイライトしているマッチのインデックス
	// ワークスペース検索結果
	WorkspaceResults []WorkspaceMatch
	WorkspaceActive  bool // ワークスペース検索が有効か
}

// NewSearchState は検索状態を初期化する
func NewSearchState() *SearchState {
	return &SearchState{
		CurrentMatch: -1,
	}
}

// SearchInBuffer はバッファ内で検索を実行しマッチ結果を更新する
func (ss *SearchState) SearchInBuffer(text string, lineCache func(int) string, lineCount int) {
	ss.Matches = ss.Matches[:0]
	ss.CurrentMatch = -1

	if ss.Query == "" {
		return
	}

	if ss.IsRegex {
		ss.searchRegexInText(text, lineCache, lineCount)
	} else {
		ss.searchPlainInText(text, lineCache, lineCount)
	}

	if len(ss.Matches) > 0 {
		ss.CurrentMatch = 0
	}
}

func (ss *SearchState) searchPlainInText(text string, lineCache func(int) string, lineCount int) {
	query := ss.Query
	searchText := text
	if !ss.CaseSensitive {
		query = strings.ToLower(query)
		searchText = strings.ToLower(text)
	}

	queryLen := utf8.RuneCountInString(ss.Query)
	offset := 0
	for {
		idx := strings.Index(searchText[offset:], query)
		if idx < 0 {
			break
		}
		byteStart := offset + idx
		byteEnd := byteStart + len(ss.Query)
		if !ss.CaseSensitive {
			// 元テキストのバイト長を使用
			byteEnd = byteStart + len(query)
		}

		line, col := posToLineCol(text, byteStart)
		lineText := ""
		if line < lineCount {
			lineText = lineCache(line)
		}

		ss.Matches = append(ss.Matches, SearchMatch{
			Line:      line,
			Col:       col,
			Length:    queryLen,
			ByteStart: byteStart,
			ByteEnd:   byteEnd,
			LineText:  lineText,
		})

		offset = byteStart + len(query)
		if offset >= len(searchText) {
			break
		}
	}
}

func (ss *SearchState) searchRegexInText(text string, lineCache func(int) string, lineCount int) {
	pattern := ss.Query
	if !ss.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return
	}

	locs := re.FindAllStringIndex(text, -1)
	for _, loc := range locs {
		byteStart := loc[0]
		byteEnd := loc[1]
		matched := text[byteStart:byteEnd]

		line, col := posToLineCol(text, byteStart)
		lineText := ""
		if line < lineCount {
			lineText = lineCache(line)
		}

		ss.Matches = append(ss.Matches, SearchMatch{
			Line:      line,
			Col:       col,
			Length:    utf8.RuneCountInString(matched),
			ByteStart: byteStart,
			ByteEnd:   byteEnd,
			LineText:  lineText,
		})
	}
}

// NextMatch は次のマッチに移動する
func (ss *SearchState) NextMatch() {
	if len(ss.Matches) == 0 {
		return
	}
	ss.CurrentMatch = (ss.CurrentMatch + 1) % len(ss.Matches)
}

// PrevMatch は前のマッチに移動する
func (ss *SearchState) PrevMatch() {
	if len(ss.Matches) == 0 {
		return
	}
	ss.CurrentMatch--
	if ss.CurrentMatch < 0 {
		ss.CurrentMatch = len(ss.Matches) - 1
	}
}

// CurrentMatchInfo は現在のマッチ情報を返す（なければnil）
func (ss *SearchState) CurrentMatchInfo() *SearchMatch {
	if ss.CurrentMatch < 0 || ss.CurrentMatch >= len(ss.Matches) {
		return nil
	}
	return &ss.Matches[ss.CurrentMatch]
}

// ReplaceCurrent は現在のマッチを置換する
func (ss *SearchState) ReplaceCurrent(doc *Document) bool {
	m := ss.CurrentMatchInfo()
	if m == nil || doc == nil {
		return false
	}

	// 選択範囲をマッチ位置に設定して削除・挿入
	doc.Buffer.SetSelection(m.ByteStart, m.ByteEnd)
	doc.Buffer.DeleteSelection()
	doc.Buffer.InsertText(ss.ReplaceText)
	doc.Modified = true
	return true
}

// ReplaceAll は全マッチを置換する
func (ss *SearchState) ReplaceAll(doc *Document) int {
	if len(ss.Matches) == 0 || doc == nil {
		return 0
	}

	// 末尾から置換してオフセットが崩れないようにする
	count := 0
	for i := len(ss.Matches) - 1; i >= 0; i-- {
		m := ss.Matches[i]
		doc.Buffer.SetSelection(m.ByteStart, m.ByteEnd)
		doc.Buffer.DeleteSelection()
		doc.Buffer.InsertText(ss.ReplaceText)
		count++
	}
	doc.Modified = true
	ss.Matches = ss.Matches[:0]
	ss.CurrentMatch = -1
	return count
}

// SearchWorkspace はワークスペース全体で検索を実行する
func (ss *SearchState) SearchWorkspace(workspace string) {
	ss.WorkspaceResults = ss.WorkspaceResults[:0]
	if ss.Query == "" || workspace == "" {
		return
	}

	_ = filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// 隠しディレクトリやnode_modules等をスキップ
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		// テキストファイルのみ対象
		if info.Size() > 1*1024*1024 { // 1MB以上はスキップ
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// バイナリチェック
		check := data
		if len(check) > 512 {
			check = check[:512]
		}
		for _, b := range check {
			if b == 0 {
				return nil
			}
		}

		text := string(data)
		lines := strings.Split(text, "\n")

		var matches []SearchMatch
		query := ss.Query
		if ss.IsRegex {
			pattern := query
			if !ss.CaseSensitive {
				pattern = "(?i)" + pattern
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil
			}
			for lineIdx, line := range lines {
				locs := re.FindAllStringIndex(line, -1)
				for _, loc := range locs {
					matches = append(matches, SearchMatch{
						Line:     lineIdx,
						Col:      utf8.RuneCountInString(line[:loc[0]]),
						Length:   utf8.RuneCountInString(line[loc[0]:loc[1]]),
						LineText: line,
					})
				}
			}
		} else {
			searchQuery := query
			if !ss.CaseSensitive {
				searchQuery = strings.ToLower(query)
			}
			for lineIdx, line := range lines {
				searchLine := line
				if !ss.CaseSensitive {
					searchLine = strings.ToLower(line)
				}
				offset := 0
				for {
					idx := strings.Index(searchLine[offset:], searchQuery)
					if idx < 0 {
						break
					}
					col := utf8.RuneCountInString(line[:offset+idx])
					matches = append(matches, SearchMatch{
						Line:     lineIdx,
						Col:      col,
						Length:   utf8.RuneCountInString(query),
						LineText: line,
					})
					offset += idx + len(searchQuery)
				}
			}
		}

		if len(matches) > 0 {
			ss.WorkspaceResults = append(ss.WorkspaceResults, WorkspaceMatch{
				FilePath: path,
				Matches:  matches,
			})
		}
		return nil
	})
}

// posToLineCol はバイトオフセットを行/列（ルーン）に変換する
func posToLineCol(text string, pos int) (line, col int) {
	if pos <= 0 {
		return 0, 0
	}
	if pos > len(text) {
		pos = len(text)
	}
	for i := 0; i < pos; {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '\n' {
			line++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return
}
