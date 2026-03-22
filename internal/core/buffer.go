package core

import (
	"strings"
	"unicode/utf8"
)

// UndoAction はUndo操作の種別
type UndoAction int

const (
	ActionInsert UndoAction = iota
	ActionDelete
)

// UndoEntry はUndo/Redo用の変更記録
type UndoEntry struct {
	Action UndoAction
	Pos    int
	Text   string
}

// Buffer はRopeをラップし、カーソル・Undo・行キャッシュを提供する
type Buffer struct {
	rope      *Rope
	cursorPos int // バイトオフセットでのカーソル位置
	selStart  int // 選択開始位置（-1で無選択）
	selEnd    int // 選択終了位置
	undoStack []UndoEntry
	redoStack []UndoEntry
	lineCache []int      // 各行の開始バイトオフセットキャッシュ
	textCache string     // Ropeのフラットテキストキャッシュ
	textDirty bool       // textCacheが古い場合true
	FoldState *FoldState // コードフォールディング状態
}

// NewBuffer はテキストから新しいBufferを作成する
func NewBuffer(text string) *Buffer {
	b := &Buffer{
		rope:      NewRope(text),
		selStart:  -1,
		selEnd:    -1,
		textCache: text,
		textDirty: false,
		FoldState: NewFoldState(),
	}
	b.rebuildLineCache()
	b.UpdateFoldRegions()
	return b
}

// cachedText はキャッシュ済みのフラットテキストを返す（必要な時だけ再構築）
func (b *Buffer) cachedText() string {
	if b.textDirty {
		b.textCache = b.rope.String()
		b.textDirty = false
	}
	return b.textCache
}

// invalidateTextCache はテキストキャッシュを無効化し再構築する
func (b *Buffer) invalidateTextCache() {
	b.textCache = b.rope.String()
	b.textDirty = false
}

// Text はバッファの全テキストを返す
func (b *Buffer) Text() string {
	return b.cachedText()
}

// Length はテキストの総バイト数を返す
func (b *Buffer) Length() int {
	return b.rope.Length()
}

// CursorPos はカーソルのバイトオフセットを返す
func (b *Buffer) CursorPos() int {
	return b.cursorPos
}

// CursorLine はカーソルの行番号を返す（0始まり）
func (b *Buffer) CursorLine() int {
	line, _ := b.posToLineCol(b.cursorPos)
	return line
}

// CursorCol はカーソルの列番号を返す（0始まり）
func (b *Buffer) CursorCol() int {
	_, col := b.posToLineCol(b.cursorPos)
	return col
}

// LineCount は行数を返す
func (b *Buffer) LineCount() int {
	return len(b.lineCache)
}

// Line は指定行のテキストを返す（0始まり）
func (b *Buffer) Line(n int) string {
	if n < 0 || n >= len(b.lineCache) {
		return ""
	}
	text := b.cachedText()
	start := b.lineCache[n]
	if n+1 < len(b.lineCache) {
		end := b.lineCache[n+1] - 1 // 改行を除く
		if end < start {
			end = start
		}
		if end > len(text) {
			end = len(text)
		}
		return text[start:end]
	}
	// 最終行
	return text[start:]
}

// InsertText はカーソル位置にテキストを挿入する
func (b *Buffer) InsertText(text string) {
	if len(text) == 0 {
		return
	}
	// Undo記録
	b.undoStack = append(b.undoStack, UndoEntry{
		Action: ActionInsert,
		Pos:    b.cursorPos,
		Text:   text,
	})
	b.redoStack = nil

	b.rope.Insert(b.cursorPos, text)
	b.cursorPos += len(text)
	b.invalidateTextCache()
	b.rebuildLineCache()
}

// InsertNewline は改行を挿入する
func (b *Buffer) InsertNewline() {
	b.InsertText("\n")
}

// DeleteBackward はカーソルの前の1文字を削除する
func (b *Buffer) DeleteBackward() {
	if b.cursorPos <= 0 {
		return
	}
	deletePos := b.prevRuneStart(b.cursorPos)
	if deletePos >= b.cursorPos {
		return
	}

	text := b.cachedText()
	deleted := text[deletePos:b.cursorPos]

	// Undo記録
	b.undoStack = append(b.undoStack, UndoEntry{
		Action: ActionDelete,
		Pos:    deletePos,
		Text:   deleted,
	})
	b.redoStack = nil

	b.rope.Delete(deletePos, b.cursorPos)
	b.cursorPos = deletePos
	b.invalidateTextCache()
	b.rebuildLineCache()
}

