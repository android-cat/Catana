package ui

import (
	"catana/internal/editor"
	"image"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// inputKind はインライン入力のモード
type inputKind int

const (
	inputNone      inputKind = iota
	inputNewFile             // 新規ファイル作成
	inputNewFolder           // 新規フォルダ作成
	inputRename              // 名前変更
)

// FileEntry はファイルツリーの1エントリ
type FileEntry struct {
	Name   string
	Path   string
	IsDir  bool
	Depth  int
	Click  widget.Clickable
	Toggle widget.Clickable
}

type fileTreeRow struct {
	Path  string
	Rect  image.Rectangle
	IsDir bool
}

// FileTree はファイルツリーウィジェット
type FileTree struct {
	theme         *Theme
	entries       []FileEntry
	expanded      map[string]bool
	list          widget.List
	loaded        bool
	rootPath      string
	invalidateFn  func()
	inputMode     inputKind
	inputEditor   widget.Editor
	inputParent   string
	inputTarget   string
	inputCancel   widget.Clickable
	needFocus     bool
	rowByPath     map[string]fileTreeRow
	visibleRows   []string
	treeRect      image.Rectangle
	contextPath   string
	contextIsDir  bool
	contextRect   image.Rectangle
	contextRename widget.Clickable
	contextDelete widget.Clickable
	dragPath      string
	dragIsDir     bool
	dragActive    bool
	dragPointer   pointer.ID
	dragStartX    float32
	dragStartY    float32
	dragPosX      float32
	dragPosY      float32
	dragRect      image.Rectangle
	hoverPath     string
	hoverRoot     bool
	// Shift選択
	selected       map[string]bool
	lastClickedIdx int
	// ファイル変更監視
	watcher     *fsnotify.Watcher
	watcherDone chan struct{}
	needReload  bool // 外部変更によるリロード要求
	reloadMu    sync.Mutex
}

// NewFileTree は新しいFileTreeを作成する
func NewFileTree(theme *Theme) *FileTree {
	ft := &FileTree{
		theme:          theme,
		expanded:       make(map[string]bool),
		rowByPath:      make(map[string]fileTreeRow),
		selected:       make(map[string]bool),
		lastClickedIdx: -1,
	}
	ft.list.Axis = layout.Vertical
	ft.inputEditor.SingleLine = true
	ft.inputEditor.Submit = true
	return ft
}

// SetInvalidate は再描画コールバックをセットする
func (ft *FileTree) SetInvalidate(fn func()) {
	ft.invalidateFn = fn
}

// StartNewFile はファイル名入力モードを開始する
func (ft *FileTree) StartNewFile(parentDir string) {
	ft.clearContextMenu()
	ft.inputMode = inputNewFile
	ft.inputParent = parentDir
	ft.inputTarget = ""
	ft.inputEditor.SetText("")
	ft.needFocus = true
}

// StartNewFolder はフォルダ名入力モードを開始する
func (ft *FileTree) StartNewFolder(parentDir string) {
	ft.clearContextMenu()
	ft.inputMode = inputNewFolder
	ft.inputParent = parentDir
	ft.inputTarget = ""
	ft.inputEditor.SetText("")
	ft.needFocus = true
}

// StartRename は名前変更入力モードを開始する
func (ft *FileTree) StartRename(path string) {
	ft.clearContextMenu()
	ft.inputMode = inputRename
	ft.inputParent = filepath.Dir(path)
	ft.inputTarget = path
	ft.inputEditor.SetText(filepath.Base(path))
	ft.needFocus = true
	ft.clearDrag()
}

// Reload はファイルツリーを強制的に再読み込みする
func (ft *FileTree) Reload() {
	ft.loadEntries(ft.rootPath)
}

// Close はファイルウォッチャーを停止する
func (ft *FileTree) Close() {
	ft.stopWatcher()
}

// startWatcher はファイルシステム監視を開始する
func (ft *FileTree) startWatcher(root string) {
	ft.stopWatcher()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[FileTree] watcher作成失敗: %v", err)
		return
	}
	ft.watcher = w
	ft.watcherDone = make(chan struct{})

	// ルートおよび展開済みディレクトリを監視
	ft.addWatchDirs(root)

	go ft.watchLoop()
}

// stopWatcher はファイルシステム監視を停止する
func (ft *FileTree) stopWatcher() {
	if ft.watcher != nil {
		ft.watcher.Close()
		<-ft.watcherDone
		ft.watcher = nil
		ft.watcherDone = nil
	}
}

