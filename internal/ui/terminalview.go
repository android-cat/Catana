package ui

import (
	"catana/internal/editor"
	"catana/internal/terminal"
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TerminalView はターミナルパネルのUI
type TerminalView struct {
	theme   *Theme
	tag     bool
	focused bool
	// ターミナルセル描画サイズ
	cellW float32
	cellH float32
	sized bool
	// 初回フォーカス要求
	needFocus bool
	// タブバーボタン
	btnClose widget.Clickable // パネル閉じるボタン
}

// NewTerminalView は新しいTerminalViewを作成する
func NewTerminalView(theme *Theme) *TerminalView {
	return &TerminalView{
		theme: theme,
	}
}

// RequestFocus はターミナルにフォーカスを要求する
func (tv *TerminalView) RequestFocus() {
	tv.needFocus = true
}

// IsFocused はターミナルにフォーカスがあるかを返す
func (tv *TerminalView) IsFocused() bool {
	return tv.focused
}

// Layout はターミナルパネルを描画する
func (tv *TerminalView) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	// 閉じるボタンクリック処理
	for tv.btnClose.Clicked(gtx) {
		state.ShowTerminal = false
	}

	size := gtx.Constraints.Max
	fillBackground(gtx, tv.theme.TerminalBg, size)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// ターミナルタブバー
		layout.Rigid(func(gtx C) D {
			return tv.layoutTabBar(gtx, state, th)
		}),
		// ターミナル本体
		layout.Flexed(1, func(gtx C) D {
			return tv.layoutTerminalBody(gtx, state, th)
		}),
	)
}

// layoutTabBar はターミナルタブバーを描画する
func (tv *TerminalView) layoutTabBar(gtx C, state *editor.EditorState, th *material.Theme) D {
	h := gtx.Dp(unit.Dp(28))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	fillBackground(gtx, tv.theme.PanelBg, image.Pt(gtx.Constraints.Max.X, h))

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// タイトル
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(11), "ターミナル")
				lbl.Color = tv.theme.Text
				return lbl.Layout(gtx)
			})
		}),
		// ターミナルタブ
		layout.Rigid(func(gtx C) D {
			if state.Terminal == nil {
				return D{}
			}
			count := state.Terminal.Count()
			if count == 0 {
				return D{}
			}
			activeIdx := state.Terminal.ActiveIndex()
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				var children []layout.FlexChild
				for i := 0; i < count; i++ {
					idx := i
					children = append(children, layout.Rigid(func(gtx C) D {
						return tv.layoutTab(gtx, th, idx, idx == activeIdx)
					}))
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			})
		}),
		// スペーサー
		layout.Flexed(1, func(gtx C) D {
			return D{Size: image.Pt(0, h)}
		}),
		// 新規ターミナルボタン
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(12), "+")
				lbl.Color = tv.theme.TextMuted
				return lbl.Layout(gtx)
			})
		}),
		// 閉じるボタン
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				return tv.btnClose.Layout(gtx, func(gtx C) D {
					lbl := material.Label(th, unit.Sp(12), "✕")
					lbl.Color = tv.theme.TextMuted
					return lbl.Layout(gtx)
				})
			})
		}),
	)
}

func (tv *TerminalView) layoutTab(gtx C, th *material.Theme, idx int, active bool) D {
	return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
		bgColor := color.NRGBA{A: 0}
		if active {
			bgColor = tv.theme.AccentBg
		}
		label := fmt.Sprintf("zsh-%d", idx+1)
		return withFlatBg(gtx, bgColor, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				col := tv.theme.TextMuted
				if active {
					col = tv.theme.Text
				}
				lbl := material.Label(th, unit.Sp(10), label)
				lbl.Color = col
				return lbl.Layout(gtx)
			})
		})
	})
}

