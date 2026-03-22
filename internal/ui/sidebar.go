package ui

import (
	"catana/internal/editor"
	"image"
	"os/exec"
	"path/filepath"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	sidebarDefaultWidthDp = 256
	sidebarMinWidthDp     = 220
	sidebarMaxWidthDp     = 480
	sidebarResizeHandleDp = 6
)

// Sidebar はサイドバーコンテナ
type Sidebar struct {
	theme         *Theme
	fileTree      *FileTree
	gitPanel      *GitPanel
	settingsPanel *SettingsPanel
	btnNewFile    widget.Clickable
	btnNewFolder  widget.Clickable
	btnOpenFolder widget.Clickable
	invalidateFn  func()
	folderPickCh  chan string
	resizeTag     bool
	widthPx       int
	availablePx   int
	resizing      bool
	resizePID     pointer.ID
	resizeStartX  float32
	resizeStartW  int
	// 検索パネル
	searchEditor       widget.Editor
	searchBtnRegex     widget.Clickable
	searchBtnCase      widget.Clickable
	searchResultList   widget.List
	searchResultClicks []widget.Clickable
}

// NewSidebar は新しいSidebarを作成する
func NewSidebar(theme *Theme) *Sidebar {
	sb := &Sidebar{
		theme:         theme,
		fileTree:      NewFileTree(theme),
		gitPanel:      NewGitPanel(theme),
		settingsPanel: NewSettingsPanel(theme),
		folderPickCh:  make(chan string, 1),
		widthPx:       -1,
	}
	sb.searchEditor.SingleLine = true
	sb.searchResultList.Axis = layout.Vertical
	return sb
}

// SetInvalidate はウィンドウ再描画コールバックをセットする
func (sb *Sidebar) SetInvalidate(fn func()) {
	sb.invalidateFn = fn
	sb.fileTree.SetInvalidate(fn)
}

// Close はサイドバーのリソースを解放する
func (sb *Sidebar) Close() {
	if sb.fileTree != nil {
		sb.fileTree.Close()
	}
}

// openFolderAsync はmacOSフォルダ選択ダイアログを非同期で開く
func (sb *Sidebar) openFolderAsync() {
	go func() {
		out, err := exec.Command("osascript", "-e", "POSIX path of (choose folder)").Output()
		if err != nil {
			return
		}
		path := strings.TrimSpace(string(out))
		if path != "" {
			select {
			case sb.folderPickCh <- path:
			default:
			}
			if sb.invalidateFn != nil {
				sb.invalidateFn()
			}
		}
	}()
}

// Layout はサイドバーを描画する
func (sb *Sidebar) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	if !state.SidebarOpen {
		return D{}
	}

	if sb.widthPx <= 0 {
		sb.widthPx = gtx.Dp(unit.Dp(sidebarDefaultWidthDp))
	}
	sb.availablePx = gtx.Constraints.Max.X

	width := sb.clampWidth(gtx, sb.widthPx)
	sb.widthPx = width
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width

	// フォルダ選択結果の反映
	select {
	case newPath := <-sb.folderPickCh:
		state.SetWorkspace(newPath)
		sb.fileTree.loaded = false
	default:
	}

	dims := withBg(gtx, func(gtx C, sz image.Point) {
		// 背景
		fillBackground(gtx, sb.theme.Surface, sz)
		// 右ボーダー
		defer op.Offset(image.Pt(sz.X-1, 0)).Push(gtx.Ops).Pop()
		fillBackground(gtx, sb.theme.Border, image.Pt(1, sz.Y))
	}, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// アクティビティバーの最初のアイコンと揃える上余白
			layout.Rigid(func(gtx C) D {
				return D{Size: image.Pt(0, gtx.Dp(unit.Dp(20)))}
			}),
			// ヘッダー
			layout.Rigid(func(gtx C) D {
				return sb.layoutHeader(gtx, state, th)
			}),
			// パネルコンテンツ
			layout.Flexed(1, func(gtx C) D {
				return sb.layoutPanel(gtx, state, th)
			}),
		)
	})

	sb.handleResizeEvents(gtx)
	sb.addResizeHandle(gtx, dims.Size)
	return dims
}

