package ui

import (
	"catana/internal/editor"
	"catana/internal/git"
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// GitPanel はサイドバー内のGit操作パネル
type GitPanel struct {
	theme *Theme

	// コミット入力
	commitEditor widget.Editor

	// ボタン
	btnCommit     widget.Clickable
	btnStageAll   widget.Clickable
	btnUnstageAll widget.Clickable
	btnRefresh    widget.Clickable
	btnStash      widget.Clickable
	btnStashPop   widget.Clickable

	// リスト
	stagedList   widget.List
	unstagedList widget.List
	logList      widget.List
	stashList    widget.List

	// ファイルごとのクリック（ステージ/アンステージ）
	stagedClicks   []gitFileClick
	unstagedClicks []gitFileClick
	stashClicks    []widget.Clickable

	// セクション折畳
	stagedExpanded   bool
	unstagedExpanded bool
	logExpanded      bool
	stashExpanded    bool

	// セクションクリック
	btnStagedHeader   widget.Clickable
	btnUnstagedHeader widget.Clickable
	btnLogHeader      widget.Clickable
	btnStashHeader    widget.Clickable

	// 全体リスト
	mainList widget.List

	// 状態
	lastError string
}

// gitFileClick はGitファイルエントリのクリック操作
type gitFileClick struct {
	btnOpen    widget.Clickable // ファイルを開く（diffタブ）
	btnStage   widget.Clickable // ステージ/アンステージ
	btnDiscard widget.Clickable // 変更破棄
}

// NewGitPanel は新しいGitPanelを作成する
func NewGitPanel(theme *Theme) *GitPanel {
	gp := &GitPanel{
		theme:            theme,
		stagedExpanded:   true,
		unstagedExpanded: true,
		logExpanded:      false,
		stashExpanded:    false,
	}
	gp.commitEditor.SingleLine = false
	gp.commitEditor.Submit = true
	gp.mainList.Axis = layout.Vertical
	gp.stagedList.Axis = layout.Vertical
	gp.unstagedList.Axis = layout.Vertical
	gp.logList.Axis = layout.Vertical
	gp.stashList.Axis = layout.Vertical
	return gp
}

// Layout はGitパネルを描画する
func (gp *GitPanel) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	// Gitリポジトリでない場合
	if state.Git == nil || !state.Git.IsGitRepo() {
		return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := material.Label(th, unit.Sp(12), "Gitリポジトリが見つかりません")
					lbl.Color = gp.theme.TextDark
					return lbl.Layout(gtx)
				}),
			)
		})
	}

	// 定期的なステータス更新
	state.RefreshGitStatusIfNeeded()

	// ボタン処理
	gp.handleButtons(gtx, state)

	// セクションヘッダーのクリック処理
	for gp.btnStagedHeader.Clicked(gtx) {
		gp.stagedExpanded = !gp.stagedExpanded
	}
	for gp.btnUnstagedHeader.Clicked(gtx) {
		gp.unstagedExpanded = !gp.unstagedExpanded
	}
	for gp.btnLogHeader.Clicked(gtx) {
		gp.logExpanded = !gp.logExpanded
		if gp.logExpanded && len(state.GitLog) == 0 {
			state.RefreshGitLog()
		}
	}
	for gp.btnStashHeader.Clicked(gtx) {
		gp.stashExpanded = !gp.stashExpanded
		if gp.stashExpanded && len(state.GitStashList) == 0 {
			state.RefreshGitStashList()
		}
	}

	// ステージ済みファイルと未ステージファイルを分類
	var staged, unstaged []git.FileEntry
	for _, entry := range state.GitStatus {
		if entry.Staged != git.StatusUnmodified {
			staged = append(staged, entry)
		}
		if entry.Status != git.StatusUnmodified {
			unstaged = append(unstaged, entry)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// コミットメッセージ入力
		layout.Rigid(func(gtx C) D {
			return gp.layoutCommitArea(gtx, state, th)
		}),
		// エラーメッセージ
		layout.Rigid(func(gtx C) D {
			if gp.lastError == "" {
				return D{}
			}
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(10), gp.lastError)
				lbl.Color = gp.theme.GitDeleted
				lbl.MaxLines = 2
				return lbl.Layout(gtx)
			})
		}),
		// メインスクロール領域
		layout.Flexed(1, func(gtx C) D {
			// セクション数を計算
			sections := 0
			// ステージ済みヘッダー + 内容
			sections++ // ヘッダー
			if gp.stagedExpanded {
				sections += len(staged)
			}
			// 未ステージヘッダー + 内容
			sections++ // ヘッダー
			if gp.unstagedExpanded {
				sections += len(unstaged)
			}
			// 履歴ヘッダー + 内容
			sections++ // ヘッダー
			if gp.logExpanded {
				sections += len(state.GitLog)
			}
			// Stashヘッダー + 内容
			sections++ // ヘッダー
			if gp.stashExpanded {
				sections += len(state.GitStashList)
			}

			type listItem struct {
				kind    string // "staged-header", "staged-file", "unstaged-header", "unstaged-file", "log-header", "log-entry", "stash-header", "stash-entry"
				fileIdx int
			}
			items := make([]listItem, 0, sections)

			// ステージ済み
			items = append(items, listItem{kind: "staged-header"})
			if gp.stagedExpanded {
				for i := range staged {
					items = append(items, listItem{kind: "staged-file", fileIdx: i})
				}
			}
			// 未ステージ
			items = append(items, listItem{kind: "unstaged-header"})
			if gp.unstagedExpanded {
				for i := range unstaged {
					items = append(items, listItem{kind: "unstaged-file", fileIdx: i})
				}
			}
			// コミット履歴
			items = append(items, listItem{kind: "log-header"})
			if gp.logExpanded {
				for i := range state.GitLog {
					items = append(items, listItem{kind: "log-entry", fileIdx: i})
				}
			}
			// Stash
			items = append(items, listItem{kind: "stash-header"})
			if gp.stashExpanded {
				for i := range state.GitStashList {
					items = append(items, listItem{kind: "stash-entry", fileIdx: i})
				}
			}

			// Clickableスライスの拡張
			for len(gp.stagedClicks) < len(staged) {
				gp.stagedClicks = append(gp.stagedClicks, gitFileClick{})
			}
			for len(gp.unstagedClicks) < len(unstaged) {
				gp.unstagedClicks = append(gp.unstagedClicks, gitFileClick{})
			}
			for len(gp.stashClicks) < len(state.GitStashList) {
				gp.stashClicks = append(gp.stashClicks, widget.Clickable{})
			}

			return material.List(th, &gp.mainList).Layout(gtx, len(items), func(gtx C, i int) D {
				item := items[i]
				switch item.kind {
				case "staged-header":
					return gp.layoutSectionHeader(gtx, th, &gp.btnStagedHeader, "STAGED CHANGES", len(staged), gp.stagedExpanded)
				case "staged-file":
					return gp.layoutFileEntry(gtx, state, th, staged[item.fileIdx], &gp.stagedClicks[item.fileIdx], true)
				case "unstaged-header":
					return gp.layoutSectionHeader(gtx, th, &gp.btnUnstagedHeader, "CHANGES", len(unstaged), gp.unstagedExpanded)
				case "unstaged-file":
					return gp.layoutFileEntry(gtx, state, th, unstaged[item.fileIdx], &gp.unstagedClicks[item.fileIdx], false)
				case "log-header":
					return gp.layoutSectionHeader(gtx, th, &gp.btnLogHeader, "COMMITS", len(state.GitLog), gp.logExpanded)
				case "log-entry":
					return gp.layoutLogEntry(gtx, th, state.GitLog[item.fileIdx])
				case "stash-header":
					return gp.layoutSectionHeader(gtx, th, &gp.btnStashHeader, "STASHES", len(state.GitStashList), gp.stashExpanded)
				case "stash-entry":
					return gp.layoutStashEntry(gtx, state, th, state.GitStashList[item.fileIdx], item.fileIdx)
				}
				return D{}
			})
		}),
	)
}

