package ui

import (
	"catana/internal/editor"
	"fmt"
	"image"
	"io"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// SearchBar はファイル内検索バーUI
type SearchBar struct {
	theme         *Theme
	queryEditor   widget.Editor
	replaceEditor widget.Editor
	btnNext       widget.Clickable
	btnPrev       widget.Clickable
	btnClose      widget.Clickable
	btnReplace    widget.Clickable
	btnReplaceAll widget.Clickable
	btnRegex      widget.Clickable
	btnCase       widget.Clickable
	showReplace   bool
	btnToggleRepl widget.Clickable
	tag           bool
}

// NewSearchBar は新しい検索バーを作成する
func NewSearchBar(theme *Theme) *SearchBar {
	sb := &SearchBar{
		theme: theme,
	}
	sb.queryEditor.SingleLine = true
	sb.replaceEditor.SingleLine = true
	return sb
}

// Layout は検索バーを描画する
func (sb *SearchBar) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	if !state.Search.Active {
		return D{}
	}

	// キーイベント処理
	sb.handleEvents(gtx, state, th)

	barW := gtx.Constraints.Max.X
	if barW > gtx.Dp(unit.Dp(480)) {
		barW = gtx.Dp(unit.Dp(480))
	}

	searchGtx := gtx
	searchGtx.Constraints.Min.X = barW
	searchGtx.Constraints.Max.X = barW

	return sb.layoutBar(searchGtx, state, th)
}

func (sb *SearchBar) layoutBar(gtx C, state *editor.EditorState, th *material.Theme) D {
	bgColor := sb.theme.SurfaceAlt
	borderColor := sb.theme.Border

	return withBg(gtx, func(gtx C, sz image.Point) {
		// 背景
		fillBackground(gtx, bgColor, sz)
		// 下ボーダー
		defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
		fillBackground(gtx, borderColor, image.Pt(sz.X, 1))
	}, func(gtx C) D {
		return layout.Inset{
			Top: unit.Dp(6), Bottom: unit.Dp(6),
			Left: unit.Dp(8), Right: unit.Dp(8),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// 検索行
				layout.Rigid(func(gtx C) D {
					return sb.layoutSearchRow(gtx, state, th)
				}),
				// 置換行（展開時のみ）
				layout.Rigid(func(gtx C) D {
					if !sb.showReplace {
						return D{}
					}
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						return sb.layoutReplaceRow(gtx, state, th)
					})
				}),
			)
		})
	})
}

func (sb *SearchBar) layoutSearchRow(gtx C, state *editor.EditorState, th *material.Theme) D {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// 置換展開ボタン
		layout.Rigid(func(gtx C) D {
			for sb.btnToggleRepl.Clicked(gtx) {
				sb.showReplace = !sb.showReplace
			}
			return sb.btnToggleRepl.Layout(gtx, func(gtx C) D {
				arrow := "▶"
				if sb.showReplace {
					arrow = "▼"
				}
				lbl := material.Label(th, unit.Sp(10), arrow)
				lbl.Color = sb.theme.TextMuted
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
					return lbl.Layout(gtx)
				})
			})
		}),
		// 検索入力フィールド
		layout.Flexed(1, func(gtx C) D {
			return sb.layoutInputField(gtx, th, &sb.queryEditor, "検索...")
		}),
		// 正規表現トグル
		layout.Rigid(func(gtx C) D {
			for sb.btnRegex.Clicked(gtx) {
				state.Search.IsRegex = !state.Search.IsRegex
				sb.executeSearch(state)
			}
			return sb.toggleButton(gtx, th, &sb.btnRegex, ".*", state.Search.IsRegex)
		}),
		// 大文字小文字トグル
		layout.Rigid(func(gtx C) D {
			for sb.btnCase.Clicked(gtx) {
				state.Search.CaseSensitive = !state.Search.CaseSensitive
				sb.executeSearch(state)
			}
			return sb.toggleButton(gtx, th, &sb.btnCase, "Aa", state.Search.CaseSensitive)
		}),
		// マッチカウント表示
		layout.Rigid(func(gtx C) D {
			text := "該当なし"
			if len(state.Search.Matches) > 0 {
				text = fmt.Sprintf("%d/%d", state.Search.CurrentMatch+1, len(state.Search.Matches))
			}
			lbl := material.Label(th, unit.Sp(11), text)
			lbl.Color = sb.theme.TextMuted
			return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				return lbl.Layout(gtx)
			})
		}),
		// 前へ
		layout.Rigid(func(gtx C) D {
			for sb.btnPrev.Clicked(gtx) {
				state.Search.PrevMatch()
				sb.jumpToCurrentMatch(state)
			}
			return sb.iconButton(gtx, th, &sb.btnPrev, "↑")
		}),
		// 次へ
		layout.Rigid(func(gtx C) D {
			for sb.btnNext.Clicked(gtx) {
				state.Search.NextMatch()
				sb.jumpToCurrentMatch(state)
			}
			return sb.iconButton(gtx, th, &sb.btnNext, "↓")
		}),
		// 閉じる
		layout.Rigid(func(gtx C) D {
			for sb.btnClose.Clicked(gtx) {
				state.Search.Active = false
				state.Search.Matches = state.Search.Matches[:0]
			}
			return sb.iconButton(gtx, th, &sb.btnClose, "✕")
		}),
	)
}

