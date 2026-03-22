package ui

import (
	"catana/internal/editor"
	"catana/internal/git"
	"catana/internal/lsp"
	"catana/internal/syntax"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mattn/go-runewidth"
)

// EditorView はコードエディタの表示領域
type EditorView struct {
	theme           *Theme
	list            widget.List
	tag             bool
	focused         bool
	cursorVisible   bool
	lastBlink       time.Time
	charWidthF      float32 // 半角文字(1カラム)のピクセル幅
	fullWidthCharF  float32 // 全角文字(2カラム)のピクセル幅
	charWidthCached bool    // 文字幅計測済みフラグ
	lineHeightF     float32
	lineNumWidthF   float32
	scrollAcc       float32 // サブピクセルスクロール蓄積
	// ドラッグ選択用
	dragging   bool
	pressedPID pointer.ID
	viewportH  int // ビューポート高さ（ピクセル）
	lineHits   []lineHit
	layoutY    int
	lastDoc    *editor.Document
	// カーソル追従スクロール用
	lastCursorLine int
	lastCursorCol  int
	// 再利用バッファ（アロケーション削減）
	spanMetricsBuf []spanMetric
	// 検索マッチ参照
	searchMatches     []editor.SearchMatch
	searchMatchIdx    int // 現在のマッチインデックス
	searchMatchColor  color.NRGBA
	searchActiveColor color.NRGBA
	// LSP補完ポップアップ
	completionPopup *CompletionPopup
	// LSP診断キャッシュ
	diagnostics []lsp.Diagnostic
	diagURI     string
	diagVersion int
	// Diff表示用
	diffList widget.List
	// AIインライン補完（ゴーストテキスト）
	ghostText string
	ghostLine int
}

// spanMetric はスパンの描画メトリクス
type spanMetric struct {
	charStart    int
	charLen      int
	text         string
	visualOffset int
	visualCols   int
	pixelWidth   int
}

type lineHit struct {
	line int
	top  int
	end  int
}

const editorTabSize = 4

// NewEditorView は新しいEditorViewを作成する
func NewEditorView(theme *Theme) *EditorView {
	ev := &EditorView{
		theme:           theme,
		cursorVisible:   true,
		lastBlink:       time.Now(),
		lastCursorLine:  -1,
		completionPopup: NewCompletionPopup(theme),
		ghostLine:       -1,
	}
	ev.list.Axis = layout.Vertical
	ev.diffList.Axis = layout.Vertical
	return ev
}

// Layout はエディタビューを描画する
func (ev *EditorView) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	doc := state.ActiveDocument()
	if doc == nil {
		return ev.layoutWelcome(gtx, th)
	}

	// Diffタブの場合はdiff専用描画
	if doc.IsDiff {
		return ev.layoutDiff(gtx, th, doc)
	}

	if doc != ev.lastDoc {
		ev.list.Position.First = 0
		ev.list.Position.Offset = 0
		ev.scrollAcc = 0
		ev.lastCursorLine = -1
		ev.lastCursorCol = -1
		ev.lastDoc = doc
	}

	// 等幅フォントの文字幅を一度だけ計測
	ev.ensureCharWidth(gtx, th)

	// キーイベント処理
	ev.handleKeyEvents(gtx, state)

	// ポインターイベント処理（クリックでフォーカス取得）
	ev.handlePointerEvents(gtx, state, th)

	// カーソル点滅
	now := time.Now()
	if now.Sub(ev.lastBlink) > 500*time.Millisecond {
		ev.cursorVisible = !ev.cursorVisible
		ev.lastBlink = now
	}

	// 即時再描画要求（連続フレーム描画で高FPSを実現）
	gtx.Execute(op.InvalidateCmd{})

	// カーソル移動時のみスクロール追従（手動スクロールを妨げない）
	curLine := doc.Buffer.CursorLine()
	curCol := doc.Buffer.CursorCol()
	if curLine != ev.lastCursorLine || curCol != ev.lastCursorCol {
		if state.ScrollCenterRequest {
			ev.scrollToCenter(doc)
			state.ScrollCenterRequest = false
		} else {
			ev.scrollToCursor(doc)
		}
		ev.lastCursorLine = curLine
		ev.lastCursorCol = curCol
	}

	// 検索マッチの更新
	if state.Search.Active && len(state.Search.Matches) > 0 {
		ev.searchMatches = state.Search.Matches
		ev.searchMatchIdx = state.Search.CurrentMatch
		ev.searchMatchColor = ev.theme.SearchMatch
		ev.searchActiveColor = ev.theme.SearchMatchActive
	} else {
		ev.searchMatches = nil
	}

	// AIゴーストテキストをstateから同期
	ev.ghostText = state.AIGhostText
	ev.ghostLine = state.AIGhostLine

	viewportSize := gtx.Constraints.Max
	dims := withBg(gtx, func(gtx C, _ image.Point) {
		fillBackground(gtx, ev.theme.Background, viewportSize)
	}, func(gtx C) D {
		// 折畳による表示行リストを構築
		foldState := doc.Buffer.FoldState
		totalLines := doc.Buffer.LineCount()
		visibleCount := foldState.FoldedLineCount(totalLines)
		ev.lineHits = ev.lineHits[:0]
		ev.layoutY = -ev.list.Position.Offset

		return material.List(th, &ev.list).Layout(gtx, visibleCount, func(gtx C, i int) D {
			realLine := foldState.VisibleLineIndex(i, totalLines)
			return ev.layoutLine(gtx, th, state, doc, realLine)
		})
	})
	dims.Size = viewportSize

	ev.viewportH = dims.Size.Y

	// LSP診断キャッシュ更新
	ev.updateDiagnostics(state, doc)

	// 補完ポップアップオーバーレイ描画
	if ev.completionPopup.IsVisible() {
		cursorX, cursorY := ev.cursorScreenPos(doc)
		ev.completionPopup.Layout(gtx, th, cursorX, cursorY)
	}

	// 入力イベント用にタグをクリップ領域に登録（Gio v0.9必須）
	defer clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	pointer.CursorText.Add(gtx.Ops)
	event.Op(gtx.Ops, &ev.tag)

	return dims
}