// layoutTerminalBody はターミナル本体を描画する
func (tv *TerminalView) layoutTerminalBody(gtx C, state *editor.EditorState, th *material.Theme) D {
	size := gtx.Constraints.Max

	// 入力イベント用にタグをクリップ領域に登録（イベントハンドラより先に）
	areaStack := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &tv.tag)

	// フォーカス要求（ターミナル初回表示時 or EditorState経由）
	if tv.needFocus || state.TerminalFocusRequest {
		tv.needFocus = false
		state.TerminalFocusRequest = false
		tv.focused = true
		gtx.Execute(key.FocusCmd{Tag: &tv.tag})
	}

	// キー入力処理
	tv.handleTerminalKeys(gtx, state)

	// ポインターイベント処理
	tv.handleTerminalPointer(gtx, state)

	// セル幅の計測（一度だけ）
	if !tv.sized {
		tv.measureCell(gtx, th)
	}

	term := tv.activeTerm(state)
	if term == nil {
		// 「ターミナル未起動」メッセージ
		return layout.Center.Layout(gtx, func(gtx C) D {
			lbl := material.Label(th, unit.Sp(12), "Cmd+J でターミナルを開く")
			lbl.Color = tv.theme.TextDark
			return lbl.Layout(gtx)
		})
	}

	// 画面サイズに基づくリサイズ
	if tv.cellW > 0 && tv.cellH > 0 {
		newCols := int(float32(size.X) / tv.cellW)
		newRows := int(float32(size.Y) / tv.cellH)
		if newCols < 10 {
			newCols = 10
		}
		if newRows < 2 {
			newRows = 2
		}
		rows, cols := term.Size()
		if rows != newRows || cols != newCols {
			term.Resize(newRows, newCols)
		}
	}

	// ターミナル画面スナップショット取得
	cells, cursorR, cursorC := term.GetScreen()
	if len(cells) == 0 {
		return D{Size: size}
	}

	// セル描画
	for r := range cells {
		for c := range cells[r] {
			cell := cells[r][c]
			x := int(float32(c) * tv.cellW)
			y := int(float32(r) * tv.cellH)
			w := int(tv.cellW)
			h := int(tv.cellH)

			// 背景色（デフォルト以外の場合描画）
			if cell.BG >= 0 {
				bgStack := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
				bgClip := clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops)
				paint.ColorOp{Color: tv.ansiColor(cell.BG)}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				bgClip.Pop()
				bgStack.Pop()
			}

			// 文字描画
			ch := cell.Char
			if ch == 0 || ch == ' ' {
				continue
			}

			fgColor := tv.theme.Text
			if cell.FG >= 0 {
				fgColor = tv.ansiColor(cell.FG)
			}

			cellStack := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			lbl := material.Label(th, unit.Sp(12), string(ch))
			lbl.Color = fgColor
			lbl.Font = font.Font{Typeface: "Go Mono"}
			if cell.Bold {
				lbl.Font.Weight = font.Bold
			}
			cellGtx := gtx
			cellGtx.Constraints.Min = image.Point{}
			cellGtx.Constraints.Max = image.Pt(w, h)
			lbl.Layout(cellGtx)
			cellStack.Pop()
		}
	}

	// カーソル描画
	if cursorR >= 0 && cursorR < len(cells) && cursorC >= 0 {
		cx := int(float32(cursorC) * tv.cellW)
		cy := int(float32(cursorR) * tv.cellH)
		cw := int(tv.cellW)
		ch := int(tv.cellH)
		curStack := op.Offset(image.Pt(cx, cy)).Push(gtx.Ops)
		curClip := clip.Rect{Max: image.Pt(cw, ch)}.Push(gtx.Ops)
		cursorCol := tv.theme.Accent
		cursorCol.A = 100
		paint.ColorOp{Color: cursorCol}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		curClip.Pop()
		curStack.Pop()
	}

	// 入力エリア登録のクリップスタック解放
	areaStack.Pop()

	// 再描画要求
	gtx.Execute(op.InvalidateCmd{})

	return D{Size: size}
}

func (tv *TerminalView) measureCell(gtx C, th *material.Theme) {
	measureGtx := gtx
	measureGtx.Constraints.Min = image.Point{}
	measureGtx.Constraints.Max.X = 1 << 20
	macro := op.Record(measureGtx.Ops)
	lbl := material.Label(th, unit.Sp(12), "M")
	lbl.Font = font.Font{Typeface: "Go Mono"}
	dims := lbl.Layout(measureGtx)
	macro.Stop()
	if dims.Size.X > 0 && dims.Size.Y > 0 {
		tv.cellW = float32(dims.Size.X)
		tv.cellH = float32(dims.Size.Y)
		tv.sized = true
	}
}

func (tv *TerminalView) activeTerm(state *editor.EditorState) *terminal.Terminal {
	if state.Terminal == nil {
		return nil
	}
	return state.Terminal.ActiveTerminal()
}