func (sb *SearchBar) layoutReplaceRow(gtx C, state *editor.EditorState, th *material.Theme) D {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		// スペーサー（展開ボタンと位置合わせ）
		layout.Rigid(func(gtx C) D {
			return D{Size: image.Pt(gtx.Dp(unit.Dp(22)), 0)}
		}),
		// 置換入力フィールド
		layout.Flexed(1, func(gtx C) D {
			return sb.layoutInputField(gtx, th, &sb.replaceEditor, "置換...")
		}),
		// 置換ボタン
		layout.Rigid(func(gtx C) D {
			for sb.btnReplace.Clicked(gtx) {
				if state.Search.ReplaceCurrent(state.ActiveDocument()) {
					doc := state.ActiveDocument()
					if doc != nil {
						doc.UpdateHighlight(state.Highlighter)
					}
					sb.executeSearch(state)
				}
			}
			return sb.iconButton(gtx, th, &sb.btnReplace, "⟳")
		}),
		// 全置換ボタン
		layout.Rigid(func(gtx C) D {
			for sb.btnReplaceAll.Clicked(gtx) {
				state.Search.ReplaceAll(state.ActiveDocument())
				doc := state.ActiveDocument()
				if doc != nil {
					doc.UpdateHighlight(state.Highlighter)
				}
				sb.executeSearch(state)
			}
			return sb.iconButton(gtx, th, &sb.btnReplaceAll, "⟳All")
		}),
	)
}

func (sb *SearchBar) layoutInputField(gtx C, th *material.Theme, ed *widget.Editor, hint string) D {
	bgCol := sb.theme.Background
	borderCol := sb.theme.Border

	height := gtx.Dp(unit.Dp(24))
	return withBg(gtx, func(gtx C, sz image.Point) {
		fillBackground(gtx, bgCol, sz)
		// ボーダー（上）
		fillBackground(gtx, borderCol, image.Pt(sz.X, 1))
		// ボーダー（下）
		defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
		fillBackground(gtx, borderCol, image.Pt(sz.X, 1))
		// ボーダー（左）
		fillBackground(gtx, borderCol, image.Pt(1, sz.Y))
		// ボーダー（右）
		defer op.Offset(image.Pt(sz.X-1, 0)).Push(gtx.Ops).Pop()
		fillBackground(gtx, borderCol, image.Pt(1, sz.Y))
	}, func(gtx C) D {
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			edStyle := material.Editor(th, ed, hint)
			edStyle.Color = sb.theme.Text
			edStyle.HintColor = sb.theme.TextDark
			edStyle.TextSize = unit.Sp(12)
			edStyle.Font = font.Font{Typeface: "Go Mono"}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start, Spacing: layout.SpaceAround}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return edStyle.Layout(gtx)
				}),
			)
		})
	})
}