// scrollToCenter はカーソル行をビューポート中央に配置する
func (ev *EditorView) scrollToCenter(doc *editor.Document) {
	if ev.lineHeightF <= 0 || ev.viewportH <= 0 {
		return
	}
	cursorLine := doc.Buffer.CursorLine()
	visibleLines := int(float32(ev.viewportH) / ev.lineHeightF)
	if visibleLines < 1 {
		visibleLines = 1
	}
	first := cursorLine - visibleLines/2
	if first < 0 {
		first = 0
	}
	ev.list.Position.First = first
	ev.list.Position.Offset = 0
}

// scrollToCursor はカーソルが表示範囲内に入るようリストをスクロールする
func (ev *EditorView) scrollToCursor(doc *editor.Document) {
	if ev.lineHeightF <= 0 || ev.viewportH <= 0 {
		return
	}
	cursorLine := doc.Buffer.CursorLine()
	first := ev.list.Position.First
	offset := ev.list.Position.Offset

	// ビューポートに表示できる行数
	visibleLines := int(float32(ev.viewportH) / ev.lineHeightF)
	if visibleLines < 1 {
		visibleLines = 1
	}

	// カーソルが表示範囲より上
	if cursorLine < first || (cursorLine == first && offset > 0) {
		ev.list.Position.First = cursorLine
		ev.list.Position.Offset = 0
	}
	// カーソルが表示範囲より下
	lastVisible := first + visibleLines - 1
	if cursorLine > lastVisible {
		ev.list.Position.First = cursorLine - visibleLines + 1
		if ev.list.Position.First < 0 {
			ev.list.Position.First = 0
		}
		ev.list.Position.Offset = 0
	}
}

func (ev *EditorView) ensureLineHit(lineIdx, height int) {
	top := ev.layoutY
	end := top + height
	ev.lineHits = append(ev.lineHits, lineHit{line: lineIdx, top: top, end: end})
	ev.layoutY = end
}

func (ev *EditorView) lineFromY(y int, doc *editor.Document) int {
	for _, hit := range ev.lineHits {
		if y >= hit.top && y < hit.end {
			return hit.line
		}
	}

	if ev.lineHeightF <= 0 {
		return 0
	}

	lineF := float32(y+ev.list.Position.Offset) / ev.lineHeightF
	line := ev.list.Position.First + int(lineF)
	maxLine := doc.Buffer.LineCount() - 1
	if line < 0 {
		return 0
	}
	if line > maxLine {
		return maxLine
	}
	return line
}

func (ev *EditorView) layoutLine(gtx C, th *material.Theme, state *editor.EditorState, doc *editor.Document, lineIdx int) D {
	cursorLine := doc.Buffer.CursorLine()
	cursorCol := doc.Buffer.CursorCol()
	isCurrentLine := lineIdx == cursorLine

	var dims D
	if isCurrentLine {
		// カレント行のみ背景描画付き（op.Record使用）
		macro := op.Record(gtx.Ops)
		innerDims := ev.layoutLineInner(gtx, th, doc, lineIdx, true, cursorCol)
		call := macro.Stop()
		fillBackground(gtx, ev.theme.CurrentLineBg, innerDims.Size)
		call.Add(gtx.Ops)
		dims = innerDims
	} else {
		// 非カレント行はop.Recordをスキップ
		dims = ev.layoutLineInner(gtx, th, doc, lineIdx, false, cursorCol)
	}

	ev.lineHeightF = float32(dims.Size.Y)
	ev.ensureLineHit(lineIdx, dims.Size.Y)
	return dims
}

// layoutLineInner は行の内容を描画する（クロージャ不使用）
func (ev *EditorView) layoutLineInner(gtx C, th *material.Theme, doc *editor.Document, lineIdx int, isCurrentLine bool, cursorCol int) D {
	insetH := gtx.Dp(unit.Dp(8))
	insetV := gtx.Dp(unit.Dp(1))
	foldW := gtx.Dp(unit.Dp(14))

	stack := op.Offset(image.Pt(insetH, insetV)).Push(gtx.Ops)

	// 折畳マーカー
	foldState := doc.Buffer.FoldState
	foldGtx := gtx
	foldGtx.Constraints.Min = image.Point{}
	if foldState.IsFoldable(lineIdx) {
		marker := "▼"
		if foldState.IsFolded(lineIdx) {
			marker = "▶"
		}
		fLbl := material.Label(th, unit.Sp(9), marker)
		fLbl.Color = ev.theme.TextDark
		fmOff := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(2)))).Push(gtx.Ops)
		fLbl.Layout(foldGtx)
		fmOff.Pop()
	}

	numOff := op.Offset(image.Pt(foldW, 0)).Push(gtx.Ops)

	// 行番号
	numStr := formatLineNum(lineIdx + 1)
	numCol := ev.theme.LineNumber
	if isCurrentLine {
		numCol = ev.theme.LineNumberActive
	}
	numGtx := gtx
	numGtx.Constraints.Min = image.Point{}
	lbl := material.Label(th, unit.Sp(13), numStr)
	lbl.Color = numCol
	lbl.Font = font.Font{Typeface: "Go Mono"}
	numDims := lbl.Layout(numGtx)
	ev.lineNumWidthF = float32(numDims.Size.X) + float32(foldW)

	// 折畳中の行は省略表示
	if foldState.IsFolded(lineIdx) {
		foldOff := op.Offset(image.Pt(numDims.Size.X, 0)).Push(gtx.Ops)
		codeDims := ev.layoutCodeSpans(gtx, th, doc, lineIdx, isCurrentLine, cursorCol)
		foldOff.Pop()

		// 折畳インジケータ追加
		codEndX := numDims.Size.X + codeDims.Size.X + gtx.Dp(unit.Dp(4))
		foldIndicOff := op.Offset(image.Pt(codEndX, 0)).Push(gtx.Ops)
		fiBg := op.Offset(image.Pt(-2, 0)).Push(gtx.Ops)
		fillBackground(gtx, ev.theme.SubtleBg, image.Pt(gtx.Dp(unit.Dp(24)), numDims.Size.Y))
		fiBg.Pop()
		fiLbl := material.Label(th, unit.Sp(10), "···")
		fiLbl.Color = ev.theme.TextDark
		fiLbl.Layout(foldGtx)
		foldIndicOff.Pop()

		numOff.Pop()
		stack.Pop()
		height := numDims.Size.Y
		return D{Size: image.Pt(gtx.Constraints.Max.X, height+insetV*2)}
	}

	// コードスパン（行番号の右に直接配置）
	codeOffs := op.Offset(image.Pt(numDims.Size.X, 0)).Push(gtx.Ops)
	codeDims := ev.layoutCodeSpans(gtx, th, doc, lineIdx, isCurrentLine, cursorCol)
	codeOffs.Pop()

	numOff.Pop()
	stack.Pop()

	height := numDims.Size.Y
	if codeDims.Size.Y > height {
		height = codeDims.Size.Y
	}
	return D{Size: image.Pt(gtx.Constraints.Max.X, height+insetV*2)}
}

