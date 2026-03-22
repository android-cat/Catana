package ui

import (
	"catana/internal/debug"
	"catana/internal/editor"
	"image"
	"log"
	"runtime"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// CatanaApp はメインアプリケーション
type CatanaApp struct {
	State           *editor.EditorState
	Theme           *Theme
	matTheme        *material.Theme
	activityBar     *ActivityBar
	sidebar         *Sidebar
	tabBar          *TabBar
	editorView      *EditorView
	statusBar       *StatusBar
	omniBar         *OmniBar
	searchBar       *SearchBar
	minimap         *Minimap
	terminalView    *TerminalView
	lastFrame       time.Time
	frameCount      int
	fpsTimer        time.Time
	debugServer     *debug.DebugServer
	lastDebugUpdate time.Time
	// 最新フレームのウィジェットサイズ記録
	lastWidgets []debug.WidgetInfo
	lastWinW    int
	lastWinH    int
}

// NewCatanaApp は新しいアプリケーションを作成する
func NewCatanaApp(workspace string) *CatanaApp {
	theme := DarkTheme()
	state := editor.NewEditorState(workspace)

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	ca := &CatanaApp{
		State:        state,
		Theme:        theme,
		matTheme:     th,
		activityBar:  NewActivityBar(theme),
		sidebar:      NewSidebar(theme),
		tabBar:       NewTabBar(theme),
		editorView:   NewEditorView(theme),
		statusBar:    NewStatusBar(theme),
		omniBar:      NewOmniBar(theme),
		searchBar:    NewSearchBar(theme),
		minimap:      NewMinimap(theme),
		terminalView: NewTerminalView(theme),
		lastFrame:    time.Now(),
		fpsTimer:     time.Now(),
		debugServer:  debug.NewDebugServer(),
	}

	// デバッグソケットサーバー起動
	if err := ca.debugServer.Start(); err != nil {
		log.Printf("[警告] デバッグサーバー起動失敗: %v", err)
	}

	return ca
}

// Run はアプリケーションのメインイベントループを実行する
func (a *CatanaApp) Run(w *app.Window) error {
	// ウィンドウの再描画コールバックをサイドバーに渡す
	a.sidebar.SetInvalidate(w.Invalidate)

	var ops op.Ops

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			// ウィンドウサイズ記録
			a.lastWinW = gtx.Constraints.Max.X
			a.lastWinH = gtx.Constraints.Max.Y

			// FPSカウント
			a.frameCount++
			now := time.Now()
			if now.Sub(a.fpsTimer) > time.Second {
				a.statusBar.fps = float64(a.frameCount) / now.Sub(a.fpsTimer).Seconds()
				a.frameCount = 0
				a.fpsTimer = now
				a.statusBar.UpdateMetrics()

				// LSPメトリクス更新（1秒に1回）
				if a.State.LSP != nil {
					errors, warnings := a.State.LSP.DiagnosticSummary()
					latency := a.State.LSP.LatencyMs()
					a.statusBar.UpdateLSPMetrics(errors, warnings, latency)
				}
			}
			a.lastFrame = now

			// メインレイアウト描画
			a.layout(gtx)

			// デバッグスナップショット更新（1秒に1回）
			if now.Sub(a.lastDebugUpdate) > time.Second {
				a.updateDebugSnapshot()
				a.lastDebugUpdate = now
			}

			e.Frame(gtx.Ops)
		}
	}
}

// layout はメインUIレイアウトを描画する
func (a *CatanaApp) layout(gtx C) D {
	// 全体背景
	fillBackground(gtx, a.Theme.Background, gtx.Constraints.Max)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// メインエリア
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				// アクティビティバー
				layout.Rigid(func(gtx C) D {
					return a.activityBar.Layout(gtx, a.State, a.matTheme)
				}),
				// サイドバー
				layout.Rigid(func(gtx C) D {
					return a.sidebar.Layout(gtx, a.State, a.matTheme)
				}),
				// メインエディタエリア
				layout.Flexed(1, func(gtx C) D {
					return a.layoutMainEditor(gtx)
				}),
			)
		}),
		// ステータスバー（最下部固定）
		layout.Rigid(func(gtx C) D {
			return a.statusBar.Layout(gtx, a.State, a.matTheme)
		}),
	)
}