func (sb *SearchBar) toggleButton(gtx C, th *material.Theme, btn *widget.Clickable, label string, active bool) D {
	return btn.Layout(gtx, func(gtx C) D {
		bgCol := sb.theme.SurfaceAlt
		txtCol := sb.theme.TextDark
		if active {
			bgCol = sb.theme.AccentBg
			txtCol = sb.theme.Accent
		}
		if btn.Hovered() {
			bgCol = sb.theme.Hover
		}
		return withRoundBg(gtx, bgCol, 4, func(gtx C) D {
			return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(10), label)
				lbl.Color = txtCol
				lbl.Font = font.Font{Typeface: "Go Mono"}
				return lbl.Layout(gtx)
			})
		})
	})
}

func (sb *SearchBar) iconButton(gtx C, th *material.Theme, btn *widget.Clickable, label string) D {
	return btn.Layout(gtx, func(gtx C) D {
		bgCol := sb.theme.SurfaceAlt
		if btn.Hovered() {
			bgCol = sb.theme.Hover
		}
		return withRoundBg(gtx, bgCol, 4, func(gtx C) D {
			return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(11), label)
				lbl.Color = sb.theme.TextMuted
				return lbl.Layout(gtx)
			})
		})
	})
}

func (sb *SearchBar) executeSearch(state *editor.EditorState) {
	doc := state.ActiveDocument()
	if doc == nil {
		state.Search.Matches = state.Search.Matches[:0]
		return
	}
	state.Search.ReplaceText = sb.replaceEditor.Text()
	state.Search.SearchInBuffer(
		doc.Buffer.Text(),
		func(n int) string { return doc.Buffer.Line(n) },
		doc.Buffer.LineCount(),
	)
}

func (sb *SearchBar) jumpToCurrentMatch(state *editor.EditorState) {
	m := state.Search.CurrentMatchInfo()
	if m == nil {
		return
	}
	doc := state.ActiveDocument()
	if doc == nil {
		return
	}
	doc.Buffer.SetCursorLineCol(m.Line, m.Col)
	state.ScrollCenterRequest = true
}

func (sb *SearchBar) handleEvents(gtx C, state *editor.EditorState, th *material.Theme) {
	// クエリ変更検知
	for {
		ev, ok := sb.queryEditor.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			state.Search.Query = sb.queryEditor.Text()
			sb.executeSearch(state)
		case widget.SubmitEvent:
			state.Search.NextMatch()
			sb.jumpToCurrentMatch(state)
		}
	}

	// 置換エディタ変更検知
	for {
		ev, ok := sb.replaceEditor.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			state.Search.ReplaceText = sb.replaceEditor.Text()
		}
	}

	// キーイベント処理
	for {
		evt, ok := gtx.Event(
			key.Filter{Focus: &sb.tag, Name: key.NameEscape},
			key.Filter{Focus: &sb.tag, Name: key.NameReturn},
		)
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case key.NameEscape:
				state.Search.Active = false
				state.Search.Matches = state.Search.Matches[:0]
			case key.NameReturn:
				state.Search.NextMatch()
				sb.jumpToCurrentMatch(state)
			}
		}
	}

	// ペースト処理
	for {
		evt, ok := gtx.Event(
			transfer.TargetFilter{Target: &sb.tag, Type: "application/text"},
		)
		if !ok {
			break
		}
		if de, ok := evt.(transfer.DataEvent); ok {
			data := de.Open()
			content, err := io.ReadAll(data)
			_ = data.Close()
			if err == nil && len(content) > 0 {
				sb.queryEditor.Insert(string(content))
			}
		}
	}

	// クリップボードイベントの登録
	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &sb.tag)

	// clipboard/transfer用の宣言（未使用変数回避）
	_ = clipboard.WriteCmd{}
}

// Focus は検索バーの入力にフォーカスを移す
func (sb *SearchBar) Focus(gtx C) {
	gtx.Execute(key.FocusCmd{Tag: &sb.queryEditor})
}

// SetQuery は検索クエリを設定する
func (sb *SearchBar) SetQuery(text string) {
	sb.queryEditor.SetText(text)
}
