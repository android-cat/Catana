package ui

import (
	"catana/internal/editor"
	"image"
	"image/color"
	"log"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// withBg は content をレイアウトし、そのサイズに合わせて背景を描画するヘルパー。
// layout.Stack の Expanded パターンの代替（Gio v0.9 で Expanded は親の Max を取る問題の回避）。
func withBg(gtx C, bgFn func(gtx C, sz image.Point), content func(gtx C) D) D {
	macro := op.Record(gtx.Ops)
	dims := content(gtx)
	call := macro.Stop()
	bgFn(gtx, dims.Size)
	call.Add(gtx.Ops)
	return dims
}

// withRoundBg は角丸背景を描画してからコンテンツを描画する。
func withRoundBg(gtx C, bgColor color.NRGBA, radius int, content func(gtx C) D) D {
	return withBg(gtx, func(gtx C, sz image.Point) {
		fillRoundRect(gtx, bgColor, sz, radius)
	}, content)
}

// withFlatBg はフラット（四角）背景を描画してからコンテンツを描画する。
func withFlatBg(gtx C, bgColor color.NRGBA, content func(gtx C) D) D {
	return withBg(gtx, func(gtx C, sz image.Point) {
		fillBackground(gtx, bgColor, sz)
	}, content)
}

// OmniBar はオムニバー（AI/CMD/TERM統合入力）
type OmniBar struct {
	theme         *Theme
	input         widget.Editor
	btnAI         widget.Clickable
	btnCmd        widget.Clickable
	btnTerm       widget.Clickable
	btnClose      widget.Clickable
	btnTermClear  widget.Clickable // TERMモード履歴クリアボタン
	requestFocus  bool
	termList      widget.List // TERM履歴スクロール用
	termScrollEnd bool        // 末尾への自動スクロール要求
	termLastCount int         // 前フレームの履歴数（新規エントリ検出用）
	termLastDone  bool        // 前フレームの最終エントリDone状態
}

// NewOmniBar は新しいOmniBarを作成する
func NewOmniBar(theme *Theme) *OmniBar {
	ob := &OmniBar{theme: theme}
	ob.input.SingleLine = true
	ob.input.Submit = true // Enterでサブミットイベント発火
	ob.termList.Axis = layout.Vertical
	return ob
}

// Layout はオムニバーをコンパクトなフローティングパネルとして描画する
func (ob *OmniBar) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	// モード切替クリック処理
	for ob.btnAI.Clicked(gtx) {
		state.OmniMode = editor.ModeAI
		state.ShowOmniChat = true
		ob.requestFocus = true
	}
	for ob.btnCmd.Clicked(gtx) {
		state.OmniMode = editor.ModeCmd
		state.ShowOmniChat = false
		ob.requestFocus = true
	}
	for ob.btnTerm.Clicked(gtx) {
		state.OmniMode = editor.ModeTerm
		// 履歴がある場合はチャットエリアを表示
		state.ShowOmniChat = len(state.TermHistory) > 0
		ob.requestFocus = true
	}
	for ob.btnClose.Clicked(gtx) {
		state.ShowOmniChat = false
	}
	for ob.btnTermClear.Clicked(gtx) {
		state.ClearTermHistory()
		state.ShowOmniChat = false
	}

	// サブミットイベント処理（Enterキー）
	for {
		evt, ok := ob.input.Update(gtx)
		if !ok {
			break
		}
		if submit, ok := evt.(widget.SubmitEvent); ok {
			ob.handleSubmit(submit.Text, state)
		}
	}

	if ob.requestFocus {
		gtx.Execute(key.FocusCmd{Tag: &ob.input})
		ob.requestFocus = false
	}
	if state.OmniMode == editor.ModeAI && gtx.Focused(&ob.input) {
		state.ShowOmniChat = true
	}
	if state.OmniMode == editor.ModeTerm && gtx.Focused(&ob.input) && len(state.TermHistory) > 0 {
		state.ShowOmniChat = true
	}

	// 最大幅768dp
	maxWidth := gtx.Dp(unit.Dp(768))
	if maxWidth > gtx.Constraints.Max.X {
		maxWidth = gtx.Constraints.Max.X
	}
	gtx.Constraints.Max.X = maxWidth
	gtx.Constraints.Min.X = 0

	// コンテンツを先に記録してサイズを取得（macro パターン）
	macro := op.Record(gtx.Ops)
	contentDims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// チャット領域（AI or TERMモードで showOmniChat の場合）
		layout.Rigid(func(gtx C) D {
			if !state.ShowOmniChat {
				return D{}
			}
			switch state.OmniMode {
			case editor.ModeAI:
				return ob.layoutChatArea(gtx, state, th)
			case editor.ModeTerm:
				return ob.layoutTermChatArea(gtx, state, th)
			default:
				return D{}
			}
		}),
		// モード切替バー
		layout.Rigid(func(gtx C) D {
			return ob.layoutModeSwitcher(gtx, state, th)
		}),
		// 入力エリア
		layout.Rigid(func(gtx C) D {
			return ob.layoutInput(gtx, state, th)
		}),
	)
	contentCall := macro.Stop()

	// 背景をコンテンツサイズに合わせて描画
	size := contentDims.Size
	// ボーダー（角丸）
	fillRoundRect(gtx, ob.theme.OmniBarBorder, size, 16)
	// 内側背景
	func() {
		defer op.Offset(image.Pt(1, 1)).Push(gtx.Ops).Pop()
		fillRoundRect(gtx, ob.theme.OmniBarBg, image.Pt(size.X-2, size.Y-2), 15)
	}()

	// コンテンツを上に再生
	contentCall.Add(gtx.Ops)

	return contentDims
}

