package ui

import (
	"catana/internal/editor"
	"catana/internal/lsp"
	"context"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// CompletionPopup はLSP補完候補のポップアップ
type CompletionPopup struct {
	theme          *Theme
	items          []lsp.CompletionItem
	selectedIdx    int
	visible        bool
	list           widget.List
	lastTriggerPos int       // 補完トリガー位置
	lastRequest    time.Time // 最後のリクエスト時刻
}

// NewCompletionPopup は新しい補完ポップアップを作成する
func NewCompletionPopup(theme *Theme) *CompletionPopup {
	cp := &CompletionPopup{
		theme: theme,
	}
	cp.list.Axis = layout.Vertical
	return cp
}

// Show は補完候補を表示する
func (cp *CompletionPopup) Show(items []lsp.CompletionItem) {
	cp.items = items
	cp.selectedIdx = 0
	cp.visible = len(items) > 0
}

// Hide は補完ポップアップを非表示にする
func (cp *CompletionPopup) Hide() {
	cp.visible = false
	cp.items = nil
	cp.selectedIdx = 0
}

// IsVisible は表示中かを返す
func (cp *CompletionPopup) IsVisible() bool {
	return cp.visible && len(cp.items) > 0
}

// SelectedItem は選択中の補完候補を返す
func (cp *CompletionPopup) SelectedItem() *lsp.CompletionItem {
	if cp.selectedIdx >= 0 && cp.selectedIdx < len(cp.items) {
		return &cp.items[cp.selectedIdx]
	}
	return nil
}

// HandleKey はキーイベントを処理する。trueを返した場合はイベントが消費された
func (cp *CompletionPopup) HandleKey(e key.Event) bool {
	if !cp.IsVisible() {
		return false
	}

	switch e.Name {
	case key.NameUpArrow:
		if cp.selectedIdx > 0 {
			cp.selectedIdx--
		}
		return true
	case key.NameDownArrow:
		if cp.selectedIdx < len(cp.items)-1 {
			cp.selectedIdx++
		}
		return true
	case key.NameReturn, key.NameTab:
		// 補完確定は呼び出し元で処理
		return true
	case key.NameEscape:
		cp.Hide()
		return true
	}
	return false
}

// RequestCompletion はLSPから補完候補を取得する（非同期）
func (cp *CompletionPopup) RequestCompletion(state *editor.EditorState) {
	doc := state.ActiveDocument()
	if doc == nil || doc.FilePath == "" || doc.Language == "" {
		return
	}
	if state.LSP == nil {
		return
	}

	client := state.LSP.ClientForLanguage(doc.Language)
	if client == nil || !client.IsReady() {
		return
	}

	// レート制限: 100ms
	now := time.Now()
	if now.Sub(cp.lastRequest) < 100*time.Millisecond {
		return
	}
	cp.lastRequest = now

	uri := lsp.FilePathToURI(doc.FilePath)
	line := doc.Buffer.CursorLine()
	col := doc.Buffer.CursorCol()

	// 非同期で補完取得
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		items, err := client.Completion(ctx, uri, line, col)
		if err != nil {
			return
		}

		// フィルタリング: カーソル位置の単語でフィルタ
		prefix := currentWordPrefix(doc)
		if prefix != "" {
			filtered := make([]lsp.CompletionItem, 0, len(items))
			for _, item := range items {
				label := item.Label
				filterText := item.FilterText
				if filterText == "" {
					filterText = label
				}
				if strings.HasPrefix(strings.ToLower(filterText), strings.ToLower(prefix)) {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}

		cp.Show(items)
	}()
}

// currentWordPrefix はカーソル位置の単語の先頭からカーソルまでの文字列を返す
func currentWordPrefix(doc *editor.Document) string {
	line := doc.Buffer.Line(doc.Buffer.CursorLine())
	col := doc.Buffer.CursorCol()
	if col > len(line) {
		col = len(line)
	}

	start := col
	for start > 0 {
		ch := line[start-1]
		if !isIdentChar(ch) {
			break
		}
		start--
	}

	if start >= col {
		return ""
	}
	return line[start:col]
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// Layout は補完ポップアップを描画する
func (cp *CompletionPopup) Layout(gtx C, th *material.Theme, cursorX, cursorY int) D {
	if !cp.IsVisible() {
		return D{}
	}

	maxItems := 10
	if len(cp.items) < maxItems {
		maxItems = len(cp.items)
	}

	itemH := gtx.Dp(unit.Dp(24))
	popupH := itemH * maxItems
	popupW := gtx.Dp(unit.Dp(350))

	// スクロール追従
	if cp.selectedIdx >= 0 {
		if cp.list.Position.First > cp.selectedIdx {
			cp.list.Position.First = cp.selectedIdx
		}
		if cp.selectedIdx >= cp.list.Position.First+maxItems {
			cp.list.Position.First = cp.selectedIdx - maxItems + 1
		}
	}

	// ポップアップ位置
	offsetX := cursorX
	offsetY := cursorY + gtx.Dp(unit.Dp(20))

	// 画面右端チェック
	if offsetX+popupW > gtx.Constraints.Max.X {
		offsetX = gtx.Constraints.Max.X - popupW
	}
	if offsetX < 0 {
		offsetX = 0
	}
	// 画面下端チェック
	if offsetY+popupH > gtx.Constraints.Max.Y {
		offsetY = cursorY - popupH
	}

	defer op.Offset(image.Pt(offsetX, offsetY)).Push(gtx.Ops).Pop()

	popupGtx := gtx
	popupGtx.Constraints.Min = image.Pt(popupW, 0)
	popupGtx.Constraints.Max = image.Pt(popupW, popupH)

	// 背景 + ボーダー
	return withBg(popupGtx, func(gtx C, sz image.Point) {
		// ボーダー
		borderRect := clip.RRect{
			Rect: image.Rectangle{Max: image.Pt(popupW, sz.Y)},
			NE:   4, NW: 4, SE: 4, SW: 4,
		}
		borderStack := borderRect.Push(gtx.Ops)
		paint.ColorOp{Color: cp.theme.BorderLight}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		borderStack.Pop()
		// 内側背景
		innerRect := clip.RRect{
			Rect: image.Rectangle{Min: image.Pt(1, 1), Max: image.Pt(popupW-1, sz.Y-1)},
			NE:   3, NW: 3, SE: 3, SW: 3,
		}
		innerStack := innerRect.Push(gtx.Ops)
		paint.ColorOp{Color: cp.theme.PopupBg}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		innerStack.Pop()
	}, func(gtx C) D {
		return material.List(th, &cp.list).Layout(gtx, len(cp.items), func(gtx C, i int) D {
			return cp.layoutItem(gtx, th, i)
		})
	})
}

func (cp *CompletionPopup) layoutItem(gtx C, th *material.Theme, idx int) D {
	item := cp.items[idx]
	isSelected := idx == cp.selectedIdx

	h := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	bgColor := color.NRGBA{A: 0}
	if isSelected {
		bgColor = cp.theme.AccentBg
	}

	return withFlatBg(gtx, bgColor, func(gtx C) D {
		return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// 種別アイコン
				layout.Rigid(func(gtx C) D {
					icon := completionKindIcon(item.Kind)
					iconColor := cp.completionKindColor(item.Kind)
					lbl := material.Label(th, unit.Sp(10), icon)
					lbl.Color = iconColor
					return lbl.Layout(gtx)
				}),
				// ラベル
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(12), item.Label)
						lbl.Color = cp.theme.Text
						if isSelected {
							lbl.Color = cp.theme.Accent
						}
						return lbl.Layout(gtx)
					})
				}),
				// スペーサー
				layout.Flexed(1, func(gtx C) D {
					return D{Size: image.Pt(0, h)}
				}),
				// 詳細
				layout.Rigid(func(gtx C) D {
					if item.Detail == "" {
						return D{}
					}
					detail := item.Detail
					if len(detail) > 30 {
						detail = detail[:30] + "…"
					}
					lbl := material.Label(th, unit.Sp(10), detail)
					lbl.Color = cp.theme.TextMuted
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func completionKindIcon(kind int) string {
	switch kind {
	case lsp.CompletionKindMethod:
		return "ƒ"
	case lsp.CompletionKindFunction:
		return "ƒ"
	case lsp.CompletionKindField:
		return "□"
	case lsp.CompletionKindVariable:
		return "𝑥"
	case lsp.CompletionKindClass, lsp.CompletionKindStruct:
		return "◇"
	case lsp.CompletionKindInterface:
		return "◈"
	case lsp.CompletionKindModule:
		return "▣"
	case lsp.CompletionKindProperty:
		return "□"
	case lsp.CompletionKindKeyword:
		return "⚷"
	case lsp.CompletionKindSnippet:
		return "✂"
	case lsp.CompletionKindConstant:
		return "π"
	default:
		return "·"
	}
}

func (cp *CompletionPopup) completionKindColor(kind int) color.NRGBA {
	switch kind {
	case lsp.CompletionKindMethod, lsp.CompletionKindFunction:
		return cp.theme.SynFunction
	case lsp.CompletionKindField, lsp.CompletionKindProperty:
		return cp.theme.SynFunction
	case lsp.CompletionKindVariable:
		return cp.theme.Text
	case lsp.CompletionKindClass, lsp.CompletionKindStruct, lsp.CompletionKindInterface:
		return cp.theme.SynType
	case lsp.CompletionKindKeyword:
		return cp.theme.SynKeyword
	case lsp.CompletionKindSnippet:
		return cp.theme.SynBuiltin
	case lsp.CompletionKindConstant:
		return cp.theme.SynNumber
	default:
		return cp.theme.SynOperator
	}
}