// layoutMainEditor はメインエディタ領域を描画する（タブ + エディタ + オムニバー + ターミナル）
func (a *CatanaApp) layoutMainEditor(gtx C) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// タブバー + ブレッドクラム
		layout.Rigid(func(gtx C) D {
			return a.tabBar.Layout(gtx, a.State, a.matTheme)
		}),
		// エディタビュー + ミニマップ + オムニバーオーバーレイ（またはDiffビュー）
		layout.Flexed(1, func(gtx C) D {
			return layout.Stack{Alignment: layout.S}.Layout(gtx,
				// エディタビュー + ミニマップ or Diffビュー（Expanded で全領域を占有）
				layout.Expanded(func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return a.editorView.Layout(gtx, a.State, a.matTheme)
						}),
						layout.Rigid(func(gtx C) D {
							listFirst := a.editorView.list.Position.First
							vpLines := 0
							if a.editorView.lineHeightF > 0 {
								vpLines = int(float32(a.editorView.viewportH) / a.editorView.lineHeightF)
							}
							return a.minimap.Layout(gtx, a.State, a.matTheme, listFirst, vpLines)
						}),
					)
				}),
				// 検索バー（右上にフローティング）
				layout.Stacked(func(gtx C) D {
					if !a.State.Search.Active {
						return D{}
					}
					maxW := gtx.Constraints.Max.X
					maxH := gtx.Constraints.Max.Y
					barW := gtx.Dp(unit.Dp(460))
					if barW > maxW {
						barW = maxW
					}
					offsetX := maxW - barW - gtx.Dp(unit.Dp(8))
					if offsetX < 0 {
						offsetX = 0
					}
					offsetY := gtx.Dp(unit.Dp(4))
					defer op.Offset(image.Pt(offsetX, offsetY)).Push(gtx.Ops).Pop()
					barGtx := gtx
					barGtx.Constraints.Min.X = barW
					barGtx.Constraints.Max.X = barW
					a.searchBar.Layout(barGtx, a.State, a.matTheme)
					// layout.S アライメントの影響を無効化: 全領域サイズを返すことで
					// Stackのオフセット=(0,0)にし、op.Offsetのみで位置制御する
					return D{Size: image.Pt(maxW, maxH)}
				}),
				// オムニバー（下部中央にフローティング）
				layout.Stacked(func(gtx C) D {
					return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
						return a.omniBar.Layout(gtx, a.State, a.matTheme)
					})
				}),
			)
		}),
		// ターミナルパネル（表示時のみ）
		layout.Rigid(func(gtx C) D {
			if !a.State.ShowTerminal {
				return D{}
			}
			height := gtx.Dp(unit.Dp(250))
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height
			return a.terminalView.Layout(gtx, a.State, a.matTheme)
		}),
	)
}

// Stop はアプリケーションのクリーンアップを行う
func (a *CatanaApp) Stop() {
	if a.debugServer != nil {
		a.debugServer.Stop()
	}
	if a.sidebar != nil {
		a.sidebar.Close()
	}
	if a.State != nil {
		a.State.Cleanup()
	}
}

// updateDebugSnapshot はデバッグスナップショットを更新する
func (a *CatanaApp) updateDebugSnapshot() {
	if a.debugServer == nil {
		return
	}

	snap := &debug.AppSnapshot{
		WindowWidth:  a.lastWinW,
		WindowHeight: a.lastWinH,
		FPS:          a.statusBar.fps,
		MemoryMB:     a.statusBar.memUsage.Load(),
	}

	// エディタ状態
	snap.Editor = a.buildEditorSnapshot()

	// ウィジェットツリー（レイアウト情報）
	snap.Widgets = a.buildWidgetTree()

	a.debugServer.UpdateSnapshot(snap)
}