// addWatchDirs はディレクトリを再帰的に監視対象に追加する（展開済みのみ）
func (ft *FileTree) addWatchDirs(dir string) {
	if ft.watcher == nil {
		return
	}
	_ = ft.watcher.Add(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > 0 && name[0] == '.' {
			continue
		}
		fullPath := filepath.Join(dir, name)
		if ft.expanded[fullPath] {
			ft.addWatchDirs(fullPath)
		}
	}
}

// watchLoop はfsnotifyイベントをデバウンスしてリロードを要求する
func (ft *FileTree) watchLoop() {
	defer close(ft.watcherDone)

	var debounce *time.Timer
	for {
		select {
		case ev, ok := <-ft.watcher.Events:
			if !ok {
				return
			}
			log.Printf("[FileTree] fsnotify: op=%s name=%s", ev.Op, ev.Name)
			// デバウンス: 200ms以内の連続イベントをまとめる
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(200*time.Millisecond, func() {
				ft.reloadMu.Lock()
				ft.needReload = true
				ft.reloadMu.Unlock()
				if ft.invalidateFn != nil {
					ft.invalidateFn()
				}
			})
		case err, ok := <-ft.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[FileTree] fsnotify error: %v", err)
		}
	}
}

func (ft *FileTree) commitInput(state *editor.EditorState) {
	name := strings.TrimSpace(ft.inputEditor.Text())
	if name != "" {
		fullPath := filepath.Join(ft.inputParent, name)
		switch ft.inputMode {
		case inputNewFile:
			if err := os.WriteFile(fullPath, []byte(""), 0644); err == nil {
				ft.loadEntries(ft.rootPath)
				_ = state.OpenFile(fullPath)
			}
		case inputNewFolder:
			if err := os.MkdirAll(fullPath, 0755); err == nil {
				ft.loadEntries(ft.rootPath)
			}
		case inputRename:
			oldPath := ft.inputTarget
			if newPath, err := state.RenamePath(oldPath, name); err == nil {
				ft.remapExpandedPaths(oldPath, newPath)
				ft.loadEntries(ft.rootPath)
			}
		}
	}
	ft.clearInput()
}