func (ob *OmniBar) layoutModeSwitcher(gtx C, state *editor.EditorState, th *material.Theme) D {
	return withFlatBg(gtx, nrgba(0, 0, 0, 153), func(gtx C) D {
		return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// AI ボタン
				layout.Rigid(func(gtx C) D {
					return ob.modeButton(gtx, th, &ob.btnAI, "AI", state.OmniMode == editor.ModeAI, ob.theme.AIPurple, ob.theme.AIPurpleBg)
				}),
				// CMD ボタン
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						return ob.modeButton(gtx, th, &ob.btnCmd, "CMD", state.OmniMode == editor.ModeCmd, ob.theme.Accent, ob.theme.AccentBg)
					})
				}),
				// TERM ボタン
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						return ob.modeButton(gtx, th, &ob.btnTerm, "TERM", state.OmniMode == editor.ModeTerm, hexColor(0x4ADE80), nrgba(0x22, 0xC5, 0x5E, 51))
					})
				}),
				// スペーサー
				layout.Flexed(1, func(gtx C) D {
					return D{}
				}),
				// コンテキストピル（AIモード時）
				layout.Rigid(func(gtx C) D {
					if state.OmniMode != editor.ModeAI {
						return D{}
					}
					doc := state.ActiveDocument()
					if doc == nil {
						return D{}
					}
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return withRoundBg(gtx, nrgba(0xFF, 0xFF, 0xFF, 13), 4, func(gtx C) D {
							return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										return DrawFileIcon(gtx, gtx.Dp(unit.Dp(10)), ob.theme.Accent)
									}),
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
											lbl := material.Label(th, unit.Sp(9), doc.FileName)
											lbl.Color = ob.theme.TextMuted
											return lbl.Layout(gtx)
										})
									}),
								)
							})
						})
					})
				}),
				// 閉じるボタン
				layout.Rigid(func(gtx C) D {
					return ob.btnClose.Layout(gtx, func(gtx C) D {
						return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
							return DrawCloseIcon(gtx, gtx.Dp(unit.Dp(14)), ob.theme.TextMuted)
						})
					})
				}),
			)
		})
	})
}