func (a *CatanaApp) buildEditorSnapshot() debug.EditorSnapshot {
	es := debug.EditorSnapshot{
		Workspace:    a.State.Workspace,
		SidebarOpen:  a.State.SidebarOpen,
		ShowOmniChat: a.State.ShowOmniChat,
	}

	// サイドバータブ
	switch a.State.SidebarTab {
	case editor.TabFiles:
		es.SidebarTab = "files"
	case editor.TabSearch:
		es.SidebarTab = "search"
	case editor.TabGit:
		es.SidebarTab = "git"
	case editor.TabExtensions:
		es.SidebarTab = "extensions"
	}

	// オムニモード
	switch a.State.OmniMode {
	case editor.ModeAI:
		es.OmniMode = "ai"
	case editor.ModeCmd:
		es.OmniMode = "cmd"
	case editor.ModeTerm:
		es.OmniMode = "term"
	}

	// 開いているファイル
	for _, doc := range a.State.Documents {
		es.OpenFiles = append(es.OpenFiles, doc.FileName)
	}

	// アクティブドキュメント
	doc := a.State.ActiveDocument()
	if doc != nil {
		es.ActiveFile = doc.FileName
		es.ActiveLanguage = doc.Language
		es.Modified = doc.Modified
		es.CursorLine = doc.Buffer.CursorLine()
		es.CursorCol = doc.Buffer.CursorCol()
	}

	return es
}

func (a *CatanaApp) buildWidgetTree() []debug.WidgetInfo {
	// 各ウィジェットのサイズを推定（Gio はフレーム後のDimensionsを直接返すが、
	// ここではdp値を記録する。正確なピクセル値はフレーム描画結果から取得）
	var widgets []debug.WidgetInfo

	// アクティビティバー: 固定幅48dp
	widgets = append(widgets, debug.WidgetInfo{
		Name:    "ActivityBar",
		X:       0,
		Y:       0,
		Width:   48,
		Height:  a.lastWinH,
		Visible: true,
	})

	// サイドバー: 288dp（開いている場合）
	sidebarX := 48
	sidebarW := 0
	if a.State.SidebarOpen {
		sidebarW = 288
	}
	widgets = append(widgets, debug.WidgetInfo{
		Name:    "Sidebar",
		X:       sidebarX,
		Y:       0,
		Width:   sidebarW,
		Height:  a.lastWinH,
		Visible: a.State.SidebarOpen,
	})

	// メインエディタエリア
	editorX := sidebarX + sidebarW
	editorW := a.lastWinW - editorX
	statusH := 24

	// タブバー: 高さ40dp
	tabBarH := 40
	widgets = append(widgets, debug.WidgetInfo{
		Name:    "TabBar",
		X:       editorX,
		Y:       0,
		Width:   editorW,
		Height:  tabBarH,
		Visible: true,
	})

	// エディタビュー
	editorViewH := a.lastWinH - tabBarH - statusH
	widgets = append(widgets, debug.WidgetInfo{
		Name:    "EditorView",
		X:       editorX,
		Y:       tabBarH,
		Width:   editorW,
		Height:  editorViewH,
		Visible: true,
	})

	// オムニバー（フローティング、下部中央）
	omniW := 768
	if omniW > editorW {
		omniW = editorW
	}
	omniH := 80 // 基本高さ（チャット非表示時）
	if a.State.ShowOmniChat && a.State.OmniMode == editor.ModeAI {
		omniH = 380 // チャット表示時
	}
	omniX := editorX + (editorW-omniW)/2
	omniY := a.lastWinH - statusH - 16 - omniH
	widgets = append(widgets, debug.WidgetInfo{
		Name:    "OmniBar",
		X:       omniX,
		Y:       omniY,
		Width:   omniW,
		Height:  omniH,
		Visible: true,
	})

	// ステータスバー
	widgets = append(widgets, debug.WidgetInfo{
		Name:    "StatusBar",
		X:       0,
		Y:       a.lastWinH - statusH,
		Width:   a.lastWinW,
		Height:  statusH,
		Visible: true,
	})

	return widgets
}

// memUsageの取得のためStatusBarにゲッターを追加する必要を回避
func init() {
	// runtime パッケージの利用を保証
	_ = runtime.NumCPU
}