func (ev *EditorView) layoutCodeSpans(gtx C, th *material.Theme, doc *editor.Document, lineIdx int, isCurrentLine bool, cursorCol int) D {
	lineText := doc.Buffer.Line(lineIdx)
	var spans []syntax.Span
	if lineIdx < len(doc.HighlightedLines) {
		spans = doc.HighlightedLines[lineIdx]
	}
	if len(spans) == 0 {
		spans = []syntax.Span{{Text: lineText, Type: syntax.TokenPlain}}
	}

	// 選択範囲の行内列範囲を計算
	selColStart := -1
	selColEnd := -1
	if doc.Buffer.HasSelection() {
		sl, sc, el, ec := doc.Buffer.SelectionLineCol()
		if lineIdx >= sl && lineIdx <= el {
			lineLen := utf8.RuneCountInString(lineText)
			if sl == el {
				selColStart = sc
				selColEnd = ec
			} else if lineIdx == sl {
				selColStart = sc
				selColEnd = lineLen
			} else if lineIdx == el {
				selColStart = 0
				selColEnd = ec
			} else {
				selColStart = 0
				selColEnd = lineLen
			}
		}
	}

	selColor := ev.theme.Selection

	// スパン描画メトリクス（カーソル位置計算用）
	ev.spanMetricsBuf = ev.spanMetricsBuf[:0]
	charOffset := 0
	visualOffset := 0
	xOffset := 0
	maxHeight := 0

	// 各スパンをop.Offsetで直接配置（Flex + クロージャを回避）
	spanGtx := gtx
	spanGtx.Constraints.Min = image.Point{}

	for idx := range spans {
		s := spans[idx]
		spanStart := charOffset
		spanLen := utf8.RuneCountInString(s.Text)
		charOffset += spanLen
		expandedText, spanVisualCols := expandTabs(s.Text, visualOffset)
		spanVisualOffset := visualOffset
		visualOffset += spanVisualCols

		// スパンメトリクスを記録
		mIdx := len(ev.spanMetricsBuf)
		ev.spanMetricsBuf = append(ev.spanMetricsBuf, spanMetric{
			charStart:    spanStart,
			charLen:      spanLen,
			text:         s.Text,
			visualOffset: spanVisualOffset,
			visualCols:   spanVisualCols,
		})

		// 選択ハイライト計算
		var hlStart, hlEnd int
		hasHighlight := false
		if selColStart >= 0 && selColEnd > selColStart {
			spanEnd := spanStart + spanLen
			hlStart = max(selColStart, spanStart) - spanStart
			hlEnd = min(selColEnd, spanEnd) - spanStart
			if hlStart < hlEnd {
				hasHighlight = true
			}
		}

		// op.Offsetで直接配置（クロージャ不要）
		stack := op.Offset(image.Pt(xOffset, 0)).Push(gtx.Ops)

		col := ev.theme.TokenColor(s.Type)
		lbl := material.Label(th, unit.Sp(13), expandedText)
		lbl.Color = col
		lbl.Font = font.Font{Typeface: "Go Mono"}
		dims := lbl.Layout(spanGtx)

		// 実際の描画幅を記録（カーソル位置計算に使用）
		ev.spanMetricsBuf[mIdx].pixelWidth = dims.Size.X

		// charWidthF のフォールバック更新
		if !ev.charWidthCached && spanVisualCols > 0 {
			ev.charWidthF = float32(dims.Size.X) / float32(spanVisualCols)
			ev.charWidthCached = true
		}

		// 選択ハイライト描画
		if hasHighlight && spanVisualCols > 0 {
			x0 := ev.scaledPixelXForPrefix(s.Text, hlStart, spanVisualOffset, dims.Size.X, spanVisualCols)
			x1 := ev.scaledPixelXForPrefix(s.Text, hlEnd, spanVisualOffset, dims.Size.X, spanVisualCols)
			hlStack := op.Offset(image.Pt(x0, 0)).Push(gtx.Ops)
			cl := clip.Rect{Max: image.Pt(x1-x0, dims.Size.Y)}.Push(gtx.Ops)
			paint.ColorOp{Color: selColor}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			cl.Pop()
			hlStack.Pop()
		}

		stack.Pop()

		if dims.Size.Y > maxHeight {
			maxHeight = dims.Size.Y
		}
		xOffset += dims.Size.X
	}

	// 検索マッチハイライト描画
	if len(ev.searchMatches) > 0 {
		for matchIdx, m := range ev.searchMatches {
			if m.Line != lineIdx {
				continue
			}
			// マッチの列範囲をピクセルに変換
			matchX0 := ev.pixelXFromColMetrics(m.Col)
			matchX1 := ev.pixelXFromColMetrics(m.Col + m.Length)
			if matchX1 > matchX0 {
				matchColor := ev.searchMatchColor
				if matchIdx == ev.searchMatchIdx {
					matchColor = ev.searchActiveColor
				}
				hs := op.Offset(image.Pt(matchX0, 0)).Push(gtx.Ops)
				hc := clip.Rect{Max: image.Pt(matchX1-matchX0, maxHeight)}.Push(gtx.Ops)
				paint.ColorOp{Color: matchColor}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				hc.Pop()
				hs.Pop()
			}
		}
	}

	// LSP診断下線描画
	diags := ev.diagnosticsForLine(lineIdx)
	for _, d := range diags {
		diagX0 := ev.pixelXFromColMetrics(d.Range.Start.Character)
		diagX1 := ev.pixelXFromColMetrics(d.Range.End.Character)
		if diagX1 <= diagX0 {
			diagX1 = diagX0 + int(ev.charWidthF*3) // 最小幅
		}
		diagColor := ev.theme.DiagError
		if d.Severity == lsp.SeverityWarning {
			diagColor = ev.theme.DiagWarning
		} else if d.Severity == lsp.SeverityInformation || d.Severity == lsp.SeverityHint {
			diagColor = ev.theme.DiagInfo
		}
		underlineH := gtx.Dp(unit.Dp(2))
		underlineY := maxHeight - underlineH
		ds := op.Offset(image.Pt(diagX0, underlineY)).Push(gtx.Ops)
		dc := clip.Rect{Max: image.Pt(diagX1-diagX0, underlineH)}.Push(gtx.Ops)
		paint.ColorOp{Color: diagColor}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		dc.Pop()
		ds.Pop()
	}

	if isCurrentLine && ev.cursorVisible {
		cursorX := ev.cursorPixelXFromMetrics(cursorCol)
		if maxHeight <= 0 {
			maxHeight = gtx.Dp(unit.Dp(18))
		}
		ev.drawCursor(gtx, cursorX, maxHeight)

		// ゴーストテキスト描画（インラインAI補完）
		if ev.ghostText != "" && ev.ghostLine == lineIdx {
			ghostX := cursorX + gtx.Dp(unit.Dp(2)) // カーソルの右
			gs := op.Offset(image.Pt(ghostX, 0)).Push(gtx.Ops)
			ghostLbl := material.Label(th, unit.Sp(13), ev.ghostText)
			ghostLbl.Color = ev.theme.GhostText
			ghostLbl.Font = font.Font{Typeface: "Go Mono"}
			ghostLbl.Layout(gtx)
			gs.Pop()
		}
	}

	return D{Size: image.Pt(xOffset, maxHeight)}
}

