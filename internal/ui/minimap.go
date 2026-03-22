package ui

import (
	"catana/internal/editor"
	"catana/internal/syntax"
	"image"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	minimapWidthDp  = 80 // ミニマップ幅
	minimapLineH    = 2  // 1行の高さ（ピクセル）
	minimapCharW    = 1  // 1文字の幅（ピクセル）
	minimapMaxChars = 80 // 表示する最大文字数
)

// Minimap はコード全体のサムネイル表示
type Minimap struct {
	theme      *Theme
	tag        bool
	dragging   bool
	pressedPID pointer.ID
}

// NewMinimap は新しいミニマップを作成する
func NewMinimap(theme *Theme) *Minimap {
	return &Minimap{
		theme: theme,
	}
}

// Layout はミニマップを描画する
func (mm *Minimap) Layout(gtx C, state *editor.EditorState, th *material.Theme, listFirst int, viewportLines int) D {
	doc := state.ActiveDocument()
	if doc == nil {
		return D{}
	}

	lineCount := doc.Buffer.LineCount()
	if lineCount <= 0 {
		return D{}
	}

	width := gtx.Dp(unit.Dp(minimapWidthDp))
	maxH := gtx.Constraints.Max.Y
	lineH := gtx.Dp(unit.Dp(minimapLineH))
	if lineH <= 0 {
		lineH = 1
	}

	totalH := lineCount * lineH
	if totalH > maxH {
		totalH = maxH
	}

	// 背景
	fillBackground(gtx, mm.theme.SurfaceDark, image.Pt(width, totalH))

	// スケーリングファクター
	scale := float32(1.0)
	if lineCount*lineH > maxH {
		scale = float32(maxH) / float32(lineCount*lineH)
	}

	// ビューポートインジケーター
	vpTop := int(float32(listFirst*lineH) * scale)
	vpHeight := int(float32(viewportLines*lineH) * scale)
	if vpHeight < lineH*2 {
		vpHeight = lineH * 2
	}
	if vpTop+vpHeight > totalH {
		vpTop = totalH - vpHeight
	}
	if vpTop < 0 {
		vpTop = 0
	}

	// ビューポート背景
	vpOff := op.Offset(image.Pt(0, vpTop)).Push(gtx.Ops)
	vpCl := clip.Rect{Max: image.Pt(width, vpHeight)}.Push(gtx.Ops)
	paint.ColorOp{Color: mm.theme.Separator}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	vpCl.Pop()
	vpOff.Pop()

	// コード行のサムネイル描画
	charW := gtx.Dp(unit.Dp(minimapCharW))
	if charW <= 0 {
		charW = 1
	}
	foldState := doc.Buffer.FoldState

	visibleIdx := 0
	for i := 0; i < lineCount; i++ {
		if foldState.IsHidden(i) {
			continue
		}
		y := int(float32(visibleIdx*lineH) * scale)
		if y >= totalH {
			break
		}
		visibleIdx++

		line := doc.Buffer.Line(i)
		if len(line) == 0 {
			continue
		}

		// スパンカラーでミニマップ描画
		var spans []syntax.Span
		if i < len(doc.HighlightedLines) {
			spans = doc.HighlightedLines[i]
		}
		if len(spans) == 0 {
			spans = []syntax.Span{{Text: line, Type: syntax.TokenPlain}}
		}

		x := 0
		for _, s := range spans {
			col := mm.theme.TokenColor(s.Type)
			col.A = col.A / 2 // 半透明にする
			runeCount := utf8.RuneCountInString(s.Text)
			w := runeCount * charW
			if x+w > width {
				w = width - x
			}
			if w <= 0 {
				break
			}
			lOff := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			lCl := clip.Rect{Max: image.Pt(w, lineH)}.Push(gtx.Ops)
			paint.ColorOp{Color: col}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			lCl.Pop()
			lOff.Pop()
			x += w
			if x >= width {
				break
			}
		}
	}

	// ポインターイベント処理（クリック/ドラッグでスクロール）
	mm.handlePointerEvents(gtx, state, lineCount, totalH, scale, lineH)

	// イベント登録
	defer clip.Rect(image.Rect(0, 0, width, totalH)).Push(gtx.Ops).Pop()
	pointer.CursorPointer.Add(gtx.Ops)
	event.Op(gtx.Ops, &mm.tag)

	return D{Size: image.Pt(width, totalH)}
}

func (mm *Minimap) handlePointerEvents(gtx C, state *editor.EditorState, lineCount, totalH int, scale float32, lineH int) {
	for {
		evt, ok := gtx.Event(
			pointer.Filter{
				Target: &mm.tag,
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
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
			mm.dragging = true
			mm.pressedPID = e.PointerID
			gtx.Execute(pointer.GrabCmd{Tag: &mm.tag, ID: e.PointerID})
			mm.scrollToPosition(state, e.Position.Y, lineCount, scale, lineH)
		case pointer.Drag:
			if !mm.dragging || e.PointerID != mm.pressedPID {
				continue
			}
			mm.scrollToPosition(state, e.Position.Y, lineCount, scale, lineH)
		case pointer.Release, pointer.Cancel:
			mm.dragging = false
		}
	}
}

func (mm *Minimap) scrollToPosition(state *editor.EditorState, y float32, lineCount int, scale float32, lineH int) {
	if scale <= 0 || lineH <= 0 {
		return
	}
	targetLine := int(y / scale / float32(lineH))
	if targetLine < 0 {
		targetLine = 0
	}
	if targetLine >= lineCount {
		targetLine = lineCount - 1
	}
	// EditorViewのスクロール位置を設定するためにカーソルを移動
	doc := state.ActiveDocument()
	if doc != nil {
		doc.Buffer.SetCursorLineCol(targetLine, 0)
	}
}