// Layout はファイルツリーを描画する
func (ft *FileTree) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	// ワークスペースが変わったらリロード + ウォッチャー再起動
	if ft.rootPath != state.Workspace || !ft.loaded {
		ft.rootPath = state.Workspace
		ft.loadEntries(state.Workspace)
		ft.loaded = true
		ft.startWatcher(state.Workspace)
	}

	// 外部変更によるリロード
	ft.reloadMu.Lock()
	needReload := ft.needReload
	ft.needReload = false
	ft.reloadMu.Unlock()
	if needReload {
		ft.loadEntries(ft.rootPath)
		// ウォッチャーの監視対象を更新（新ディレクトリが追加されている可能性）
		if ft.watcher != nil {
			ft.addWatchDirs(ft.rootPath)
		}
	}

	for ft.contextRename.Clicked(gtx) {
		if ft.contextPath != "" {
			ft.StartRename(ft.contextPath)
		}
	}
	for ft.contextDelete.Clicked(gtx) {
		if ft.contextPath != "" {
			ft.deletePath(state, ft.contextPath, ft.contextIsDir)
		}
	}

	ft.handlePointerEvents(gtx, state)

	// インライン入力モードのイベント処理
	if ft.inputMode != inputNone {
		for ft.inputCancel.Clicked(gtx) {
			ft.clearInput()
		}
		for {
			ev, ok := ft.inputEditor.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				ft.commitInput(state)
			}
		}
	}

	// クリックイベント処理（entries 再構築はループ後に行う）
	needToggleReload := false
	for i := range ft.entries {
		entry := &ft.entries[i]
		if entry.IsDir {
			for entry.Toggle.Clicked(gtx) {
				ft.expanded[entry.Path] = !ft.expanded[entry.Path]
				needToggleReload = true
			}
		} else {
			// ファイルクリックはhandlePointerEventsで処理（Shift検出のため）
			for entry.Click.Clicked(gtx) {
			}
		}
	}
	if needToggleReload {
		ft.loadEntries(ft.rootPath)
		// 展開されたディレクトリを監視対象に追加
		if ft.watcher != nil {
			ft.addWatchDirs(ft.rootPath)
		}
	}

	// インライン入力行がある場合は先頭に1行追加
	count := len(ft.entries)
	offset := 0
	showCreateInput := ft.inputMode == inputNewFile || ft.inputMode == inputNewFolder
	if showCreateInput {
		count++
		offset = 1
	}
	ft.rowByPath = make(map[string]fileTreeRow, len(ft.entries))
	ft.visibleRows = ft.visibleRows[:0]
	listY := 0
	haveRowY := false

	dims := withBg(gtx, func(gtx C, sz image.Point) {
		if ft.hoverRoot {
			fillRoundRect(gtx, ft.theme.AccentBg, sz, 6)
		}
	}, func(gtx C) D {
		return material.List(th, &ft.list).Layout(gtx, count, func(gtx C, i int) D {
			if !haveRowY {
				listY = -ft.list.Position.Offset
				haveRowY = true
			}
			if i == 0 && showCreateInput {
				dims := ft.layoutInputRow(gtx, th)
				listY += dims.Size.Y
				return dims
			}
			entry := &ft.entries[i-offset]
			var dims D
			if ft.inputMode == inputRename && entry.Path == ft.inputTarget {
				dims = ft.layoutRenameRow(gtx, th, entry)
			} else {
				dims = ft.layoutEntry(gtx, th, entry, state)
			}
			ft.rowByPath[entry.Path] = fileTreeRow{
				Path:  entry.Path,
				Rect:  image.Rect(0, listY, dims.Size.X, listY+dims.Size.Y),
				IsDir: entry.IsDir,
			}
			ft.visibleRows = append(ft.visibleRows, entry.Path)
			listY += dims.Size.Y
			return dims
		})
	})

	ft.treeRect = image.Rect(0, 0, dims.Size.X, dims.Size.Y)
	if ft.dragActive {
		if ft.autoScrollDrag(gtx, dims.Size) && ft.invalidateFn != nil {
			ft.invalidateFn()
		}
		ft.layoutDragPreview(gtx, th, dims.Size)
	}
	if ft.contextPath != "" {
		ft.layoutContextMenu(gtx, th, dims.Size)
	}
	return dims
}

func (ft *FileTree) layoutInputRow(gtx C, th *material.Theme) D {
	var iconFn IconFunc = DrawFileIcon
	if ft.inputMode == inputNewFolder {
		iconFn = DrawFolderIcon
	}
	return ft.layoutEditorRow(gtx, th, unit.Dp(8), iconFn, ft.theme.TextMuted, "名前を入力...")
}

func (ft *FileTree) layoutRenameRow(gtx C, th *material.Theme, entry *FileEntry) D {
	iconFn := DrawFileIcon
	iconColor := ft.theme.FileIconColor(entry.Name)
	if entry.IsDir {
		iconFn = DrawFolderIcon
		iconColor = ft.theme.TextMuted
	}
	indent := unit.Dp(8 + float32(entry.Depth)*16)
	return ft.layoutEditorRow(gtx, th, indent, iconFn, iconColor, "新しい名前")
}

func (ft *FileTree) layoutEditorRow(gtx C, th *material.Theme, indent unit.Dp, iconFn IconFunc, iconColor color.NRGBA, placeholder string) D {
	// 初回レンダリング時にフォーカスをセット
	if ft.needFocus {
		gtx.Execute(key.FocusCmd{Tag: &ft.inputEditor})
		ft.needFocus = false
	}

	return layout.Inset{
		Left: indent, Right: unit.Dp(4),
		Top: unit.Dp(2), Bottom: unit.Dp(2),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			// シェブロン位置のスペーサー
			layout.Rigid(func(gtx C) D {
				return D{Size: image.Pt(gtx.Dp(unit.Dp(14)), 0)}
			}),
			// アイコン
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					return iconFn(gtx, gtx.Dp(unit.Dp(14)), iconColor)
				})
			}),
			// テキスト入力フィールド
			layout.Flexed(1, func(gtx C) D {
				ed := material.Editor(th, &ft.inputEditor, placeholder)
				ed.Color = ft.theme.Text
				ed.HintColor = ft.theme.TextMuted
				ed.TextSize = unit.Sp(13)
				return withRoundBg(gtx, ft.theme.BorderLight, 3, func(gtx C) D {
					return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
						return ed.Layout(gtx)
					})
				})
			}),
			// キャンセルボタン
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
					return ft.inputCancel.Layout(gtx, func(gtx C) D {
						return DrawCloseIcon(gtx, gtx.Dp(unit.Dp(14)), ft.theme.TextMuted)
					})
				})
			}),
		)
	})
}