// cursorPixelXFromMetrics は描画済みスパンの実幅からカーソルX座標を算出する
func (ev *EditorView) cursorPixelXFromMetrics(cursorCol int) int {
	return ev.pixelXFromColMetrics(cursorCol)
}

// pixelXFromColMetrics はルーン列からピクセルX座標を算出する（スパンメトリクスベース）
func (ev *EditorView) pixelXFromColMetrics(col int) int {
	pixelX := 0
	remaining := col
	for _, m := range ev.spanMetricsBuf {
		if remaining <= m.charLen {
			// カーソルはこのスパン内 — 推定幅を実描画幅でスケーリング補正
			pixelX += ev.scaledPixelXForPrefix(m.text, remaining, m.visualOffset, m.pixelWidth, m.visualCols)
			return pixelX
		}
		pixelX += m.pixelWidth
		remaining -= m.charLen
	}
	return pixelX
}

func (ev *EditorView) drawCursor(gtx C, x, height int) {
	cursorW := gtx.Dp(unit.Dp(2))
	defer op.Offset(image.Pt(x, 0)).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: image.Pt(cursorW, height)}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: ev.theme.Cursor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func (ev *EditorView) handleKeyEvents(gtx C, state *editor.EditorState) {
	doc := state.ActiveDocument()
	if doc == nil {
		return
	}

	for {
		evt, ok := gtx.Event(
			key.FocusFilter{Target: &ev.tag},
			transfer.TargetFilter{Target: &ev.tag, Type: "application/text"},
			key.Filter{Focus: &ev.tag, Name: key.NameReturn},
			key.Filter{Focus: &ev.tag, Name: key.NameDeleteBackward},
			key.Filter{Focus: &ev.tag, Name: key.NameDeleteForward},
			key.Filter{Focus: &ev.tag, Name: key.NameLeftArrow, Optional: key.ModShift},
			key.Filter{Focus: &ev.tag, Name: key.NameRightArrow, Optional: key.ModShift},
			key.Filter{Focus: &ev.tag, Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Focus: &ev.tag, Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Focus: &ev.tag, Name: key.NameHome},
			key.Filter{Focus: &ev.tag, Name: key.NameEnd},
			key.Filter{Focus: &ev.tag, Name: key.NameTab},
			key.Filter{Focus: &ev.tag, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "X", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "V", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "S", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "Z", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "B", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "I", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "K", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "J", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "F", Required: key.ModShortcut},
			key.Filter{Focus: &ev.tag, Name: "H", Required: key.ModShortcut},
			// テキスト入力用キャッチオールフィルタ（EditEvent受信用）
			key.Filter{Focus: &ev.tag, Optional: key.ModShortcut | key.ModShift},
		)
		if !ok {
			break
		}
		switch e := evt.(type) {
		case key.Event:
			if e.State == key.Press {
				ev.handleKeyPressWithContext(gtx, e, state)
			}
		case transfer.DataEvent:
			data := e.Open()
			content, err := io.ReadAll(data)
			_ = data.Close()
			if err == nil && len(content) > 0 {
				if doc.Buffer.HasSelection() {
					doc.Buffer.DeleteSelection()
				}
				doc.Buffer.InsertText(string(content))
				state.MarkModified()
				state.NotifyDidChange()
				doc.UpdateHighlight(state.Highlighter)
			}
		case key.EditEvent:
			if e.Text != "" {
				if doc.Buffer.HasSelection() {
					doc.Buffer.DeleteSelection()
				}
				doc.Buffer.InsertText(e.Text)
				state.MarkModified()
				state.NotifyDidChange()
				doc.UpdateHighlight(state.Highlighter)
				// 補完トリガー
				ev.completionPopup.RequestCompletion(state)
			}
		}
	}
}

