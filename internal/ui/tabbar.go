package ui

import (
	"catana/internal/editor"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TabBar はタブバーとブレッドクラムを描画する
type TabBar struct {
	theme       *Theme
	tabClicks   []widget.Clickable
	closeClicks []widget.Clickable
	btnSidebar  widget.Clickable
}

// NewTabBar は新しいTabBarを作成する
func NewTabBar(theme *Theme) *TabBar {
	return &TabBar{theme: theme}
}

// Layout はタブバーとブレッドクラムを描画する
func (tb *TabBar) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return tb.layoutTabs(gtx, state, th)
		}),
	)
}

func (tb *TabBar) layoutTabs(gtx C, state *editor.EditorState, th *material.Theme) D {
	tb.ensureClickables(len(state.Documents))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return D{Size: image.Pt(0, gtx.Dp(unit.Dp(20)))}
		}),
		layout.Rigid(func(gtx C) D {
			height := gtx.Dp(unit.Dp(40))
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height

			return withBg(gtx, func(gtx C, sz image.Point) {
				surfAlpha := tb.theme.Surface
				surfAlpha.A = 128
				fillBackground(gtx, surfAlpha, sz)
				defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
				fillBackground(gtx, tb.theme.BorderSubtle, image.Pt(sz.X, 1))
			}, func(gtx C) D {
				for i := range state.Documents {
					for tb.tabClicks[i].Clicked(gtx) {
						state.ActiveDocIdx = i
					}
				}
				for i := range state.Documents {
					for tb.closeClicks[i].Clicked(gtx) {
						state.CloseTab(i)
						break
					}
				}

				return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
					return layout.Center.Layout(gtx, func(gtx C) D {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								if state.SidebarOpen {
									return D{}
								}
								for tb.btnSidebar.Clicked(gtx) {
									state.SidebarOpen = true
								}
								return tb.btnSidebar.Layout(gtx, func(gtx C) D {
									return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
										return DrawSidebarOpenIcon(gtx, gtx.Dp(unit.Dp(14)), tb.theme.TextMuted)
									})
								})
							}),
							layout.Flexed(1, func(gtx C) D {
								if len(state.Documents) == 0 {
									lbl := material.Label(th, unit.Sp(12), "Catana Editor")
									lbl.Color = tb.theme.TextMuted
									lbl.LineHeightScale = 1.0
									return lbl.Layout(gtx)
								}

								children := make([]layout.FlexChild, 0, len(state.Documents))
								for i, doc := range state.Documents {
									idx := i
									currentDoc := doc
									children = append(children, layout.Rigid(func(gtx C) D {
										return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
											return tb.layoutTabChip(gtx, th, state, idx, currentDoc)
										})
									}))
								}
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
							}),
						)
					})
				})
			})
		}),
	)
}

func (tb *TabBar) ensureClickables(n int) {
	for len(tb.tabClicks) < n {
		tb.tabClicks = append(tb.tabClicks, widget.Clickable{})
	}
	if len(tb.tabClicks) > n {
		tb.tabClicks = tb.tabClicks[:n]
	}
	for len(tb.closeClicks) < n {
		tb.closeClicks = append(tb.closeClicks, widget.Clickable{})
	}
	if len(tb.closeClicks) > n {
		tb.closeClicks = tb.closeClicks[:n]
	}
}

func (tb *TabBar) layoutTabChip(gtx C, th *material.Theme, state *editor.EditorState, idx int, doc *editor.Document) D {
	isActive := idx == state.ActiveDocIdx
	bg := tb.theme.Surface
	textCol := tb.theme.TextMuted
	iconCol := tb.theme.FileIconColor(doc.FileName)
	if isActive {
		bg = tb.theme.SurfaceAlt
		textCol = tb.theme.Text
		iconCol = tb.theme.Accent
	}

	return tb.tabClicks[idx].Layout(gtx, func(gtx C) D {
		return withBg(gtx, func(gtx C, sz image.Point) {
			fillRoundRect(gtx, bg, sz, 6)
		}, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return DrawFileIcon(gtx, gtx.Dp(unit.Dp(12)), iconCol)
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(12), doc.FileName)
							lbl.Color = textCol
							lbl.LineHeightScale = 1.0
							lbl.MaxLines = 1
							return lbl.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx C) D {
						if doc.Modified {
							return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
								dot := gtx.Dp(unit.Dp(6))
								fillRoundRect(gtx, tb.theme.Accent, image.Pt(dot, dot), dot/2)
								return D{Size: image.Pt(dot, dot)}
							})
						}
						return D{}
					}),
					layout.Rigid(func(gtx C) D {
						return tb.closeClicks[idx].Layout(gtx, func(gtx C) D {
							icon := tb.theme.TextMuted
							if isActive {
								icon = tb.theme.Text
							}
							return DrawCloseIcon(gtx, gtx.Dp(unit.Dp(12)), icon)
						})
					}),
				)
			})
		})
	})
}