func (ob *OmniBar) modeButton(gtx C, th *material.Theme, btn *widget.Clickable, label string, active bool, activeColor, activeBg color.NRGBA) D {
	return btn.Layout(gtx, func(gtx C) D {
		bgColor := nrgba(0, 0, 0, 0) // 透明
		textColor := ob.theme.TextMuted
		if active {
			bgColor = activeBg
			textColor = activeColor
		}
		return withRoundBg(gtx, bgColor, 6, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(12), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// アイコン
					layout.Rigid(func(gtx C) D {
						iconSize := gtx.Dp(unit.Dp(12))
						switch label {
						case "AI":
							return DrawSparklesIcon(gtx, iconSize, textColor)
						case "CMD":
							return DrawCommandIcon(gtx, iconSize, textColor)
						case "TERM":
							return DrawTerminalIcon(gtx, iconSize, textColor)
						}
						return D{}
					}),
					// テキスト
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(11), label)
							lbl.Color = textColor
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

func (ob *OmniBar) layoutInput(gtx C, state *editor.EditorState, th *material.Theme) D {
	return withFlatBg(gtx, nrgba(0x0A, 0x0A, 0x0A, 204), func(gtx C) D {
		return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// モードアイコン
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						iconSize := gtx.Dp(unit.Dp(18))
						switch state.OmniMode {
						case editor.ModeAI:
							return DrawSparklesIcon(gtx, iconSize, ob.theme.AIPurple)
						case editor.ModeCmd:
							return DrawCommandIcon(gtx, iconSize, ob.theme.Accent)
						case editor.ModeTerm:
							return DrawTerminalIcon(gtx, iconSize, hexColor(0x4ADE80))
						}
						return D{}
					})
				}),
				// 入力フィールド
				layout.Flexed(1, func(gtx C) D {
					placeholder := "Ask AI to edit, explain, or generate code..."
					switch state.OmniMode {
					case editor.ModeCmd:
						placeholder = "Search files, commands, or settings..."
					case editor.ModeTerm:
						placeholder = "Execute terminal command..."
					}
					ed := material.Editor(th, &ob.input, placeholder)
					ed.Color = ob.theme.Text
					ed.HintColor = ob.theme.TextDark
					ed.TextSize = unit.Sp(15)
					return ed.Layout(gtx)
				}),
				// 送信ボタン（AIモード時のみ）
				layout.Rigid(func(gtx C) D {
					if state.OmniMode != editor.ModeAI {
						return D{}
					}
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return withRoundBg(gtx, nrgba(0xA8, 0x55, 0xF7, 25), 8, func(gtx C) D {
							return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx C) D {
								return DrawEnterIcon(gtx, gtx.Dp(unit.Dp(16)), ob.theme.AIPurple)
							})
						})
					})
				}),
			)
		})
	})
}

func (ob *OmniBar) layoutChatArea(gtx C, state *editor.EditorState, th *material.Theme) D {
	// AIチャット表示エリア（ウィンドウの半分まで）
	maxH := gtx.Constraints.Max.Y / 2
	gtx.Constraints.Max.Y = maxH
	gtx.Constraints.Min.Y = 0

	return withBg(gtx, func(gtx C, sz image.Point) {
		// 背景: bg-black/40
		fillBackground(gtx, nrgba(0, 0, 0, 102), sz)
		// 下ボーダー: border-white/5
		defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
		fillBackground(gtx, nrgba(0xFF, 0xFF, 0xFF, 13), image.Pt(sz.X, 1))
	}, func(gtx C) D {
		return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				// ユーザーメッセージ
				layout.Rigid(func(gtx C) D {
					return ob.layoutUserMessage(gtx, th)
				}),
				// AI応答
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
						return ob.layoutAIMessage(gtx, state, th)
					})
				}),
			)
		})
	})
}

// layoutTermChatArea はTERMモードのコマンド履歴チャットエリアを描画する
func (ob *OmniBar) layoutTermChatArea(gtx C, state *editor.EditorState, th *material.Theme) D {
	if len(state.TermHistory) == 0 {
		return D{}
	}

	maxH := gtx.Constraints.Max.Y / 2
	gtx.Constraints.Max.Y = maxH
	gtx.Constraints.Min.Y = 0

	count := len(state.TermHistory)

	// 新しいエントリが追加されたか、最終エントリの出力が完了した場合に自動スクロール
	lastDone := false
	if count > 0 {
		lastDone = state.TermHistory[count-1].Done
	}
	if count != ob.termLastCount || (lastDone && !ob.termLastDone) {
		ob.termScrollEnd = true
	}
	ob.termLastCount = count
	ob.termLastDone = lastDone

	if ob.termScrollEnd {
		ob.termList.Position.First = count - 1
		ob.termList.Position.Offset = 1 << 20
		ob.termList.Position.BeforeEnd = false
		ob.termScrollEnd = false
	}

	return withBg(gtx, func(gtx C, sz image.Point) {
		fillBackground(gtx, nrgba(0, 0, 0, 102), sz)
		defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
		fillBackground(gtx, nrgba(0xFF, 0xFF, 0xFF, 13), image.Pt(sz.X, 1))
	}, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// コマンド履歴リスト
			layout.Flexed(1, func(gtx C) D {
				return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
					listStyle := material.List(th, &ob.termList)
					listStyle.AnchorStrategy = material.Overlay
					return listStyle.Layout(gtx, count, func(gtx C, i int) D {
						entry := state.TermHistory[i]
						var inset layout.Inset
						if i > 0 {
							inset.Top = unit.Dp(12)
						}
						return inset.Layout(gtx, func(gtx C) D {
							return ob.layoutTermHistoryEntry(gtx, th, entry)
						})
					})
				})
			}),
			// クリアボタン（最下部）
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Bottom: unit.Dp(6), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
					return layout.E.Layout(gtx, func(gtx C) D {
						return ob.btnTermClear.Layout(gtx, func(gtx C) D {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return DrawTrashIcon(gtx, gtx.Dp(unit.Dp(13)), ob.theme.GitDeleted)
								}),
								layout.Rigid(func(gtx C) D {
									return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
										lbl := material.Label(th, unit.Sp(11), "Clear")
										lbl.Color = ob.theme.GitDeleted
										return lbl.Layout(gtx)
									})
								}),
							)
						})
					})
				})
			}),
		)
	})
}