func expandTabs(text string, startVisualCol int) (string, int) {
	if text == "" {
		return "", 0
	}

	// 高速パス: タブなしならアロケーション不要
	if strings.IndexByte(text, '\t') < 0 {
		visualCol := startVisualCol
		for _, r := range text {
			visualCol += displayAdvance(r, visualCol)
		}
		return text, visualCol - startVisualCol
	}

	var b strings.Builder
	visualCol := startVisualCol
	for _, r := range text {
		advance := displayAdvance(r, visualCol)
		if r == '\t' {
			for i := 0; i < advance; i++ {
				b.WriteByte(' ')
			}
		} else {
			b.WriteRune(r)
		}
		visualCol += advance
	}

	return b.String(), visualCol - startVisualCol
}

func displayColsForPrefix(text string, runeCount int, startVisualCol int) int {
	if runeCount <= 0 || text == "" {
		return 0
	}

	visualCol := startVisualCol
	usedRunes := 0
	for _, r := range text {
		if usedRunes >= runeCount {
			break
		}
		visualCol += displayAdvance(r, visualCol)
		usedRunes++
	}

	return visualCol - startVisualCol
}

func displayAdvance(r rune, visualCol int) int {
	if r != '\t' {
		width := runewidth.RuneWidth(r)
		if width <= 0 {
			return 1
		}
		return width
	}
	advance := editorTabSize - (visualCol % editorTabSize)
	if advance <= 0 {
		return editorTabSize
	}
	return advance
}

func prefixRunes(text string, runeCount int) string {
	if runeCount <= 0 || text == "" {
		return ""
	}

	idx := 0
	for i := range text {
		if idx == runeCount {
			return text[:i]
		}
		idx++
	}
	return text
}

// formatLineNum は行番号を右寄せ4桁+スペースにフォーマットする（fmt.Sprintf回避）
func formatLineNum(n int) string {
	s := strconv.Itoa(n)
	const pad = "    " // 4スペース
	if len(s) < 4 {
		return pad[:4-len(s)] + s + " "
	}
	return s + " "
}

// ensureCharWidth は等幅フォントの半角・全角文字幅を一度だけ計測する
func (ev *EditorView) ensureCharWidth(gtx C, th *material.Theme) {
	if ev.charWidthCached {
		return
	}
	measureGtx := gtx
	measureGtx.Constraints.Min = image.Point{}
	measureGtx.Constraints.Max.X = 1 << 20

	// 半角(ASCII)文字の幅計測
	macro := op.Record(measureGtx.Ops)
	lbl := material.Label(th, unit.Sp(13), "M")
	lbl.Font = font.Font{Typeface: "Go Mono"}
	dims := lbl.Layout(measureGtx)
	macro.Stop()
	if dims.Size.X > 0 {
		ev.charWidthF = float32(dims.Size.X)
	}

	// 全角(CJK)文字の幅計測（フォールバックフォントの実際の幅を取得）
	macro2 := op.Record(measureGtx.Ops)
	lbl2 := material.Label(th, unit.Sp(13), "あ")
	lbl2.Font = font.Font{Typeface: "Go Mono"}
	dims2 := lbl2.Layout(measureGtx)
	macro2.Stop()
	if dims2.Size.X > 0 {
		ev.fullWidthCharF = float32(dims2.Size.X)
	} else {
		ev.fullWidthCharF = ev.charWidthF * 2
	}

	ev.charWidthCached = true
}

// runePixelWidth は文字の表示幅に応じたピクセル幅を返す
func (ev *EditorView) runePixelWidth(r rune, visualCol int) float32 {
	advance := displayAdvance(r, visualCol)
	if advance >= 2 && r != '\t' {
		// 全角文字: 計測済みの実際の全角幅を使用
		return ev.fullWidthCharF
	}
	// 半角文字・タブ: 半角幅 × カラム数
	return ev.charWidthF * float32(advance)
}

// scaledPixelXForPrefix はスパンの実描画幅で補正したプレフィックス幅を算出する
// 推定幅と実描画幅の比率でスケーリングし、フォントシェーパーとの誤差を吸収する
func (ev *EditorView) scaledPixelXForPrefix(text string, runeCount int, startVisualCol int, actualSpanWidth int, spanVisualCols int) int {
	if runeCount <= 0 || ev.charWidthF <= 0 {
		return 0
	}
	// スパン全体の推定幅
	estimatedTotal := ev.pixelXForPrefixF(text, utf8.RuneCountInString(text), startVisualCol)
	if estimatedTotal <= 0 {
		return 0
	}
	// プレフィックスの推定幅
	estimatedPrefix := ev.pixelXForPrefixF(text, runeCount, startVisualCol)
	// 実描画幅でスケーリング
	return int(estimatedPrefix * float32(actualSpanWidth) / estimatedTotal)
}

// pixelXForPrefixF はテキスト先頭からruneCount文字分の推定ピクセル幅を算出する（float32版）
func (ev *EditorView) pixelXForPrefixF(text string, runeCount int, startVisualCol int) float32 {
	if runeCount <= 0 || ev.charWidthF <= 0 {
		return 0
	}
	pxAccum := float32(0)
	visualCol := startVisualCol
	usedRunes := 0
	for _, r := range text {
		if usedRunes >= runeCount {
			break
		}
		pxAccum += ev.runePixelWidth(r, visualCol)
		visualCol += displayAdvance(r, visualCol)
		usedRunes++
	}
	return pxAccum
}