// DeleteForward はカーソル位置の文字を削除する
func (b *Buffer) DeleteForward() {
	if b.cursorPos >= b.rope.Length() {
		return
	}
	text := b.cachedText()
	deleteEnd := b.nextRuneEnd(b.cursorPos)
	if deleteEnd <= b.cursorPos {
		return
	}
	deleted := text[b.cursorPos:deleteEnd]

	// Undo記録
	b.undoStack = append(b.undoStack, UndoEntry{
		Action: ActionDelete,
		Pos:    b.cursorPos,
		Text:   deleted,
	})
	b.redoStack = nil

	b.rope.Delete(b.cursorPos, deleteEnd)
	b.invalidateTextCache()
	b.rebuildLineCache()
}

// MoveCursorLeft はカーソルを1文字左に移動する
func (b *Buffer) MoveCursorLeft() {
	if b.cursorPos > 0 {
		b.cursorPos = b.prevRuneStart(b.cursorPos)
	}
}

// MoveCursorRight はカーソルを1文字右に移動する
func (b *Buffer) MoveCursorRight() {
	if b.cursorPos < b.rope.Length() {
		b.cursorPos = b.nextRuneEnd(b.cursorPos)
	}
}

// MoveCursorUp はカーソルを1行上に移動する
func (b *Buffer) MoveCursorUp() {
	line := b.CursorLine()
	col := b.CursorCol()
	if line > 0 {
		b.SetCursorLineCol(line-1, col)
	}
}

// MoveCursorDown はカーソルを1行下に移動する
func (b *Buffer) MoveCursorDown() {
	line := b.CursorLine()
	col := b.CursorCol()
	if line < b.LineCount()-1 {
		b.SetCursorLineCol(line+1, col)
	}
}

// MoveCursorHome はカーソルを行頭に移動する
func (b *Buffer) MoveCursorHome() {
	line := b.CursorLine()
	b.cursorPos = b.lineCache[line]
}

// MoveCursorEnd はカーソルを行末に移動する
func (b *Buffer) MoveCursorEnd() {
	line := b.CursorLine()
	lineText := b.Line(line)
	b.cursorPos = b.lineCache[line] + len(lineText)
}

// SetCursorLineCol はカーソルを指定の行/列に設定する
func (b *Buffer) SetCursorLineCol(line, col int) {
	if line < 0 {
		line = 0
	}
	if line >= b.LineCount() {
		line = b.LineCount() - 1
	}
	b.cursorPos = b.lineColToPos(line, col)
}

// SetCursorPos はカーソルをバイトオフセットで設定する
func (b *Buffer) SetCursorPos(pos int) {
	if pos < 0 {
		pos = 0
	}
	if pos > b.rope.Length() {
		pos = b.rope.Length()
	}
	b.cursorPos = pos
}

// Undo は直前の操作を元に戻す
func (b *Buffer) Undo() {
	if len(b.undoStack) == 0 {
		return
	}
	entry := b.undoStack[len(b.undoStack)-1]
	b.undoStack = b.undoStack[:len(b.undoStack)-1]
	b.redoStack = append(b.redoStack, entry)

	switch entry.Action {
	case ActionInsert:
		// 挿入を取り消し：テキスト削除
		b.rope.Delete(entry.Pos, entry.Pos+len(entry.Text))
		b.cursorPos = entry.Pos
	case ActionDelete:
		// 削除を取り消し：テキスト復元
		b.rope.Insert(entry.Pos, entry.Text)
		b.cursorPos = entry.Pos + len(entry.Text)
	}
	b.invalidateTextCache()
	b.rebuildLineCache()
}

// Redo はUndoを再実行する
func (b *Buffer) Redo() {
	if len(b.redoStack) == 0 {
		return
	}
	entry := b.redoStack[len(b.redoStack)-1]
	b.redoStack = b.redoStack[:len(b.redoStack)-1]
	b.undoStack = append(b.undoStack, entry)

	switch entry.Action {
	case ActionInsert:
		b.rope.Insert(entry.Pos, entry.Text)
		b.cursorPos = entry.Pos + len(entry.Text)
	case ActionDelete:
		b.rope.Delete(entry.Pos, entry.Pos+len(entry.Text))
		b.cursorPos = entry.Pos
	}
	b.invalidateTextCache()
	b.rebuildLineCache()
}