// handleButtons はボタンのクリック処理を行う
func (gp *GitPanel) handleButtons(gtx C, state *editor.EditorState) {
	gp.lastError = ""

	for gp.btnCommit.Clicked(gtx) {
		if err := state.GitCommit(); err != nil {
			gp.lastError = err.Error()
		}
		state.GitClearDiffCache()
	}
	for gp.btnStageAll.Clicked(gtx) {
		if err := state.GitStageAll(); err != nil {
			gp.lastError = err.Error()
		}
	}
	for gp.btnUnstageAll.Clicked(gtx) {
		if err := state.GitUnstageAll(); err != nil {
			gp.lastError = err.Error()
		}
	}
	for gp.btnRefresh.Clicked(gtx) {
		state.GitClearDiffCache()
		state.RefreshGitStatus()
		state.RefreshGitLog()
		state.RefreshGitStashList()
	}
	for gp.btnStash.Clicked(gtx) {
		if err := state.GitStash(""); err != nil {
			gp.lastError = err.Error()
		}
	}
	for gp.btnStashPop.Clicked(gtx) {
		if err := state.GitStashPop(); err != nil {
			gp.lastError = err.Error()
		}
	}

	// コミットメッセージの同期
	for {
		ev, ok := gp.commitEditor.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			state.GitCommitMsg = gp.commitEditor.Text()
		case widget.SubmitEvent:
			state.GitCommitMsg = gp.commitEditor.Text()
			if err := state.GitCommit(); err != nil {
				gp.lastError = err.Error()
			} else {
				gp.commitEditor.SetText("")
			}
			state.GitClearDiffCache()
		}
	}
}