// pixelXForPrefix はテキスト先頭からruneCount文字分のピクセル幅を算出する
func (ev *EditorView) pixelXForPrefix(text string, runeCount int, startVisualCol int) int {
	if runeCount <= 0 || ev.charWidthF <= 0 {
		return 0
	}
	pxAccum := float32(0)
	visualCol := startVisualCol
	usedRunes := 0
	for _, r := range text {
		if usedRunes >= runeCount {
			break
		}
		pxAccum += ev.runePixelWidth(r, visualCol)
		visualCol += displayAdvance(r, visualCol)
		usedRunes++
	}
	return int(pxAccum)
}

// colFromPixelX はピクセルX座標から論理列を算出する（全角幅対応）
func (ev *EditorView) colFromPixelX(text string, textX float32, startVisualCol int) int {
	if ev.charWidthF <= 0 {
		return 0
	}
	pxAccum := float32(0)
	visualCol := startVisualCol
	runeIdx := 0
	for _, r := range text {
		charPxW := ev.runePixelWidth(r, visualCol)
		mid := pxAccum + charPxW/2
		if textX < mid {
			return runeIdx
		}
		pxAccum += charPxW
		visualCol += displayAdvance(r, visualCol)
		runeIdx++
	}
	return runeIdx
}

// posFromClick はピクセル座標から行・列を算出する（タブ幅を考慮、算術ベース）
func (ev *EditorView) posFromClick(gtx C, th *material.Theme, x, y int, doc *editor.Document) (line, col int) {
	if ev.lineHeightF <= 0 && len(ev.lineHits) == 0 {
		return 0, 0
	}
	line = ev.lineFromY(y, doc)
	leftPad := float32(gtx.Dp(unit.Dp(8)))
	textX := float32(x) - leftPad - ev.lineNumWidthF
	if textX <= 0 {
		col = 0
		return
	}
	lineText := doc.Buffer.Line(line)
	col = ev.colFromPixelX(lineText, textX, 0)
	return
}

func (ev *EditorView) handlePointerEvents(gtx C, state *editor.EditorState, th *material.Theme) {
	doc := state.ActiveDocument()
	if doc == nil {
		return
	}
	for {
		evt, ok := gtx.Event(
			pointer.Filter{
				Target:  &ev.tag,
				Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
				ScrollY: pointer.ScrollRange{Min: -5000, Max: 5000},
			},
		)
		if !ok {
			break
		}
		e, ok := evt.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if e.Buttons != pointer.ButtonPrimary {
				continue
			}
			ev.focused = true
			gtx.Execute(key.FocusCmd{Tag: &ev.tag})
			if state.ShowOmniChat {
				state.ShowOmniChat = false
			}

			// 折畳マーカークリック判定（行番号左側の領域）
			foldAreaW := float32(gtx.Dp(unit.Dp(22)))
			if e.Position.X < foldAreaW {
				line, _ := ev.posFromClick(gtx, th, int(e.Position.X), int(e.Position.Y), doc)
				if doc.Buffer.FoldState.IsFoldable(line) {
					doc.Buffer.FoldState.ToggleFold(line)
					continue
				}
			}

			ev.dragging = true
			ev.pressedPID = e.PointerID
			ev.cursorVisible = true
			ev.lastBlink = time.Now()

			line, col := ev.posFromClick(gtx, th, int(e.Position.X), int(e.Position.Y), doc)
			doc.Buffer.ClearSelection()
			doc.Buffer.SetCursorLineCol(line, col)
			// 選択開始位置を記録
			doc.Buffer.SetSelection(doc.Buffer.CursorPos(), doc.Buffer.CursorPos())
			// マウスをグラブしてドラッグイベントを取得
			gtx.Execute(pointer.GrabCmd{Tag: &ev.tag, ID: e.PointerID})

		case pointer.Drag:
			if !ev.dragging || e.PointerID != ev.pressedPID {
				continue
			}
			line, col := ev.posFromClick(gtx, th, int(e.Position.X), int(e.Position.Y), doc)
			doc.Buffer.SetCursorLineCol(line, col)
			// 選択終了位置を更新（開始位置は維持）
			anchor := doc.Buffer.SelectionAnchor()
			doc.Buffer.SetSelection(anchor, doc.Buffer.CursorPos())

			ev.cursorVisible = true
			ev.lastBlink = time.Now()

		case pointer.Scroll:
			// トラックパッド/マウスホイールスクロール処理
			ev.scrollAcc += e.Scroll.Y
			delta := int(ev.scrollAcc)
			ev.scrollAcc -= float32(delta)
			ev.list.Position.Offset += delta

		case pointer.Release, pointer.Cancel:
			ev.dragging = false
			// 選択が空なら解除
			if !doc.Buffer.HasSelection() {
				doc.Buffer.ClearSelection()
			}
		}
	}
}