// layoutTermHistoryEntry はTERMモードの1つのコマンド履歴エントリを描画する
func (ob *OmniBar) layoutTermHistoryEntry(gtx C, th *material.Theme, entry *editor.TermHistoryEntry) D {
	// 実行中のエントリがある場合は再描画を要求
	if entry.Running {
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// ディレクトリ表示
		layout.Rigid(func(gtx C) D {
			if entry.Dir == "" {
				return D{}
			}
			return layout.Inset{Left: unit.Dp(40), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(10), entry.Dir)
				lbl.Color = ob.theme.TextDark
				return lbl.Layout(gtx)
			})
		}),
		// コマンド行
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				// ターミナルアバター
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						sz := gtx.Dp(unit.Dp(28))
						fillRoundRect(gtx, nrgba(0x22, 0xC5, 0x5E, 51), image.Pt(sz, sz), sz/2)
						return layout.Stack{}.Layout(gtx,
							layout.Stacked(func(gtx C) D {
								gtx.Constraints.Min = image.Pt(sz, sz)
								gtx.Constraints.Max = image.Pt(sz, sz)
								return layout.Center.Layout(gtx, func(gtx C) D {
									return DrawTerminalIcon(gtx, gtx.Dp(unit.Dp(14)), hexColor(0x4ADE80))
								})
							}),
						)
					})
				}),
				// コマンドテキスト
				layout.Flexed(1, func(gtx C) D {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(14), "$ "+entry.Command)
						lbl.Color = hexColor(0x4ADE80)
						return lbl.Layout(gtx)
					})
				}),
			)
		}),
		// 出力行（実行中 or 完了）
		layout.Rigid(func(gtx C) D {
			if entry.Running {
				return layout.Inset{Left: unit.Dp(40), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					return withRoundBg(gtx, nrgba(0xFF, 0xFF, 0xFF, 8), 6, func(gtx C) D {
						return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(12), "⏳ 実行中...")
							lbl.Color = hexColor(0xFBBF24)
							return lbl.Layout(gtx)
						})
					})
				})
			}
			if entry.Done {
				output := entry.Output
				if output == "" {
					output = "(出力なし)"
				}
				// 出力ボーダー色: 成功=緑, 失敗=赤
				borderColor := nrgba(0x22, 0xC5, 0x5E, 40)
				statusLabel := "✓"
				statusColor := hexColor(0x4ADE80)
				if !entry.ExitOK {
					borderColor = nrgba(0xEF, 0x44, 0x44, 40)
					statusLabel = "✗"
					statusColor = hexColor(0xF87171)
				}
				return layout.Inset{Left: unit.Dp(40), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					// ボーダー付き出力ボックス
					return withRoundBg(gtx, borderColor, 7, func(gtx C) D {
						return layout.UniformInset(unit.Dp(1)).Layout(gtx, func(gtx C) D {
							return withRoundBg(gtx, nrgba(0x0A, 0x0A, 0x0A, 230), 6, func(gtx C) D {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									// 出力ヘッダー
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx C) D {
													lbl := material.Label(th, unit.Sp(10), statusLabel+" OUTPUT")
													lbl.Color = statusColor
													return lbl.Layout(gtx)
												}),
											)
										})
									}),
									// 出力セパレーター
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
											fillBackground(gtx, nrgba(0xFF, 0xFF, 0xFF, 10), image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1))))
											return D{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}
										})
									}),
									// 出力テキスト
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
											lbl := material.Label(th, unit.Sp(12), output)
											lbl.Color = hexColor(0xD4D4D4)
											return lbl.Layout(gtx)
										})
									}),
								)
							})
						})
					})
				})
			}
			return D{}
		}),
	)
}