func (ft *FileTree) layoutEntry(gtx C, th *material.Theme, entry *FileEntry, state *editor.EditorState) D {
	indent := unit.Dp(8 + float32(entry.Depth)*16)

	// アクティブなファイルをハイライト
	isActive := false
	doc := state.ActiveDocument()
	if doc != nil && doc.FilePath == entry.Path {
		isActive = true
	}
	isSelected := ft.selected[entry.Path]

	btn := &entry.Click
	if entry.IsDir {
		btn = &entry.Toggle
	}
	isDragged := ft.dragActive && entry.Path == ft.dragPath
	isDropTarget := ft.hoverPath == entry.Path
	isContextTarget := ft.contextPath == entry.Path

	dims := btn.Layout(gtx, func(gtx C) D {
		return withBg(gtx, func(gtx C, sz image.Point) {
			// ホバー/アクティブ背景
			if isDropTarget {
				fillRoundRect(gtx, ft.theme.AccentBg, sz, 4)
			} else if isContextTarget {
				fillRoundRect(gtx, ft.theme.SurfaceAlt, sz, 4)
			} else if isActive {
				fillRoundRect(gtx, ft.theme.AccentBg, sz, 4)
			} else if isSelected {
				fillRoundRect(gtx, ft.theme.AccentBg, sz, 4)
			} else if isDragged {
				fillRoundRect(gtx, nrgba(0x34, 0xD3, 0x99, 18), sz, 4)
			} else if btn.Hovered() {
				fillRoundRect(gtx, ft.theme.SurfaceAlt, sz, 4)
			}
		}, func(gtx C) D {
			return layout.Inset{
				Left: indent, Right: unit.Dp(8),
				Top: unit.Dp(3), Bottom: unit.Dp(3),
			}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// シェブロン / スペーサー
					layout.Rigid(func(gtx C) D {
						iconSize := gtx.Dp(unit.Dp(14))
						if entry.IsDir {
							if ft.expanded[entry.Path] {
								return DrawChevronDown(gtx, iconSize, ft.theme.TextMuted)
							}
							return DrawChevronRight(gtx, iconSize, ft.theme.TextMuted)
						}
						return D{Size: image.Pt(iconSize, iconSize)}
					}),
					// ファイル/フォルダ アイコン
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
							iconSize := gtx.Dp(unit.Dp(14))
							if entry.IsDir {
								return DrawFolderIcon(gtx, iconSize, ft.theme.TextMuted)
							}
							col := ft.theme.FileIconColor(entry.Name)
							return DrawFileIcon(gtx, iconSize, col)
						})
					}),
					// ファイル名
					layout.Flexed(1, func(gtx C) D {
						col := ft.theme.TextMuted
						if isActive {
							col = ft.theme.Accent
						}
						if entry.IsDir {
							col = ft.theme.Text
						}
						return layout.Inset{Top: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(13), entry.Name)
							lbl.Color = col
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	})
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	defer clip.Rect(image.Rect(0, 0, dims.Size.X, dims.Size.Y)).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, entry)
	return dims
}

func (ft *FileTree) loadEntries(root string) {
	ft.entries = nil
	ft.buildEntries(root, 0)
}

