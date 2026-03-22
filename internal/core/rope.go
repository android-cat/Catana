package core

import "unicode/utf8"

// Rope はテキスト編集用の永続データ構造
// 大きなテキストの挿入・削除を O(log n) で実行する

const maxLeafSize = 512

// Rope はテキストを効率的に操作するためのデータ構造
type Rope struct {
	root *ropeNode
}

type ropeNode struct {
	weight int
	left   *ropeNode
	right  *ropeNode
	text   []byte // リーフノードのみ使用
}

// NewRope はテキストから新しいRopeを構築する
func NewRope(text string) *Rope {
	if len(text) == 0 {
		return &Rope{root: &ropeNode{weight: 0, text: []byte{}}}
	}
	return &Rope{root: buildRope([]byte(text))}
}

func buildRope(data []byte) *ropeNode {
	if len(data) <= maxLeafSize {
		buf := make([]byte, len(data))
		copy(buf, data)
		return &ropeNode{weight: len(data), text: buf}
	}
	mid := len(data) / 2
	left := buildRope(data[:mid])
	right := buildRope(data[mid:])
	return &ropeNode{
		weight: left.length(),
		left:   left,
		right:  right,
	}
}

func (n *ropeNode) isLeaf() bool {
	return n.text != nil
}

func (n *ropeNode) length() int {
	if n == nil {
		return 0
	}
	if n.isLeaf() {
		return len(n.text)
	}
	return n.weight + n.right.length()
}

// Length はRopeの総バイト数を返す
func (r *Rope) Length() int {
	if r.root == nil {
		return 0
	}
	return r.root.length()
}

// Insert は指定位置にテキストを挿入する
func (r *Rope) Insert(pos int, text string) {
	if len(text) == 0 {
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos > r.Length() {
		pos = r.Length()
	}
	newNode := buildRope([]byte(text))
	left, right := split(r.root, pos)
	r.root = merge(merge(left, newNode), right)
}

// Delete は指定範囲のテキストを削除する
func (r *Rope) Delete(start, end int) {
	if start >= end {
		return
	}
	if start < 0 {
		start = 0
	}
	if end > r.Length() {
		end = r.Length()
	}
	left, mid := split(r.root, start)
	_, right := split(mid, end-start)
	r.root = merge(left, right)
}

// String はRope全体をstringとして返す
func (r *Rope) String() string {
	if r.root == nil {
		return ""
	}
	buf := make([]byte, 0, r.Length())
	collectBytes(r.root, &buf)
	return string(buf)
}

// Substring は指定範囲のテキストを返す
func (r *Rope) Substring(start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > r.Length() {
		end = r.Length()
	}
	if start >= end {
		return ""
	}
	_, mid := split(r.root, start)
	result, _ := split(mid, end-start)
	buf := make([]byte, 0, end-start)
	collectBytes(result, &buf)
	// splitは破壊的なので元に戻すためにStringから再構築
	// Phase 1ではコピーベースの安全な方法を採用
	text := r.String()
	return text[start:end]
}

// Index は指定位置のバイトを返す
func (r *Rope) Index(pos int) byte {
	return indexNode(r.root, pos)
}

func indexNode(n *ropeNode, pos int) byte {
	if n.isLeaf() {
		return n.text[pos]
	}
	if pos < n.weight {
		return indexNode(n.left, pos)
	}
	return indexNode(n.right, pos-n.weight)
}

// LineCount は行数を返す（改行文字でカウント）
func (r *Rope) LineCount() int {
	text := r.String()
	count := 1
	for _, b := range []byte(text) {
		if b == '\n' {
			count++
		}
	}
	return count
}

// Line は指定行のテキストを返す（0始まり、改行文字は含まない）
func (r *Rope) Line(n int) string {
	text := r.String()
	line := 0
	start := 0
	for i, b := range []byte(text) {
		if b == '\n' {
			if line == n {
				return text[start:i]
			}
			line++
			start = i + 1
		}
	}
	if line == n {
		return text[start:]
	}
	return ""
}

// LineStart は指定行の開始バイトオフセットを返す（0始まり）
func (r *Rope) LineStart(n int) int {
	if n <= 0 {
		return 0
	}
	text := r.String()
	line := 0
	for i, b := range []byte(text) {
		if b == '\n' {
			line++
			if line == n {
				return i + 1
			}
		}
	}
	return r.Length()
}

// PosToLineCol はバイトオフセットを行/列に変換する
func (r *Rope) PosToLineCol(pos int) (line, col int) {
	text := r.String()
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

// LineColToPos は行/列をバイトオフセットに変換する
func (r *Rope) LineColToPos(line, col int) int {
	start := r.LineStart(line)
	lineText := r.Line(line)
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

// split はRopeノードを指定位置で分割する
func split(n *ropeNode, pos int) (*ropeNode, *ropeNode) {
	if n == nil {
		return nil, nil
	}
	if n.isLeaf() {
		if pos <= 0 {
			return nil, n
		}
		if pos >= len(n.text) {
			return n, nil
		}
		leftBuf := make([]byte, pos)
		copy(leftBuf, n.text[:pos])
		rightBuf := make([]byte, len(n.text)-pos)
		copy(rightBuf, n.text[pos:])
		return &ropeNode{weight: pos, text: leftBuf},
			&ropeNode{weight: len(rightBuf), text: rightBuf}
	}
	if pos <= n.weight {
		left, right := split(n.left, pos)
		return left, merge(right, n.right)
	}
	left, right := split(n.right, pos-n.weight)
	return merge(n.left, left), right
}

// merge は2つのRopeノードを結合する
func merge(left, right *ropeNode) *ropeNode {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return &ropeNode{
		weight: left.length(),
		left:   left,
		right:  right,
	}
}

func collectBytes(n *ropeNode, buf *[]byte) {
	if n == nil {
		return
	}
	if n.isLeaf() {
		*buf = append(*buf, n.text...)
		return
	}
	collectBytes(n.left, buf)
	collectBytes(n.right, buf)
}