func (ob *OmniBar) layoutUserMessage(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// ユーザーアバター
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				sz := gtx.Dp(unit.Dp(28))
				// 固定サイズのアバター: fillRoundRect で直接描画
				fillRoundRect(gtx, hexColor(0x374151), image.Pt(sz, sz), sz/2)
				return layout.Stack{}.Layout(gtx,
					layout.Stacked(func(gtx C) D {
						gtx.Constraints.Min = image.Pt(sz, sz)
						gtx.Constraints.Max = image.Pt(sz, sz)
						return layout.Center.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(11), "U")
							lbl.Color = ob.theme.Text
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		}),
		// メッセージ本文
		layout.Flexed(1, func(gtx C) D {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
				lbl := material.Label(th, unit.Sp(14), "new 関数をリファクタリングして、パニック（unwrap）せずに正しくResultを返すように修正して。")
				lbl.Color = ob.theme.Text
				return lbl.Layout(gtx)
			})
		}),
	)
}

func (ob *OmniBar) layoutAIMessage(gtx C, state *editor.EditorState, th *material.Theme) D {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// AIアバター
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				sz := gtx.Dp(unit.Dp(28))
				fillRoundRect(gtx, ob.theme.AIPurpleBg, image.Pt(sz, sz), 8)
				return layout.Stack{}.Layout(gtx,
					layout.Stacked(func(gtx C) D {
						gtx.Constraints.Min = image.Pt(sz, sz)
						gtx.Constraints.Max = image.Pt(sz, sz)
						return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx C) D {
							return DrawSparklesIcon(gtx, gtx.Dp(unit.Dp(14)), ob.theme.AIPurple)
						})
					}),
				)
			})
		}),
		// AI応答テキスト + Diff
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// 説明テキスト
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(14), "了解しました。unwrap() を削除し、呼び出し元にエラーを伝播させるために Result<Self, EngineError> を返すように変更しました。以下の修正案を確認してください。")
						lbl.Color = ob.theme.Text
						return lbl.Layout(gtx)
					})
				}),
				// Diff UIボックス
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
						return ob.layoutDiffBox(gtx, th)
					})
				}),
			)
		}),
	)
}

func (ob *OmniBar) layoutDiffBox(gtx C, th *material.Theme) D {
	// ボーダー + 内側背景を macro パターンで描画
	return withRoundBg(gtx, hexColor(0x333333), 8, func(gtx C) D {
		return layout.UniformInset(unit.Dp(1)).Layout(gtx, func(gtx C) D {
			return withRoundBg(gtx, hexColor(0x050505), 7, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return ob.layoutDiffHeader(gtx, th)
					}),
					layout.Rigid(func(gtx C) D {
						return ob.layoutDiffContent(gtx, th)
					}),
				)
			})
		})
	})
}

func (ob *OmniBar) layoutDiffHeader(gtx C, th *material.Theme) D {
	return withFlatBg(gtx, hexColor(0x151515), func(gtx C) D {
		return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				// ファイル名
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return DrawFileIcon(gtx, gtx.Dp(unit.Dp(12)), ob.theme.TextMuted)
						}),
						layout.Rigid(func(gtx C) D {
							return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
								lbl := material.Label(th, unit.Sp(12), "engine.rs")
								lbl.Color = ob.theme.TextMuted
								return lbl.Layout(gtx)
							})
						}),
					)
				}),
				layout.Flexed(1, func(gtx C) D { return D{} }),
				// Apply ボタン
				layout.Rigid(func(gtx C) D {
					return withRoundBg(gtx, nrgba(0xA8, 0x55, 0xF7, 38), 4, func(gtx C) D {
						return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(11), "\u2713 Apply (\u2318Enter)")
							lbl.Color = ob.theme.AIPurple
							return lbl.Layout(gtx)
						})
					})
				}),
				// 閉じるボタン
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return withRoundBg(gtx, nrgba(0xFF, 0xFF, 0xFF, 13), 4, func(gtx C) D {
							return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
								return DrawCloseIcon(gtx, gtx.Dp(unit.Dp(14)), ob.theme.TextMuted)
							})
						})
					})
				}),
			)
		})
	})
}