// handleTerminalKeys はターミナルへのキー入力を処理する
func (tv *TerminalView) handleTerminalKeys(gtx C, state *editor.EditorState) {
	for {
		evt, ok := gtx.Event(
			key.FocusFilter{Target: &tv.tag},
			key.Filter{Focus: &tv.tag, Name: key.NameReturn},
			key.Filter{Focus: &tv.tag, Name: key.NameDeleteBackward},
			key.Filter{Focus: &tv.tag, Name: key.NameTab},
			key.Filter{Focus: &tv.tag, Name: key.NameUpArrow},
			key.Filter{Focus: &tv.tag, Name: key.NameDownArrow},
			key.Filter{Focus: &tv.tag, Name: key.NameLeftArrow},
			key.Filter{Focus: &tv.tag, Name: key.NameRightArrow},
			key.Filter{Focus: &tv.tag, Name: key.NameEscape},
			key.Filter{Focus: &tv.tag, Name: "C", Required: key.ModCtrl},
			key.Filter{Focus: &tv.tag, Name: "D", Required: key.ModCtrl},
			key.Filter{Focus: &tv.tag, Name: "L", Required: key.ModCtrl},
			key.Filter{Focus: &tv.tag, Name: "A", Required: key.ModCtrl},
			key.Filter{Focus: &tv.tag, Name: "E", Required: key.ModCtrl},
			// Cmd+J でターミナルパネルを隠す
			key.Filter{Focus: &tv.tag, Name: "J", Required: key.ModShortcut},
			// テキスト入力
			key.Filter{Focus: &tv.tag, Optional: key.ModShift},
		)
		if !ok {
			break
		}

		term := tv.activeTerm(state)
		if term == nil {
			continue
		}

		switch e := evt.(type) {
		case key.FocusEvent:
			tv.focused = e.Focus
		case key.Event:
			if e.State != key.Press {
				continue
			}
			// Cmd+J でターミナルパネルを隠す
			if e.Name == "J" && e.Modifiers.Contain(key.ModShortcut) {
				state.ShowTerminal = false
				continue
			}
			data := tv.keyToTerminal(e)
			if data != "" {
				_ = term.WriteString(data)
			}
		case key.EditEvent:
			if e.Text != "" {
				_ = term.WriteString(e.Text)
			}
		}
	}
}

// keyToTerminal はGioキーイベントをターミナルエスケープシーケンスに変換する
func (tv *TerminalView) keyToTerminal(e key.Event) string {
	// Ctrlキー組み合わせ
	if e.Modifiers.Contain(key.ModCtrl) {
		switch e.Name {
		case "C":
			return "\x03"
		case "D":
			return "\x04"
		case "L":
			return "\x0c"
		case "A":
			return "\x01"
		case "E":
			return "\x05"
		}
	}

	switch e.Name {
	case key.NameReturn:
		return "\r"
	case key.NameDeleteBackward:
		return "\x7f"
	case key.NameTab:
		return "\t"
	case key.NameUpArrow:
		return "\x1b[A"
	case key.NameDownArrow:
		return "\x1b[B"
	case key.NameRightArrow:
		return "\x1b[C"
	case key.NameLeftArrow:
		return "\x1b[D"
	case key.NameEscape:
		return "\x1b"
	}
	return ""
}

// handleTerminalPointer はターミナルのポインターイベントを処理する
func (tv *TerminalView) handleTerminalPointer(gtx C, state *editor.EditorState) {
	for {
		evt, ok := gtx.Event(
			pointer.Filter{
				Target: &tv.tag,
				Kinds:  pointer.Press,
			},
		)
		if !ok {
			break
		}
		if e, ok := evt.(pointer.Event); ok {
			if e.Kind == pointer.Press {
				tv.focused = true
				gtx.Execute(key.FocusCmd{Tag: &tv.tag})
			}
		}
	}
}

// ansiColor はANSI 256色パレットから色を返す
func (tv *TerminalView) ansiColor(idx int) color.NRGBA {
	if idx < 0 || idx > 255 {
		return tv.theme.Text
	}
	// 標準16色
	if idx < 16 {
		return tv.theme.TerminalANSI[idx]
	}
	// 216色キューブ (16-231)
	if idx < 232 {
		i := idx - 16
		b := i % 6
		g := (i / 6) % 6
		r := i / 36
		return nrgba(
			uint8(r*255/5),
			uint8(g*255/5),
			uint8(b*255/5),
			255,
		)
	}
	// グレースケール (232-255)
	v := uint8((idx-232)*10 + 8)
	return nrgba(v, v, v, 255)
}