// layoutCommitArea はコミットメッセージ入力エリアを描画する
func (gp *GitPanel) layoutCommitArea(gtx C, state *editor.EditorState, th *material.Theme) D {
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// コミットメッセージ入力欄
			layout.Rigid(func(gtx C) D {
				h := gtx.Dp(unit.Dp(60))
				return withBg(gtx, func(gtx C, sz image.Point) {
					fillBackground(gtx, gp.theme.Background, sz)
					// ボーダー
					drawRectBorder(gtx, sz, gp.theme.Border, 1)
				}, func(gtx C) D {
					gtx.Constraints.Min.Y = h
					gtx.Constraints.Max.Y = h
					return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						edStyle := material.Editor(th, &gp.commitEditor, "コミットメッセージ...")
						edStyle.Color = gp.theme.Text
						edStyle.HintColor = gp.theme.TextDark
						edStyle.TextSize = unit.Sp(12)
						return edStyle.Layout(gtx)
					})
				})
			}),
			// ボタン行
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx,
						// コミットボタン
						layout.Rigid(func(gtx C) D {
							return gp.actionButton(gtx, th, &gp.btnCommit, "✓ Commit", gp.theme.Accent, state.HasGitStagedChanges())
						}),
						layout.Rigid(func(gtx C) D { return D{Size: image.Pt(gtx.Dp(unit.Dp(4)), 0)} }),
						// Stashボタン
						layout.Rigid(func(gtx C) D {
							return gp.actionButton(gtx, th, &gp.btnStash, "Stash", gp.theme.TextMuted, true)
						}),
						layout.Rigid(func(gtx C) D { return D{Size: image.Pt(gtx.Dp(unit.Dp(4)), 0)} }),
						// リフレッシュボタン
						layout.Rigid(func(gtx C) D {
							return gp.actionButton(gtx, th, &gp.btnRefresh, "↻", gp.theme.TextMuted, true)
						}),
					)
				})
			}),
		)
	})
}

// actionButton はアクションボタンを描画する
func (gp *GitPanel) actionButton(gtx C, th *material.Theme, btn *widget.Clickable, label string, col color.NRGBA, enabled bool) D {
	if !enabled {
		col = gp.theme.TextDark
	}
	return btn.Layout(gtx, func(gtx C) D {
		bgCol := gp.theme.SurfaceAlt
		if btn.Hovered() && enabled {
			bgCol = gp.theme.AccentBg
		}
		return withRoundBg(gtx, bgCol, 4, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(11), label)
				lbl.Color = col
				return lbl.Layout(gtx)
			})
		})
	})
}

// layoutSectionHeader はセクションヘッダーを描画する
func (gp *GitPanel) layoutSectionHeader(gtx C, th *material.Theme, btn *widget.Clickable, title string, count int, expanded bool) D {
	return btn.Layout(gtx, func(gtx C) D {
		bgCol := gp.theme.Surface
		if btn.Hovered() {
			bgCol = gp.theme.Hover
		}
		return withFlatBg(gtx, bgCol, func(gtx C) D {
			h := gtx.Dp(unit.Dp(26))
			gtx.Constraints.Min.Y = h
			gtx.Constraints.Max.Y = h
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// 展開/折畳アイコン
					layout.Rigid(func(gtx C) D {
						arrow := "▶"
						if expanded {
							arrow = "▼"
						}
						lbl := material.Label(th, unit.Sp(8), arrow)
						lbl.Color = gp.theme.TextMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return D{Size: image.Pt(gtx.Dp(unit.Dp(6)), 0)} }),
					// タイトル
					layout.Rigid(func(gtx C) D {
						lbl := material.Label(th, unit.Sp(10), title)
						lbl.Color = gp.theme.TextMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return D{Size: image.Pt(gtx.Dp(unit.Dp(6)), 0)} }),
					// カウント
					layout.Rigid(func(gtx C) D {
						if count == 0 {
							return D{}
						}
						lbl := material.Label(th, unit.Sp(9), fmt.Sprintf("%d", count))
						lbl.Color = gp.theme.TextDark
						return lbl.Layout(gtx)
					}),
				)
			})
		})
	})
}