func (sb *Sidebar) clampWidth(gtx C, width int) int {
	minWidth := gtx.Dp(unit.Dp(sidebarMinWidthDp))
	maxWidth := gtx.Dp(unit.Dp(sidebarMaxWidthDp))
	if sb.availablePx > 0 {
		limit := sb.availablePx - gtx.Dp(unit.Dp(160))
		if limit > 0 && limit < maxWidth {
			maxWidth = limit
		}
	}
	if maxWidth < minWidth {
		maxWidth = minWidth
	}
	if width < minWidth {
		return minWidth
	}
	if width > maxWidth {
		return maxWidth
	}
	return width
}

func (sb *Sidebar) handleResizeEvents(gtx C) {
	for {
		evt, ok := gtx.Event(pointer.Filter{
			Target: &sb.resizeTag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
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
			handleW := float32(gtx.Dp(unit.Dp(sidebarResizeHandleDp)))
			if e.Position.X < float32(sb.widthPx)-handleW {
				continue
			}
			sb.resizing = true
			sb.resizePID = e.PointerID
			sb.resizeStartX = e.Position.X
			sb.resizeStartW = sb.widthPx
			gtx.Execute(pointer.GrabCmd{Tag: &sb.resizeTag, ID: e.PointerID})
		case pointer.Drag:
			if !sb.resizing || e.PointerID != sb.resizePID {
				continue
			}
			delta := int(e.Position.X - sb.resizeStartX)
			if delta != 0 {
				sb.widthPx = sb.clampWidth(gtx, sb.resizeStartW+delta)
				if sb.invalidateFn != nil {
					sb.invalidateFn()
				}
			}
		case pointer.Release, pointer.Cancel:
			if e.PointerID == sb.resizePID {
				sb.resizing = false
			}
		}
	}
}

func (sb *Sidebar) addResizeHandle(gtx C, size image.Point) {
	handleW := gtx.Dp(unit.Dp(sidebarResizeHandleDp))
	if handleW <= 0 || size.X <= 0 || size.Y <= 0 {
		return
	}
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rect(0, 0, size.X, size.Y)).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &sb.resizeTag)

	defer op.Offset(image.Pt(size.X-handleW, 0)).Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rect(0, 0, handleW, size.Y)).Push(gtx.Ops).Pop()
	pointer.CursorColResize.Add(gtx.Ops)
	fillBackground(gtx, sb.theme.Separator, image.Pt(1, size.Y))
}

func (sb *Sidebar) layoutHeader(gtx C, state *editor.EditorState, th *material.Theme) D {
	height := gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height

	// ボタンクリック処理
	for sb.btnNewFile.Clicked(gtx) {
		sb.fileTree.StartNewFile(state.Workspace)
	}
	for sb.btnNewFolder.Clicked(gtx) {
		sb.fileTree.StartNewFolder(state.Workspace)
	}
	for sb.btnOpenFolder.Clicked(gtx) {
		sb.openFolderAsync()
	}

	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			// タイトル（アイコンボタンと同じ高さで垂直中央揃え）
			layout.Flexed(1, func(gtx C) D {
				var title string
				switch state.SidebarTab {
				case editor.TabFiles:
					title = strings.ToUpper(filepath.Base(state.Workspace))
				case editor.TabSearch:
					title = "SEARCH"
				case editor.TabGit:
					title = "SOURCE CONTROL"
				case editor.TabExtensions:
					title = "EXTENSIONS"
				case editor.TabSettings:
					title = "SETTINGS"
				}
				// アイコンボタンの高さ(18dp icon + 4dp*2 padding = 26dp)に合わせる
				btnH := gtx.Dp(unit.Dp(26))
				gtx.Constraints.Min.Y = btnH
				gtx.Constraints.Max.Y = btnH
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start, Spacing: layout.SpaceAround}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := material.Label(th, unit.Sp(11), title)
						lbl.Color = sb.theme.TextMuted
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
				)
			}),
			// アクションボタン（TabFilesのみ表示）
			layout.Rigid(func(gtx C) D {
				if state.SidebarTab != editor.TabFiles {
					return D{}
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return sb.headerIconBtn(gtx, &sb.btnNewFile, DrawNewFileIcon)
					}),
					layout.Rigid(func(gtx C) D {
						return sb.headerIconBtn(gtx, &sb.btnNewFolder, DrawNewFolderIcon)
					}),
					layout.Rigid(func(gtx C) D {
						return sb.headerIconBtn(gtx, &sb.btnOpenFolder, DrawOpenFolderIcon)
					}),
				)
			}),
		)
	})
}