func (ev *EditorView) handleKeyPress(e key.Event, state *editor.EditorState) {
	doc := state.ActiveDocument()
	if doc == nil {
		return
	}

	// カーソル点滅リセット
	ev.cursorVisible = true
	ev.lastBlink = time.Now()

	// ゴーストテキスト（AIインライン補完）の処理
	if state.AIGhostText != "" {
		if e.Name == key.NameTab && e.Modifiers == 0 {
			// Tab: ゴーストテキストを受け入れ
			if state.AIAcceptGhostText() {
				return
			}
		}
		if e.Name == key.NameEscape {
			// Escape: ゴーストテキストを破棄
			state.AIDismissGhostText()
			return
		}
		// その他のキー入力でゴーストテキストを破棄
		state.AIDismissGhostText()
	}

	// 補完ポップアップへのキーイベント転送
	if ev.completionPopup.IsVisible() {
		if e.Name == key.NameReturn || e.Name == key.NameTab {
			// 補完確定
			item := ev.completionPopup.SelectedItem()
			if item != nil {
				// カーソル位置の単語プレフィックスを削除してから挿入
				prefix := currentWordPrefix(doc)
				for range prefix {
					doc.Buffer.DeleteBackward()
				}
				insertText := item.InsertText
				if insertText == "" {
					insertText = item.Label
				}
				doc.Buffer.InsertText(insertText)
				state.MarkModified()
				state.NotifyDidChange()
				doc.UpdateHighlight(state.Highlighter)
			}
			ev.completionPopup.Hide()
			return
		}
		if ev.completionPopup.HandleKey(e) {
			return
		}
	}

	switch e.Name {
	case key.NameReturn:
		if doc.Buffer.HasSelection() {
			doc.Buffer.DeleteSelection()
		}
		doc.Buffer.InsertNewline()
		state.MarkModified()
		state.NotifyDidChange()
		doc.UpdateHighlight(state.Highlighter)
	case key.NameDeleteBackward:
		if doc.Buffer.HasSelection() {
			doc.Buffer.DeleteSelection()
		} else {
			doc.Buffer.DeleteBackward()
		}
		state.MarkModified()
		state.NotifyDidChange()
		doc.UpdateHighlight(state.Highlighter)
		// 補完更新
		ev.completionPopup.RequestCompletion(state)
	case key.NameDeleteForward:
		if doc.Buffer.HasSelection() {
			doc.Buffer.DeleteSelection()
		} else {
			doc.Buffer.DeleteForward()
		}
		state.MarkModified()
		state.NotifyDidChange()
		doc.UpdateHighlight(state.Highlighter)
	case key.NameLeftArrow:
		doc.Buffer.ClearSelection()
		doc.Buffer.MoveCursorLeft()
		ev.completionPopup.Hide()
	case key.NameRightArrow:
		doc.Buffer.ClearSelection()
		doc.Buffer.MoveCursorRight()
		ev.completionPopup.Hide()
	case key.NameUpArrow:
		doc.Buffer.ClearSelection()
		doc.Buffer.MoveCursorUp()
		ev.completionPopup.Hide()
	case key.NameDownArrow:
		doc.Buffer.ClearSelection()
		doc.Buffer.MoveCursorDown()
		ev.completionPopup.Hide()
	case key.NameHome:
		doc.Buffer.ClearSelection()
		doc.Buffer.MoveCursorHome()
	case key.NameEnd:
		doc.Buffer.ClearSelection()
		doc.Buffer.MoveCursorEnd()
	case key.NameTab:
		if doc.Buffer.HasSelection() {
			doc.Buffer.DeleteSelection()
		}
		doc.Buffer.InsertText("\t")
		state.MarkModified()
		state.NotifyDidChange()
		doc.UpdateHighlight(state.Highlighter)
	case "S":
		if e.Modifiers.Contain(key.ModShortcut) {
			_ = state.SaveActiveFile()
		}
	case "Z":
		if e.Modifiers.Contain(key.ModShortcut) {
			doc.Buffer.Undo()
			doc.Modified = true
			state.NotifyDidChange()
			doc.UpdateHighlight(state.Highlighter)
		}
	case "B":
		if e.Modifiers.Contain(key.ModShortcut) {
			state.ToggleSidebar()
		}
	case "I":
		if e.Modifiers.Contain(key.ModShortcut) {
			state.OmniMode = editor.ModeAI
			state.ShowOmniChat = true
		}
	case "K":
		if e.Modifiers.Contain(key.ModShortcut) {
			state.OmniMode = editor.ModeCmd
			state.ShowOmniChat = false
		}
	case "J":
		if e.Modifiers.Contain(key.ModShortcut) {
			// ターミナル表示/非表示トグル
			state.ShowTerminal = !state.ShowTerminal
			if state.ShowTerminal && state.Terminal != nil && state.Terminal.Count() == 0 {
				if _, err := state.Terminal.NewTerminal(24, 80); err != nil {
					log.Printf("[ターミナル] 起動失敗: %v", err)
				}
			}
			if state.ShowTerminal {
				state.TerminalFocusRequest = true
			}
		}
	case "F":
		if e.Modifiers.Contain(key.ModShortcut) {
			state.Search.Active = true
		}
	case "H":
		if e.Modifiers.Contain(key.ModShortcut) {
			state.Search.Active = true
		}
	}
}

func (ev *EditorView) handleKeyPressWithContext(gtx C, e key.Event, state *editor.EditorState) {
	doc := state.ActiveDocument()
	if doc == nil {
		return
	}

	if e.State != key.Press {
		return
	}

	switch e.Name {
	case "A":
		if e.Modifiers.Contain(key.ModShortcut) {
			doc.Buffer.SetSelection(0, doc.Buffer.Length())
			doc.Buffer.SetCursorPos(doc.Buffer.Length())
			return
		}
	case "C":
		if e.Modifiers.Contain(key.ModShortcut) {
			ev.copySelection(gtx, doc)
			return
		}
	case "X":
		if e.Modifiers.Contain(key.ModShortcut) {
			if ev.copySelection(gtx, doc) {
				doc.Buffer.DeleteSelection()
				state.MarkModified()
				doc.UpdateHighlight(state.Highlighter)
			}
			return
		}
	case "V":
		if e.Modifiers.Contain(key.ModShortcut) {
			gtx.Execute(clipboard.ReadCmd{Tag: &ev.tag})
			return
		}
	}

	ev.handleKeyPress(e, state)
}