// layoutFileEntry はGitファイルエントリを描画する
func (gp *GitPanel) layoutFileEntry(gtx C, state *editor.EditorState, th *material.Theme, entry git.FileEntry, clicks *gitFileClick, isStaged bool) D {
	// クリック処理
	for clicks.btnOpen.Clicked(gtx) {
		state.OpenDiffTab(entry.Path, isStaged)
	}
	for clicks.btnStage.Clicked(gtx) {
		if isStaged {
			if err := state.GitUnstage(entry.Path); err != nil {
				gp.lastError = err.Error()
			}
		} else {
			if err := state.GitStage(entry.Path); err != nil {
				gp.lastError = err.Error()
			}
		}
		state.GitClearDiffCache()
	}
	for clicks.btnDiscard.Clicked(gtx) {
		if !isStaged {
			if err := state.GitDiscardChanges(entry.Path); err != nil {
				gp.lastError = err.Error()
			}
		}
	}

	// ファイルエントリ描画
	return clicks.btnOpen.Layout(gtx, func(gtx C) D {
		bgCol := gp.theme.Surface
		if clicks.btnOpen.Hovered() {
			bgCol = gp.theme.Hover
		}
		return withFlatBg(gtx, bgCol, func(gtx C) D {
			h := gtx.Dp(unit.Dp(24))
			gtx.Constraints.Min.Y = h
			gtx.Constraints.Max.Y = h
			return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// ファイル名
					layout.Flexed(1, func(gtx C) D {
						name := filepath.Base(entry.Path)
						dir := filepath.Dir(entry.Path)
						displayText := name
						if dir != "." {
							displayText = name + "  " + dir
						}
						lbl := material.Label(th, unit.Sp(11), displayText)
						lbl.Color = gp.theme.Text
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
					// ステータスバッジ
					layout.Rigid(func(gtx C) D {
						status := entry.Status
						if isStaged {
							status = entry.Staged
						}
						label := git.StatusLabel(status)
						col := gp.statusColor(status)
						return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(10), label)
							lbl.Color = col
							return lbl.Layout(gtx)
						})
					}),
					// ステージ/アンステージ ボタン
					layout.Rigid(func(gtx C) D {
						if !clicks.btnOpen.Hovered() {
							return D{}
						}
						actionLabel := "+"
						if isStaged {
							actionLabel = "−"
						}
						return clicks.btnStage.Layout(gtx, func(gtx C) D {
							return layout.Inset{Left: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
								lbl := material.Label(th, unit.Sp(12), actionLabel)
								lbl.Color = gp.theme.TextMuted
								if clicks.btnStage.Hovered() {
									lbl.Color = gp.theme.Accent
								}
								return lbl.Layout(gtx)
							})
						})
					}),
					// 変更破棄ボタン（未ステージのみ）
					layout.Rigid(func(gtx C) D {
						if isStaged || !clicks.btnOpen.Hovered() {
							return D{}
						}
						return clicks.btnDiscard.Layout(gtx, func(gtx C) D {
							return layout.Inset{Left: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
								lbl := material.Label(th, unit.Sp(10), "✕")
								lbl.Color = gp.theme.TextMuted
								if clicks.btnDiscard.Hovered() {
									lbl.Color = gp.theme.GitDeleted
								}
								return lbl.Layout(gtx)
							})
						})
					}),
				)
			})
		})
	})
}