func (sb *Sidebar) headerIconBtn(gtx C, btn *widget.Clickable, icon IconFunc) D {
	return btn.Layout(gtx, func(gtx C) D {
		var bgCol = sb.theme.Surface
		iconCol := sb.theme.TextMuted
		if btn.Hovered() {
			bgCol = sb.theme.AccentBg
			iconCol = sb.theme.Accent
		}
		return withRoundBg(gtx, bgCol, 6, func(gtx C) D {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
				return icon(gtx, gtx.Dp(unit.Dp(18)), iconCol)
			})
		})
	})
}

func (sb *Sidebar) layoutPanel(gtx C, state *editor.EditorState, th *material.Theme) D {
	switch state.SidebarTab {
	case editor.TabFiles:
		return sb.fileTree.Layout(gtx, state, th)
	case editor.TabSearch:
		return sb.layoutSearchPanel(gtx, state, th)
	case editor.TabGit:
		return sb.layoutGitPanel(gtx, state, th)
	case editor.TabExtensions:
		return sb.layoutExtensionsPanel(gtx, state, th)
	case editor.TabSettings:
		return sb.settingsPanel.Layout(gtx, state, th)
	default:
		return D{}
	}
}

// 検索パネル（Phase 2: ワークスペース検索）
func (sb *Sidebar) layoutSearchPanel(gtx C, state *editor.EditorState, th *material.Theme) D {
	// 検索入力の変更検知
	for {
		ev, ok := sb.searchEditor.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			state.Search.Query = sb.searchEditor.Text()
			state.Search.WorkspaceActive = true
			state.Search.SearchWorkspace(state.Workspace)
		case widget.SubmitEvent:
			state.Search.Query = sb.searchEditor.Text()
			state.Search.WorkspaceActive = true
			state.Search.SearchWorkspace(state.Workspace)
		}
	}

	// トグルボタン
	for sb.searchBtnRegex.Clicked(gtx) {
		state.Search.IsRegex = !state.Search.IsRegex
		if sb.searchEditor.Text() != "" {
			state.Search.SearchWorkspace(state.Workspace)
		}
	}
	for sb.searchBtnCase.Clicked(gtx) {
		state.Search.CaseSensitive = !state.Search.CaseSensitive
		if sb.searchEditor.Text() != "" {
			state.Search.SearchWorkspace(state.Workspace)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// 検索入力行
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						h := gtx.Dp(unit.Dp(26))
						return withBg(gtx, func(gtx C, sz image.Point) {
							fillBackground(gtx, sb.theme.Background, sz)
						}, func(gtx C) D {
							gtx.Constraints.Min.Y = h
							gtx.Constraints.Max.Y = h
							return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
								edStyle := material.Editor(th, &sb.searchEditor, "ワークスペース検索...")
								edStyle.Color = sb.theme.Text
								edStyle.HintColor = sb.theme.TextDark
								edStyle.TextSize = unit.Sp(12)
								return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceAround}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										return edStyle.Layout(gtx)
									}),
								)
							})
						})
					}),
					layout.Rigid(func(gtx C) D {
						return sb.searchToggle(gtx, th, &sb.searchBtnRegex, ".*", state.Search.IsRegex)
					}),
					layout.Rigid(func(gtx C) D {
						return sb.searchToggle(gtx, th, &sb.searchBtnCase, "Aa", state.Search.CaseSensitive)
					}),
				)
			})
		}),
		// 結果表示
		layout.Flexed(1, func(gtx C) D {
			return sb.layoutSearchResults(gtx, state, th)
		}),
	)
}