// rebuildLineCache は行キャッシュを再構築する
func (b *Buffer) rebuildLineCache() {
	text := b.cachedText()
	b.lineCache = b.lineCache[:0]
	b.lineCache = append(b.lineCache, 0)
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			b.lineCache = append(b.lineCache, i+1)
		}
	}
	_ = strings.Count // パッケージ使用のため
}

// HasSelection は選択範囲があるかどうかを返す
func (b *Buffer) HasSelection() bool {
	return b.selStart >= 0 && b.selStart != b.selEnd
}

// Selection は選択範囲のバイトオフセット（start, end）を返す。startは常にend以下。
func (b *Buffer) Selection() (int, int) {
	if b.selStart < 0 {
		return b.cursorPos, b.cursorPos
	}
	s, e := b.selStart, b.selEnd
	if s > e {
		s, e = e, s
	}
	return s, e
}

// SetSelection は選択範囲を設定する
func (b *Buffer) SetSelection(start, end int) {
	b.selStart = start
	b.selEnd = end
}

// SelectionAnchor は選択開始点（正規化前）を返す
func (b *Buffer) SelectionAnchor() int {
	return b.selStart
}

// ClearSelection は選択をリセットする
func (b *Buffer) ClearSelection() {
	b.selStart = -1
	b.selEnd = -1
}

// SelectionLineCol は選択範囲を行・列で返す
func (b *Buffer) SelectionLineCol() (startLine, startCol, endLine, endCol int) {
	s, e := b.Selection()
	startLine, startCol = b.posToLineCol(s)
	endLine, endCol = b.posToLineCol(e)
	return
}

// DeleteSelection は選択範囲を削除し、カーソルをその先頭に移動する
func (b *Buffer) DeleteSelection() string {
	if !b.HasSelection() {
		return ""
	}
	s, e := b.Selection()
	text := b.cachedText()
	deleted := text[s:e]

	b.undoStack = append(b.undoStack, UndoEntry{
		Action: ActionDelete,
		Pos:    s,
		Text:   deleted,
	})
	b.redoStack = nil

	b.rope.Delete(s, e)
	b.cursorPos = s
	b.selStart = -1
	b.selEnd = -1
	b.invalidateTextCache()
	b.rebuildLineCache()
	return deleted
}

// LineColToPos は行・列をバイトオフセットに変換する
func (b *Buffer) LineColToPos(line, col int) int {
	return b.lineColToPos(line, col)
}

// posToLineCol はバイトオフセットを行/列に変換する（キャッシュベース）
func (b *Buffer) posToLineCol(pos int) (line, col int) {
	text := b.cachedText()
	if pos < 0 {
		pos = 0
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

// lineColToPos は行/列をバイトオフセットに変換する（キャッシュベース）
func (b *Buffer) lineColToPos(line, col int) int {
	if line < 0 || line >= len(b.lineCache) {
		if line >= len(b.lineCache) {
			return b.rope.Length()
		}
		return 0
	}
	start := b.lineCache[line]
	lineText := b.Line(line)
	if col <= 0 {
		return start
	}
	runeCol := 0
	byteOffset := 0
	for byteOffset < len(lineText) && runeCol < col {
		_, size := utf8.DecodeRuneInString(lineText[byteOffset:])
		byteOffset += size
		runeCol++
	}
	return start + byteOffset
}

func (b *Buffer) prevRuneStart(pos int) int {
	if pos <= 0 {
		return 0
	}
	text := b.cachedText()
	if pos > len(text) {
		pos = len(text)
	}
	_, size := utf8.DecodeLastRuneInString(text[:pos])
	if size <= 0 {
		return pos - 1
	}
	return pos - size
}

func (b *Buffer) nextRuneEnd(pos int) int {
	text := b.cachedText()
	if pos < 0 {
		pos = 0
	}
	if pos >= len(text) {
		return len(text)
	}
	_, size := utf8.DecodeRuneInString(text[pos:])
	if size <= 0 {
		return pos + 1
	}
	return pos + size
}

// UpdateFoldRegions はコードフォールディング領域を再検出する
func (b *Buffer) UpdateFoldRegions() {
	b.FoldState.DetectFoldRegions(b.LineCount(), b.Line)
}