func (ft *FileTree) buildEntries(dir string, depth int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// ディレクトリとファイルを分離してソート
	var dirs, files []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		// 隠しファイル・ディレクトリをスキップ
		if len(name) > 0 && name[0] == '.' {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	// まずディレクトリ、次にファイル
	for _, e := range dirs {
		fullPath := filepath.Join(dir, e.Name())
		ft.entries = append(ft.entries, FileEntry{
			Name:  e.Name(),
			Path:  fullPath,
			IsDir: true,
			Depth: depth,
		})
		if ft.expanded[fullPath] {
			ft.buildEntries(fullPath, depth+1)
		}
	}
	for _, e := range files {
		fullPath := filepath.Join(dir, e.Name())
		ft.entries = append(ft.entries, FileEntry{
			Name:  e.Name(),
			Path:  fullPath,
			IsDir: false,
			Depth: depth,
		})
	}
}

func (ft *FileTree) handlePointerEvents(gtx C, state *editor.EditorState) {
	for i := range ft.entries {
		entry := &ft.entries[i]
		for {
			evt, ok := gtx.Event(pointer.Filter{
				Target: entry,
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
				if e.Buttons == pointer.ButtonSecondary {
					ft.openContextMenu(entry)
					continue
				}
				if e.Buttons != pointer.ButtonPrimary {
					continue
				}
				ft.clearContextMenu()
				ft.dragPath = entry.Path
				ft.dragIsDir = entry.IsDir
				ft.dragActive = false
				ft.dragPointer = e.PointerID
				ft.dragStartX = e.Position.X
				ft.dragStartY = e.Position.Y
				ft.dragPosX = e.Position.X
				ft.dragPosY = e.Position.Y
				if row, ok := ft.rowByPath[entry.Path]; ok {
					ft.dragRect = row.Rect
				}
				ft.hoverPath = ""
				ft.hoverRoot = false
			case pointer.Drag:
				if e.PointerID != ft.dragPointer || entry.Path != ft.dragPath {
					continue
				}
				if !ft.dragActive {
					dx := e.Position.X - ft.dragStartX
					if dx < 0 {
						dx = -dx
					}
					dy := e.Position.Y - ft.dragStartY
					if dy < 0 {
						dy = -dy
					}
					if dx < 4 && dy < 4 {
						continue
					}
					ft.dragActive = true
					ft.clearContextMenu()
					if row, ok := ft.rowByPath[entry.Path]; ok {
						ft.dragRect = row.Rect
					}
					gtx.Execute(pointer.GrabCmd{Tag: entry, ID: e.PointerID})
				}
				ft.updateDragPosition(e.Position)
				ft.updateDropTarget()
			case pointer.Release:
				if e.PointerID != ft.dragPointer || entry.Path != ft.dragPath {
					continue
				}
				if ft.dragActive {
					ft.updateDragPosition(e.Position)
					ft.updateDropTarget()
					ft.commitDrop(state)
				} else if !entry.IsDir {
					// ファイルクリック: Shift選択対応
					ft.handleFileClick(i, e.Modifiers, state)
				}
				ft.clearDrag()
			case pointer.Cancel:
				if e.PointerID == ft.dragPointer && entry.Path == ft.dragPath {
					ft.clearDrag()
				}
			}
		}
	}
}

// handleFileClick はファイルクリック時の処理（Shift範囲選択対応）
func (ft *FileTree) handleFileClick(idx int, mods key.Modifiers, state *editor.EditorState) {
	if mods.Contain(key.ModShift) && ft.lastClickedIdx >= 0 && ft.lastClickedIdx < len(ft.entries) {
		// Shift+クリック: 範囲選択
		start, end := ft.lastClickedIdx, idx
		if start > end {
			start, end = end, start
		}
		ft.selected = make(map[string]bool)
		for j := start; j <= end; j++ {
			e := &ft.entries[j]
			if !e.IsDir {
				ft.selected[e.Path] = true
			}
		}
		// 選択範囲のファイルをすべて開く
		for j := start; j <= end; j++ {
			e := &ft.entries[j]
			if !e.IsDir {
				_ = state.OpenFile(e.Path)
			}
		}
	} else {
		// 通常クリック: 選択をリセットして1ファイルだけ開く
		ft.selected = make(map[string]bool)
		ft.selected[ft.entries[idx].Path] = true
		ft.lastClickedIdx = idx
		_ = state.OpenFile(ft.entries[idx].Path)
	}
}

func (ft *FileTree) openContextMenu(entry *FileEntry) {
	ft.clearDrag()
	ft.contextPath = entry.Path
	ft.contextIsDir = entry.IsDir
	if row, ok := ft.rowByPath[entry.Path]; ok {
		ft.contextRect = row.Rect
	}
	if ft.invalidateFn != nil {
		ft.invalidateFn()
	}
}

func (ft *FileTree) updateDragPosition(pos f32.Point) {
	ft.dragPosX = float32(ft.dragRect.Min.X) + pos.X
	ft.dragPosY = float32(ft.dragRect.Min.Y) + pos.Y
}

func (ft *FileTree) updateDropTarget() {
	ft.hoverPath = ""
	ft.hoverRoot = false
	if !ft.dragActive {
		return
	}
	point := image.Pt(int(ft.dragPosX), int(ft.dragPosY))
	for _, path := range ft.visibleRows {
		row, ok := ft.rowByPath[path]
		if !ok || !point.In(row.Rect) {
			continue
		}
		if ft.canDropToRow(row) {
			ft.hoverPath = path
		}
		return
	}
	if point.In(ft.treeRect) && ft.canDropToDir(ft.rootPath) {
		ft.hoverRoot = true
	}
}

func (ft *FileTree) canDropToRow(row fileTreeRow) bool {
	dstDir := row.Path
	if !row.IsDir {
		dstDir = filepath.Dir(row.Path)
	}
	return ft.canDropToDir(dstDir)
}

func (ft *FileTree) canDropToDir(dstDir string) bool {
	if ft.dragPath == "" {
		return false
	}
	srcPath := filepath.Clean(ft.dragPath)
	dstDir = filepath.Clean(dstDir)
	if ft.dragIsDir && pathInsideBase(srcPath, dstDir) {
		return false
	}
	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	return filepath.Clean(dstPath) != srcPath
}

func (ft *FileTree) commitDrop(state *editor.EditorState) {
	dstDir, ok := ft.dropDir()
	if !ok {
		return
	}
	oldPath := ft.dragPath
	newPath, err := state.MovePath(oldPath, dstDir)
	if err != nil {
		return
	}
	ft.remapExpandedPaths(oldPath, newPath)
	ft.loadEntries(ft.rootPath)
}

func (ft *FileTree) deletePath(state *editor.EditorState, targetPath string, isDir bool) {
	if err := state.DeletePath(targetPath); err != nil {
		return
	}
	ft.removeExpandedPaths(targetPath)
	if ft.inputMode == inputRename && (ft.inputTarget == targetPath || pathInsideBase(targetPath, ft.inputTarget)) {
		ft.clearInput()
	}
	ft.clearContextMenu()
	if isDir && pathInsideBase(targetPath, ft.dragPath) {
		ft.clearDrag()
	}
	if ft.dragPath == targetPath {
		ft.clearDrag()
	}
	ft.loadEntries(ft.rootPath)
}

func (ft *FileTree) dropDir() (string, bool) {
	if ft.hoverRoot {
		return ft.rootPath, true
	}
	if ft.hoverPath == "" {
		return "", false
	}
	row, ok := ft.rowByPath[ft.hoverPath]
	if !ok {
		return "", false
	}
	if row.IsDir {
		return row.Path, true
	}
	return filepath.Dir(row.Path), true
}

func (ft *FileTree) remapExpandedPaths(oldBase string, newBase string) {
	if len(ft.expanded) == 0 {
		return
	}
	updated := make(map[string]bool, len(ft.expanded))
	for path, isExpanded := range ft.expanded {
		newPath := path
		if filepath.Clean(path) == filepath.Clean(oldBase) || pathInsideBase(oldBase, path) {
			rel, err := filepath.Rel(oldBase, path)
			if err == nil {
				if rel == "." {
					newPath = newBase
				} else {
					newPath = filepath.Join(newBase, rel)
				}
			}
		}
		updated[newPath] = isExpanded
	}
	ft.expanded = updated
}

func (ft *FileTree) removeExpandedPaths(base string) {
	if len(ft.expanded) == 0 {
		return
	}
	updated := make(map[string]bool, len(ft.expanded))
	for path, isExpanded := range ft.expanded {
		if filepath.Clean(path) == filepath.Clean(base) || pathInsideBase(base, path) {
			continue
		}
		updated[path] = isExpanded
	}
	ft.expanded = updated
}

func (ft *FileTree) autoScrollDrag(gtx C, size image.Point) bool {
	if !ft.dragActive || size.Y <= 0 {
		return false
	}
	margin := gtx.Dp(unit.Dp(28))
	if margin <= 0 {
		return false
	}
	y := int(ft.dragPosY)
	delta := 0
	if y < margin {
		delta = -maxInt(2, (margin-y)/3)
	} else if y > size.Y-margin {
		delta = maxInt(2, (y-(size.Y-margin))/3)
	}
	if delta == 0 {
		return false
	}
	ft.list.Position.Offset += delta
	ft.updateDropTarget()
	return true
}

func (ft *FileTree) layoutDragPreview(gtx C, th *material.Theme, size image.Point) {
	if ft.dragPath == "" || size.X <= 0 || size.Y <= 0 {
		return
	}
	label := filepath.Base(ft.dragPath)
	target := "ワークスペース直下へ移動"
	if ft.hoverPath != "" {
		if row, ok := ft.rowByPath[ft.hoverPath]; ok {
			if row.IsDir {
				target = filepath.Base(row.Path) + " に移動"
			} else {
				target = filepath.Base(filepath.Dir(row.Path)) + " に移動"
			}
		}
	}
	x := clampInt(int(ft.dragPosX)+12, 8, maxInt(8, size.X-200))
	y := clampInt(int(ft.dragPosY)+12, 8, maxInt(8, size.Y-64))
	previewMaxW := minInt(220, maxInt(140, size.X-x-8))
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	previewGtx := gtx
	previewGtx.Constraints.Min = image.Point{}
	previewGtx.Constraints.Max = image.Pt(previewMaxW, maxInt(48, size.Y-y-8))
	withRoundBg(previewGtx, ft.theme.Surface, 8, func(gtx C) D {
		return withBg(gtx, func(gtx C, sz image.Point) {
			fillRoundRect(gtx, ft.theme.BorderLight, sz, 8)
			fillRoundRect(gtx, nrgba(0x08, 0x0F, 0x0C, 235), image.Pt(maxInt(0, sz.X-2), maxInt(0, sz.Y-2)), 7)
		}, func(gtx C) D {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						lbl := material.Label(th, unit.Sp(12), label)
						lbl.Color = ft.theme.Text
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(10), target)
							lbl.Color = ft.theme.Accent
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (ft *FileTree) layoutContextMenu(gtx C, th *material.Theme, size image.Point) {
	if ft.contextPath == "" || size.X <= 0 || size.Y <= 0 {
		return
	}
	width := minInt(156, maxInt(132, size.X-16))
	x := clampInt(ft.contextRect.Max.X-width-6, 4, maxInt(4, size.X-width-4))
	y := clampInt(ft.contextRect.Max.Y+4, 4, maxInt(4, size.Y-72))
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	menuGtx := gtx
	menuGtx.Constraints.Min = image.Point{}
	menuGtx.Constraints.Max = image.Pt(width, maxInt(72, size.Y-y-4))
	withRoundBg(menuGtx, ft.theme.Surface, 8, func(gtx C) D {
		return withBg(gtx, func(gtx C, sz image.Point) {
			fillRoundRect(gtx, ft.theme.BorderLight, sz, 8)
			fillRoundRect(gtx, nrgba(0x0B, 0x0F, 0x0D, 245), image.Pt(maxInt(0, sz.X-2), maxInt(0, sz.Y-2)), 7)
		}, func(gtx C) D {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return ft.layoutContextAction(gtx, th, &ft.contextRename, DrawEditIcon, ft.theme.Text, "名前を変更")
					}),
					layout.Rigid(func(gtx C) D {
						return ft.layoutContextAction(gtx, th, &ft.contextDelete, DrawTrashIcon, ft.theme.GitDeleted, "削除")
					}),
				)
			})
		})
	})
}

func (ft *FileTree) layoutContextAction(gtx C, th *material.Theme, btn *widget.Clickable, iconFn IconFunc, col color.NRGBA, label string) D {
	return btn.Layout(gtx, func(gtx C) D {
		bg := ft.theme.Surface
		if btn.Hovered() {
			bg = ft.theme.SurfaceAlt
		}
		return withRoundBg(gtx, bg, 6, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return iconFn(gtx, gtx.Dp(unit.Dp(14)), col)
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(12), label)
							lbl.Color = col
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (ft *FileTree) clearInput() {
	ft.inputMode = inputNone
	ft.inputParent = ""
	ft.inputTarget = ""
	ft.needFocus = false
	ft.inputEditor.SetText("")
}

func (ft *FileTree) clearContextMenu() {
	ft.contextPath = ""
	ft.contextIsDir = false
	ft.contextRect = image.Rectangle{}
}

func (ft *FileTree) clearDrag() {
	ft.dragPath = ""
	ft.dragIsDir = false
	ft.dragActive = false
	ft.hoverPath = ""
	ft.hoverRoot = false
	ft.dragRect = image.Rectangle{}
	ft.dragStartX = 0
	ft.dragStartY = 0
	ft.dragPosX = 0
	ft.dragPosY = 0
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func pathInsideBase(base string, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