func (sb *Sidebar) searchToggle(gtx C, th *material.Theme, btn *widget.Clickable, label string, active bool) D {
	return btn.Layout(gtx, func(gtx C) D {
		bgCol := sb.theme.Surface
		txtCol := sb.theme.TextDark
		if active {
			bgCol = sb.theme.AccentBg
			txtCol = sb.theme.Accent
		}
		return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx C) D {
			lbl := material.Label(th, unit.Sp(10), label)
			lbl.Color = txtCol
			_ = bgCol
			return lbl.Layout(gtx)
		})
	})
}

func (sb *Sidebar) layoutSearchResults(gtx C, state *editor.EditorState, th *material.Theme) D {
	results := state.Search.WorkspaceResults
	if len(results) == 0 {
		if state.Search.Query != "" {
			return layout.Inset{Left: unit.Dp(12), Top: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(12), "該当なし")
				lbl.Color = sb.theme.TextDark
				return lbl.Layout(gtx)
			})
		}
		return D{}
	}

	// 結果一覧（ファイルごとにグルーピング）
	totalItems := 0
	for _, wr := range results {
		totalItems++ // ファイル名ヘッダー
		totalItems += len(wr.Matches)
	}

	type resultItem struct {
		isHeader bool
		filePath string
		relPath  string
		match    editor.SearchMatch
		clickIdx int // searchResultClicks内のインデックス (headerは-1)
	}
	items := make([]resultItem, 0, totalItems)
	clickCount := 0
	for _, wr := range results {
		rel := state.RelativePath(wr.FilePath)
		items = append(items, resultItem{isHeader: true, filePath: wr.FilePath, relPath: rel, clickIdx: -1})
		for _, m := range wr.Matches {
			items = append(items, resultItem{filePath: wr.FilePath, match: m, clickIdx: clickCount})
			clickCount++
		}
	}

	// Clickableスライスを必要数に拡張
	for len(sb.searchResultClicks) < clickCount {
		sb.searchResultClicks = append(sb.searchResultClicks, widget.Clickable{})
	}

	return material.List(th, &sb.searchResultList).Layout(gtx, len(items), func(gtx C, i int) D {
		item := items[i]
		if item.isHeader {
			return layout.Inset{Left: unit.Dp(12), Top: unit.Dp(6), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(11), item.relPath)
				lbl.Color = sb.theme.Text
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		}
		return sb.layoutSearchResultItem(gtx, state, th, item.filePath, item.match, item.clickIdx)
	})
}

func (sb *Sidebar) layoutSearchResultItem(gtx C, state *editor.EditorState, th *material.Theme, filePath string, m editor.SearchMatch, clickIdx int) D {
	// クリック検知
	if clickIdx >= 0 && clickIdx < len(sb.searchResultClicks) {
		for sb.searchResultClicks[clickIdx].Clicked(gtx) {
			_ = state.OpenFile(filePath)
			doc := state.ActiveDocument()
			if doc != nil {
				doc.Buffer.SetCursorLineCol(m.Line, m.Col)
			}
		}
		return sb.searchResultClicks[clickIdx].Layout(gtx, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(8), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
				lineText := m.LineText
				if len(lineText) > 80 {
					lineText = lineText[:80] + "..."
				}
				lbl := material.Label(th, unit.Sp(11), lineText)
				lbl.Color = sb.theme.TextMuted
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		})
	}

	return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(8), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		lineText := m.LineText
		if len(lineText) > 80 {
			lineText = lineText[:80] + "..."
		}
		lbl := material.Label(th, unit.Sp(11), lineText)
		lbl.Color = sb.theme.TextMuted
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

// Gitパネル（Phase 4: 実装済み）
func (sb *Sidebar) layoutGitPanel(gtx C, state *editor.EditorState, th *material.Theme) D {
	return sb.gitPanel.Layout(gtx, state, th)
}

// 拡張機能パネル（Phase 1: スタブ）
func (sb *Sidebar) layoutExtensionsPanel(gtx C, state *editor.EditorState, th *material.Theme) D {
	return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		lbl := material.Label(th, unit.Sp(12), "拡張機能は Phase 6 で実装予定")
		lbl.Color = sb.theme.TextDark
		return lbl.Layout(gtx)
	})
}