func (ev *EditorView) copySelection(gtx C, doc *editor.Document) bool {
	if !doc.Buffer.HasSelection() {
		return false
	}
	start, end := doc.Buffer.Selection()
	text := doc.Buffer.Text()[start:end]
	if text == "" {
		return false
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	return true
}

func (ev *EditorView) layoutWelcome(gtx C, th *material.Theme) D {
	fillBackground(gtx, ev.theme.Background, gtx.Constraints.Max)
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				lbl := material.Label(th, unit.Sp(28), "Catana")
				lbl.Color = ev.theme.Accent
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
					lbl := material.Label(th, unit.Sp(14), "次世代ネイティブコードエディタ")
					lbl.Color = ev.theme.TextMuted
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: unit.Dp(24)}.Layout(gtx, func(gtx C) D {
					lbl := material.Label(th, unit.Sp(12), "ファイルツリーからファイルを選択して開始")
					lbl.Color = ev.theme.TextDark
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}

// handleKeyPress メソッド内のシャドウを避けるためリネーム
func (view *EditorView) processKeyEvent(e key.Event, state *editor.EditorState) {
	view.handleKeyPress(e, state)
}

// cursorScreenPos はカーソルの画面座標を返す
func (ev *EditorView) cursorScreenPos(doc *editor.Document) (int, int) {
	cursorLine := doc.Buffer.CursorLine()
	cursorCol := doc.Buffer.CursorCol()

	// 行の画面Y座標を探す
	cursorY := 0
	for _, hit := range ev.lineHits {
		if hit.line == cursorLine {
			cursorY = hit.top
			break
		}
	}

	// X座標はスパンメトリクスから算出
	insetH := 8 // dp近似
	cursorX := insetH + int(ev.lineNumWidthF) + ev.pixelXFromColMetrics(cursorCol)

	return cursorX, cursorY
}

// updateDiagnostics はLSP診断キャッシュを更新する
func (ev *EditorView) updateDiagnostics(state *editor.EditorState, doc *editor.Document) {
	if state.LSP == nil || doc.FilePath == "" {
		ev.diagnostics = nil
		return
	}

	uri := lsp.FilePathToURI(doc.FilePath)
	version := state.LSP.DiagVersion()
	if uri == ev.diagURI && version == ev.diagVersion {
		return // 変更なし
	}

	ev.diagURI = uri
	ev.diagVersion = version
	ev.diagnostics = state.LSP.GetDiagnostics(uri)
}

// diagnosticsForLine は指定行の診断を返す
func (ev *EditorView) diagnosticsForLine(lineIdx int) []lsp.Diagnostic {
	if len(ev.diagnostics) == 0 {
		return nil
	}
	var result []lsp.Diagnostic
	for _, d := range ev.diagnostics {
		if d.Range.Start.Line == lineIdx {
			result = append(result, d)
		}
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// unused はコンパイラの使用されていない変数警告を抑制する
var _ color.NRGBA

// layoutDiff はDiffタブの内容を描画する
func (ev *EditorView) layoutDiff(gtx C, th *material.Theme, doc *editor.Document) D {
	// diff行をフラットに展開
	type displayLine struct {
		diffLine git.DiffLine
		isHunk   bool
		hunkText string
	}
	var lines []displayLine

	for _, fd := range doc.DiffData {
		for _, hunk := range fd.Hunks {
			hunkLabel := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount)
			lines = append(lines, displayLine{isHunk: true, hunkText: hunkLabel})
			for _, dl := range hunk.Lines {
				lines = append(lines, displayLine{diffLine: dl})
			}
		}
	}

	if len(lines) == 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			lbl := material.Label(th, unit.Sp(13), "差分なし")
			lbl.Color = ev.theme.TextDark
			return lbl.Layout(gtx)
		})
	}

	viewportSize := gtx.Constraints.Max
	return withBg(gtx, func(gtx C, _ image.Point) {
		fillBackground(gtx, ev.theme.Background, viewportSize)
	}, func(gtx C) D {
		dims := material.List(th, &ev.diffList).Layout(gtx, len(lines), func(gtx C, i int) D {
			dl := lines[i]
			lineH := gtx.Dp(unit.Dp(20))
			gtx.Constraints.Min.Y = lineH
			gtx.Constraints.Max.Y = lineH

			if dl.isHunk {
				return withFlatBg(gtx, ev.theme.DiffHunkBg, func(gtx C) D {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return layout.Center.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(11), dl.hunkText)
							lbl.Color = ev.theme.TextMuted
							return lbl.Layout(gtx)
						})
					})
				})
			}

			// diff行の背景色とプレフィックス
			var bgCol color.NRGBA
			var prefixCol color.NRGBA
			prefix := " "
			switch dl.diffLine.Type {
			case git.DiffAdded:
				bgCol = ev.theme.DiffAddedBg
				prefixCol = ev.theme.GitAdded
				prefix = "+"
			case git.DiffDeleted:
				bgCol = ev.theme.DiffDeletedBg
				prefixCol = ev.theme.GitDeleted
				prefix = "-"
			default:
				bgCol = color.NRGBA{}
				prefixCol = ev.theme.TextDark
			}

			return withFlatBg(gtx, bgCol, func(gtx C) D {
				return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// 行番号（変更前）
						layout.Rigid(func(gtx C) D {
							w := gtx.Dp(unit.Dp(40))
							gtx.Constraints.Min.X = w
							gtx.Constraints.Max.X = w
							numStr := ""
							if dl.diffLine.OldNum > 0 {
								numStr = fmt.Sprintf("%d", dl.diffLine.OldNum)
							}
							lbl := material.Label(th, unit.Sp(11), numStr)
							lbl.Color = ev.theme.LineNumber
							lbl.Alignment = 2 // End
							return lbl.Layout(gtx)
						}),
						// 行番号（変更後）
						layout.Rigid(func(gtx C) D {
							w := gtx.Dp(unit.Dp(40))
							gtx.Constraints.Min.X = w
							gtx.Constraints.Max.X = w
							numStr := ""
							if dl.diffLine.NewNum > 0 {
								numStr = fmt.Sprintf("%d", dl.diffLine.NewNum)
							}
							lbl := material.Label(th, unit.Sp(11), numStr)
							lbl.Color = ev.theme.LineNumber
							lbl.Alignment = 2 // End
							return lbl.Layout(gtx)
						}),
						// プレフィックス（+/-/スペース）
						layout.Rigid(func(gtx C) D {
							w := gtx.Dp(unit.Dp(16))
							gtx.Constraints.Min.X = w
							gtx.Constraints.Max.X = w
							lbl := material.Label(th, unit.Sp(11), prefix)
							lbl.Color = prefixCol
							return layout.Center.Layout(gtx, lbl.Layout)
						}),
						// コンテンツ
						layout.Flexed(1, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(11), dl.diffLine.Content)
							lbl.Color = ev.theme.Text
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						}),
					)
				})
			})
		})
		dims.Size = viewportSize
		return dims
	})
}