// layoutLogEntry はコミット履歴エントリを描画する
func (gp *GitPanel) layoutLogEntry(gtx C, th *material.Theme, entry git.LogEntry) D {
	h := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceAround}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// ハッシュ
					layout.Rigid(func(gtx C) D {
						lbl := material.Label(th, unit.Sp(10), entry.Hash)
						lbl.Color = gp.theme.Accent
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D { return D{Size: image.Pt(gtx.Dp(unit.Dp(8)), 0)} }),
					// メッセージ
					layout.Flexed(1, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(10), entry.Message)
						lbl.Color = gp.theme.Text
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(func(gtx C) D {
				dateStr := ""
				if !entry.Date.IsZero() {
					dateStr = entry.Date.Format("2006-01-02 15:04")
				}
				info := entry.Author
				if dateStr != "" {
					info += "  " + dateStr
				}
				lbl := material.Label(th, unit.Sp(9), info)
				lbl.Color = gp.theme.TextDark
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	})
}

// layoutStashEntry はStashエントリを描画する
func (gp *GitPanel) layoutStashEntry(gtx C, state *editor.EditorState, th *material.Theme, entry git.StashEntry, idx int) D {
	// クリック処理
	if idx < len(gp.stashClicks) {
		for gp.stashClicks[idx].Clicked(gtx) {
			// stash popとして動作
			if err := state.GitStashPop(); err != nil {
				gp.lastError = err.Error()
			}
		}
	}

	h := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h

	return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			// stash番号
			layout.Rigid(func(gtx C) D {
				lbl := material.Label(th, unit.Sp(10), fmt.Sprintf("stash@{%d}", entry.Index))
				lbl.Color = gp.theme.Accent
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D { return D{Size: image.Pt(gtx.Dp(unit.Dp(8)), 0)} }),
			// メッセージ
			layout.Flexed(1, func(gtx C) D {
				msg := entry.Message
				if msg == "" {
					msg = "(メッセージなし)"
				}
				lbl := material.Label(th, unit.Sp(10), msg)
				lbl.Color = gp.theme.Text
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
		)
	})
}

// statusColor はFileStatusに対応する色を返す
func (gp *GitPanel) statusColor(status git.FileStatus) color.NRGBA {
	switch status {
	case git.StatusModified, git.StatusStaged:
		return gp.theme.GitModified
	case git.StatusAdded, git.StatusStagedNew, git.StatusUntracked:
		return gp.theme.GitAdded
	case git.StatusDeleted, git.StatusStagedDeleted:
		return gp.theme.GitDeleted
	case git.StatusConflicted:
		return gp.theme.GitDeleted
	default:
		return gp.theme.TextMuted
	}
}

// drawRectBorder は矩形のボーダーを描画する
func drawRectBorder(gtx C, sz image.Point, col color.NRGBA, width int) {
	if width <= 0 || sz.X <= 0 || sz.Y <= 0 {
		return
	}
	// 上辺
	func() {
		defer clip.Rect(image.Rect(0, 0, sz.X, width)).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()
	// 下辺
	func() {
		defer op.Offset(image.Pt(0, sz.Y-width)).Push(gtx.Ops).Pop()
		defer clip.Rect(image.Rect(0, 0, sz.X, width)).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()
	// 左辺
	func() {
		defer clip.Rect(image.Rect(0, 0, width, sz.Y)).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()
	// 右辺
	func() {
		defer op.Offset(image.Pt(sz.X-width, 0)).Push(gtx.Ops).Pop()
		defer clip.Rect(image.Rect(0, 0, width, sz.Y)).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()
}

// SplitStagedAndUnstaged はGitStatusをステージ済みと未ステージに分割する
func SplitStagedAndUnstaged(entries []git.FileEntry) (staged, unstaged []git.FileEntry) {
	for _, e := range entries {
		if e.Staged != git.StatusUnmodified {
			staged = append(staged, e)
		}
		if e.Status != git.StatusUnmodified {
			unstaged = append(unstaged, e)
		}
	}
	return
}

// FormatGitDiffSummary はGit変更の概要テキストを返す
func FormatGitDiffSummary(entries []git.FileEntry) string {
	staged, unstaged := SplitStagedAndUnstaged(entries)
	parts := []string{}
	if len(staged) > 0 {
		parts = append(parts, fmt.Sprintf("ステージ済み: %d", len(staged)))
	}
	if len(unstaged) > 0 {
		parts = append(parts, fmt.Sprintf("変更: %d", len(unstaged)))
	}
	if len(parts) == 0 {
		return "変更なし"
	}
	return strings.Join(parts, "  ")
}
