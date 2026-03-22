package ui

import (
	"catana/internal/editor"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// IconFunc はアイコン描画関数の型
type IconFunc func(C, int, color.NRGBA) D

// ActivityBar はエディタ左端のアイコンバー
type ActivityBar struct {
	theme       *Theme
	btnFiles    widget.Clickable
	btnSearch   widget.Clickable
	btnGit      widget.Clickable
	btnExt      widget.Clickable
	btnTheme    widget.Clickable
	btnSettings widget.Clickable
}

// NewActivityBar は新しいActivityBarを作成する
func NewActivityBar(theme *Theme) *ActivityBar {
	return &ActivityBar{theme: theme}
}

// Layout はActivityBarを描画する
func (ab *ActivityBar) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	// 幅48dp固定
	width := gtx.Dp(unit.Dp(48))
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width

	// クリックイベント処理
	for ab.btnFiles.Clicked(gtx) {
		state.SetSidebarTab(editor.TabFiles)
	}
	for ab.btnSearch.Clicked(gtx) {
		state.SetSidebarTab(editor.TabSearch)
	}
	for ab.btnGit.Clicked(gtx) {
		state.SetSidebarTab(editor.TabGit)
	}
	for ab.btnExt.Clicked(gtx) {
		state.SetSidebarTab(editor.TabExtensions)
	}
	for ab.btnTheme.Clicked(gtx) {
		// テーマ切替（Phase 1ではスタブ）
	}

	return withBg(gtx, func(gtx C, sz image.Point) {
		// 背景
		fillBackground(gtx, ab.theme.Surface, sz)
		// 右ボーダー
		defer op.Offset(image.Pt(sz.X-1, 0)).Push(gtx.Ops).Pop()
		fillBackground(gtx, ab.theme.Border, image.Pt(1, sz.Y))
	}, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// 上部ボタン群
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						ab.iconButton(state, editor.TabFiles, &ab.btnFiles, DrawFilesIcon),
						ab.iconButton(state, editor.TabSearch, &ab.btnSearch, DrawSearchIcon),
						ab.iconButton(state, editor.TabGit, &ab.btnGit, DrawGitBranchIcon),
						ab.iconButton(state, editor.TabExtensions, &ab.btnExt, DrawBlocksIcon),
					)
				})
			}),
			// スペーサー
			layout.Flexed(1, func(gtx C) D {
				return D{Size: image.Pt(gtx.Constraints.Max.X, 0)}
			}),
			// 下部ボタン群
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return ab.renderIconButton(gtx, &ab.btnTheme, DrawSunIcon, ab.theme.TextMuted)
						}),
						layout.Rigid(func(gtx C) D {
							return ab.renderIconButton(gtx, &ab.btnSettings, DrawSettingsIcon, ab.theme.TextMuted)
						}),
					)
				})
			}),
		)
	})
}

func (ab *ActivityBar) iconButton(state *editor.EditorState, tab editor.SidebarTab, btn *widget.Clickable, icon IconFunc) layout.FlexChild {
	return layout.Rigid(func(gtx C) D {
		isActive := state.SidebarOpen && state.SidebarTab == tab
		col := ab.theme.TextMuted
		if isActive {
			col = ab.theme.Accent
		}
		return ab.renderIconButton(gtx, btn, icon, col)
	})
}

func (ab *ActivityBar) renderIconButton(gtx C, btn *widget.Clickable, icon IconFunc, col color.NRGBA) D {
	isActive := col == ab.theme.Accent
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			var bgColor color.NRGBA
			iconCol := col
			if isActive {
				bgColor = ab.theme.AccentBg
			} else if btn.Hovered() {
				bgColor = ab.theme.AccentBg
				iconCol = ab.theme.Accent
			}
			return withRoundBg(gtx, bgColor, 12, func(gtx C) D {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
					return icon(gtx, gtx.Dp(unit.Dp(20)), iconCol)
				})
			})
		})
	})
}