func (ob *OmniBar) layoutDiffContent(gtx C, th *material.Theme) D {
	type diffLine struct {
		text string
		kind int // 0=context, 1=delete, 2=add
	}
	lines := []diffLine{
		{"- pub async fn new(file_path: &str) -> Self {", 1},
		{"+ pub async fn new(file_path: &str) -> Result<Self, EngineError> {", 2},
		{"      let buffer = RopeBuffer::load(file_path).await", 0},
		{"-         .unwrap();", 1},
		{"+         .map_err(EngineError::IoError)?;", 2},
		{"      ...", 0},
		{"-     Self { buffer: buffer_arc, lsp_client: lsp, gpu_renderer: renderer }", 1},
		{"+     Ok(Self { buffer: buffer_arc, lsp_client: lsp, gpu_renderer: renderer })", 2},
	}

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		children := make([]layout.FlexChild, len(lines))
		for i := range lines {
			line := lines[i]
			children[i] = layout.Rigid(func(gtx C) D {
				var bgColor color.NRGBA
				var textColor color.NRGBA
				switch line.kind {
				case 1: // 削除
					bgColor = nrgba(0xEF, 0x44, 0x44, 25)
					textColor = hexColor(0xF87171)
				case 2: // 追加
					bgColor = nrgba(0x22, 0xC5, 0x5E, 38)
					textColor = hexColor(0x4ADE80)
				default:
					textColor = ob.theme.TextMuted
				}
				if bgColor.A > 0 {
					return withFlatBg(gtx, bgColor, func(gtx C) D {
						return layout.Inset{Left: unit.Dp(8), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(12), line.text)
							lbl.Color = textColor
							return lbl.Layout(gtx)
						})
					})
				}
				return layout.Inset{Left: unit.Dp(8), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
					lbl := material.Label(th, unit.Sp(12), line.text)
					lbl.Color = textColor
					return lbl.Layout(gtx)
				})
			})
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// handleSubmit はオムニバーのサブミットイベントを処理する
func (ob *OmniBar) handleSubmit(text string, state *editor.EditorState) {
	if text == "" {
		return
	}

	switch state.OmniMode {
	case editor.ModeTerm:
		// インタラクティブコマンドはターミナルパネルに転送
		if editor.IsInteractiveCommand(text) {
			ob.runInTerminalPanel(text, state)
			ob.input.SetText("")
			return
		}
		// 永続シェルを起動（未起動の場合）
		if err := state.EnsureTermShell(); err != nil {
			log.Printf("[TermShell] 起動失敗: %v", err)
			return
		}
		// 現在のcwdを取得して履歴エントリを作成
		cwd := state.TermShell.Cwd()
		entry := &editor.TermHistoryEntry{
			Command: text,
			Dir:     cwd,
			Running: true,
		}
		state.TermHistory = append(state.TermHistory, entry)
		state.ShowOmniChat = true
		ob.termScrollEnd = true
		// 永続シェルでコマンドを実行
		shell := state.TermShell
		go func() {
			output, _, exitOK := shell.Execute(text)
			entry.ExitOK = exitOK
			entry.Output = output
			entry.Running = false
			entry.Done = true
		}()
		ob.input.SetText("")
	case editor.ModeCmd:
		// コマンドパレット検索（将来実装）
		ob.input.SetText("")
	case editor.ModeAI:
		// AI送信（将来実装）
		ob.input.SetText("")
	}
}

// runInTerminalPanel はインタラクティブコマンドをターミナルパネルで実行する
func (ob *OmniBar) runInTerminalPanel(command string, state *editor.EditorState) {
	// ターミナルがなければ新規作成
	if state.Terminal.Count() == 0 {
		if _, err := state.Terminal.NewTerminal(24, 80); err != nil {
			log.Printf("[Terminal] 作成失敗: %v", err)
			return
		}
	}
	term := state.Terminal.ActiveTerminal()
	if term == nil {
		return
	}
	// TermShellのcwdに合わせてcdしてからコマンド実行
	if state.TermShell != nil && state.TermShell.IsAlive() {
		cwd := state.TermShell.Cwd()
		if cwd != "" {
			term.WriteString("cd " + cwd + "\r")
		}
	}
	term.WriteString(command + "\r")
	// ターミナルパネルを表示してフォーカス
	state.ShowTerminal = true
	state.TerminalFocusRequest = true
}
