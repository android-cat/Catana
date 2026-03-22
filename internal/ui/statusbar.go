package ui

import (
	"catana/internal/editor"
	"fmt"
	"image"
	"runtime"
	"sync/atomic"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// StatusBar はエディタ下部のステータスバー
type StatusBar struct {
	theme      *Theme
	fps        float64
	memUsage   atomic.Uint64 // バックグラウンド goroutine から更新
	lspLatency atomic.Int64  // LSP応答時間（ミリ秒）
	diagErrors atomic.Int32  // 診断エラー数
	diagWarns  atomic.Int32  // 診断警告数
}

// NewStatusBar は新しいStatusBarを作成する
func NewStatusBar(theme *Theme) *StatusBar {
	sb := &StatusBar{
		theme: theme,
		fps:   240,
	}
	// バックグラウンドでメモリ使用量を定期計測（メインスレッドのSTW回避）
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			sb.memUsage.Store(m.Alloc / (1024 * 1024))
		}
	}()
	return sb
}

// UpdateMetrics はパフォーマンスメトリクスを更新する
func (sb *StatusBar) UpdateMetrics() {
	// メモリ使用量はバックグラウンド goroutine が更新するため何もしない
}

// UpdateLSPMetrics はLSPの診断・レイテンシを更新する
func (sb *StatusBar) UpdateLSPMetrics(errors, warnings int, latencyMs int64) {
	sb.diagErrors.Store(int32(errors))
	sb.diagWarns.Store(int32(warnings))
	sb.lspLatency.Store(latencyMs)
}

// Layout はステータスバーを描画する
func (sb *StatusBar) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	height := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height

	return withFlatBg(gtx, sb.theme.StatusBar, func(gtx C) D {
		return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// 左側: Git ブランチ + 診断情報
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							// Git ブランチ
							layout.Rigid(func(gtx C) D {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										return DrawGitBranchIcon(gtx, gtx.Dp(unit.Dp(12)), sb.theme.StatusBarText)
									}),
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
											branch := "main"
											if state.GitBranch != "" {
												branch = state.GitBranch
											}
											lbl := material.Label(th, unit.Sp(10), branch)
											lbl.Color = sb.theme.StatusBarText
											return lbl.Layout(gtx)
										})
									}),
								)
							}),
							// 診断情報
							layout.Rigid(func(gtx C) D {
								return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
									errors := sb.diagErrors.Load()
									warns := sb.diagWarns.Load()
									lbl := material.Label(th, unit.Sp(10), fmt.Sprintf("✕ %d  ✓ %d", errors, warns))
									lbl.Color = sb.theme.StatusBarText
									if errors > 0 {
										lbl.Color = sb.theme.ErrorText
									}
									return lbl.Layout(gtx)
								})
							}),
						)
					})
				}),
				// スペーサー
				layout.Flexed(1, func(gtx C) D {
					return D{Size: image.Pt(0, height)}
				}),
				// 右側: パフォーマンス + 言語 + エンコーディング
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// パフォーマンスセクション（暗い背景）
						layout.Rigid(func(gtx C) D {
							return withFlatBg(gtx, sb.theme.SurfaceDark, func(gtx C) D {
								return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										// FPS
										layout.Rigid(func(gtx C) D {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx C) D {
													return DrawActivityIcon(gtx, gtx.Dp(unit.Dp(10)), sb.theme.PerfGreen)
												}),
												layout.Rigid(func(gtx C) D {
													return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
														lbl := material.Label(th, unit.Sp(10), fmt.Sprintf("%.0f fps", sb.fps))
														lbl.Color = sb.theme.PerfGreen
														return lbl.Layout(gtx)
													})
												}),
											)
										}),
										// メモリ
										layout.Rigid(func(gtx C) D {
											return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx C) D {
														return DrawCpuIcon(gtx, gtx.Dp(unit.Dp(10)), sb.theme.PerfYellow)
													}),
													layout.Rigid(func(gtx C) D {
														return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
															lbl := material.Label(th, unit.Sp(10), fmt.Sprintf("%d MB", sb.memUsage.Load()))
															lbl.Color = sb.theme.PerfYellow
															return lbl.Layout(gtx)
														})
													}),
												)
											})
										}),
										// LSP
										layout.Rigid(func(gtx C) D {
											return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx C) D {
														return DrawZapIcon(gtx, gtx.Dp(unit.Dp(10)), sb.theme.PerfPurple)
													}),
													layout.Rigid(func(gtx C) D {
														return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
															latency := sb.lspLatency.Load()
															text := fmt.Sprintf("%dms", latency)
															if latency == 0 {
																text = "---"
															}
															lbl := material.Label(th, unit.Sp(10), text)
															lbl.Color = sb.theme.PerfPurple
															return lbl.Layout(gtx)
														})
													}),
												)
											})
										}),
									)
								})
							})
						}),
						// 言語 + エンコーディング
						layout.Rigid(func(gtx C) D {
							doc := state.ActiveDocument()
							lang := "Plain"
							if doc != nil && doc.Language != "" {
								lang = doc.Language
							}
							return withFlatBg(gtx, sb.theme.SubtleBg, func(gtx C) D {
								return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx C) D {
											lbl := material.Label(th, unit.Sp(10), lang)
											lbl.Color = sb.theme.StatusBarText
											return lbl.Layout(gtx)
										}),
										layout.Rigid(func(gtx C) D {
											return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
												lbl := material.Label(th, unit.Sp(10), "UTF-8")
												lbl.Color = sb.theme.StatusBarText
												return lbl.Layout(gtx)
											})
										}),
									)
								})
							})
						}),
					)
				}),
			)
		})
	})
}

// 未使用インポート回避
var _ = op.Offset
