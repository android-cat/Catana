package ui

import (
	"catana/internal/ai"
	"catana/internal/editor"
	"context"
	"image"
	"image/color"
	"log"
	"sort"
	"strings"

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
	btnAIClear    widget.Clickable // AIチャットクリアボタン
	requestFocus  bool
	termList      widget.List // TERM履歴スクロール用
	termScrollEnd bool        // 末尾への自動スクロール要求
	termLastCount int         // 前フレームの履歴数（新規エントリ検出用）
	termLastDone  bool        // 前フレームの最終エントリDone状態
	// AIチャット用
	aiChatList      widget.List // AIチャットスクロール用
	aiScrollEnd     bool        // AIチャット末尾自動スクロール
	aiLastCount     int         // 前フレームのAIメッセージ数
	aiLastStreaming bool        // 前フレームのストリーミング状態
	// Diff Apply用
	diffApplyBtns []widget.Clickable // 各diffブロックのApplyボタン
	diffDismiss   []widget.Clickable // 各diffブロックの閉じるボタン
	diffHidden    map[int]bool       // 非表示にしたdiffブロックのインデックス
	diffApplied   map[int]bool       // 適用済みdiffブロックのインデックス
	// モデル選択用
	btnModel          widget.Clickable                   // モデル選択ピル
	showModelMenu     bool                               // モデルドロップダウン表示
	modelEntries      []modelEntry                       // フラットなモデルエントリ一覧
	modelBtns         []widget.Clickable                 // 各モデルエントリのクリックボタン
	modelList         widget.List                        // モデルドロップダウンスクロール用
	modelCache        map[ai.ProviderType][]ai.ModelInfo // プロバイダ別モデルキャッシュ
	modelCacheReady   bool                               // キャッシュ取得完了フラグ
	modelCacheLoading bool                               // モデル取得中フラグ
	modelResultCh     chan modelFetchResult              // 非同期取得結果チャネル
}

// modelEntry はモデルドロップダウンの1エントリ
type modelEntry struct {
	Provider ai.ProviderType
	Model    ai.ModelInfo
	IsHeader bool   // プロバイダ名ヘッダー行
	Label    string // 表示ラベル
}

// modelFetchResult は非同期モデル取得の結果
type modelFetchResult struct {
	Models map[ai.ProviderType][]ai.ModelInfo
}

// NewOmniBar は新しいOmniBarを作成する
func NewOmniBar(theme *Theme) *OmniBar {
	ob := &OmniBar{theme: theme}
	ob.input.SingleLine = true
	ob.input.Submit = true // Enterでサブミットイベント発火
	ob.termList.Axis = layout.Vertical
	ob.aiChatList.Axis = layout.Vertical
	ob.modelList.Axis = layout.Vertical
	ob.diffHidden = make(map[int]bool)
	ob.diffApplied = make(map[int]bool)
	ob.modelCache = make(map[ai.ProviderType][]ai.ModelInfo)
	ob.modelResultCh = make(chan modelFetchResult, 1)
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
	for ob.btnAIClear.Clicked(gtx) {
		state.AIClearHistory()
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
	return withFlatBg(gtx, ob.theme.OverlayBg, func(gtx C) D {
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
						return ob.modeButton(gtx, th, &ob.btnTerm, "TERM", state.OmniMode == editor.ModeTerm, ob.theme.AIGreen, ob.theme.AIGreenBg)
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
					return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						return withRoundBg(gtx, ob.theme.Separator, 4, func(gtx C) D {
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
				// モデル選択ピル（AIモード時）
				layout.Rigid(func(gtx C) D {
					if state.OmniMode != editor.ModeAI {
						return D{}
					}
					return ob.layoutModelSelector(gtx, state, th)
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
		bgColor := color.NRGBA{} // 透明
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

// layoutModelSelector はAIモード時のモデル選択ピルとドロップダウンを描画する
func (ob *OmniBar) layoutModelSelector(gtx C, state *editor.EditorState, th *material.Theme) D {
	if state.AI == nil {
		return D{}
	}

	// 非同期モデル取得結果をポーリング
	select {
	case result := <-ob.modelResultCh:
		ob.modelCache = result.Models
		ob.modelCacheReady = true
		ob.modelCacheLoading = false
		ob.buildModelEntries(state)
	default:
	}

	// モデルエントリのクリック処理
	for i, entry := range ob.modelEntries {
		if entry.IsHeader || i >= len(ob.modelBtns) {
			continue
		}
		for ob.modelBtns[i].Clicked(gtx) {
			if err := state.AI.SetActiveModel(entry.Provider, entry.Model.ID); err == nil {
				ob.showModelMenu = false
				// 設定に永続化
				if state.Config != nil {
					state.Config.AI.ActiveProvider = string(entry.Provider)
					state.Config.AI.ActiveModel = entry.Model.ID
					go state.Config.Save()
				}
			}
		}
	}

	// ピルクリックでメニュー表示トグル＋モデル一覧取得開始
	for ob.btnModel.Clicked(gtx) {
		ob.showModelMenu = !ob.showModelMenu
		if ob.showModelMenu && !ob.modelCacheReady && !ob.modelCacheLoading {
			ob.modelCacheLoading = true
			mgr := state.AI
			ch := ob.modelResultCh
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*1000*1000*1000) // 10s
				defer cancel()
				models := mgr.ListAllModels(ctx)
				ch <- modelFetchResult{Models: models}
			}()
		}
		if ob.showModelMenu && ob.modelCacheReady {
			ob.buildModelEntries(state)
		}
	}

	// 現在のモデル名を表示
	activeLabel := state.AI.ActiveModel()
	if activeLabel == "" {
		activeLabel = providerDisplayName(state.AI.ActiveType())
	}

	return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// ピルボタン
			layout.Rigid(func(gtx C) D {
				return ob.btnModel.Layout(gtx, func(gtx C) D {
					return withRoundBg(gtx, ob.theme.AIPurpleBg, 4, func(gtx C) D {
						return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return DrawSparklesIcon(gtx, gtx.Dp(unit.Dp(10)), ob.theme.AIPurple)
								}),
								layout.Rigid(func(gtx C) D {
									return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
										lbl := material.Label(th, unit.Sp(9), activeLabel)
										lbl.Color = ob.theme.AIPurple
										return lbl.Layout(gtx)
									})
								}),
								layout.Rigid(func(gtx C) D {
									return layout.Inset{Left: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
										lbl := material.Label(th, unit.Sp(7), "▼")
										lbl.Color = ob.theme.AIPurple
										return lbl.Layout(gtx)
									})
								}),
							)
						})
					})
				})
			}),
			// ドロップダウンメニュー
			layout.Rigid(func(gtx C) D {
				if !ob.showModelMenu {
					return D{}
				}
				return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
					return withRoundBg(gtx, ob.theme.Surface, 6, func(gtx C) D {
						return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
							return withRoundBg(gtx, ob.theme.SurfaceAlt, 5, func(gtx C) D {
								return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
									if ob.modelCacheLoading && !ob.modelCacheReady {
										// ローディング表示
										lbl := material.Label(th, unit.Sp(10), "モデル取得中...")
										lbl.Color = ob.theme.TextMuted
										return layout.Inset{Left: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, lbl.Layout)
									}
									if len(ob.modelEntries) == 0 {
										// フォールバック: プロバイダ名のみ表示
										return ob.layoutFallbackProviders(gtx, state, th)
									}
									// モデル一覧（スクロール可能、最大高さ制限）
									maxH := gtx.Dp(unit.Dp(300))
									if gtx.Constraints.Max.Y > maxH {
										gtx.Constraints.Max.Y = maxH
									}
									return material.List(th, &ob.modelList).Layout(gtx, len(ob.modelEntries), func(gtx C, i int) D {
										entry := ob.modelEntries[i]
										if entry.IsHeader {
											return ob.layoutModelGroupHeader(gtx, th, entry.Label)
										}
										if i >= len(ob.modelBtns) {
											return D{}
										}
										activeType := state.AI.ActiveType()
										activeModel := state.AI.ActiveModel()
										isActive := entry.Provider == activeType && entry.Model.ID == activeModel
										return ob.layoutModelItem(gtx, th, &ob.modelBtns[i], entry, isActive)
									})
								})
							})
						})
					})
				})
			}),
		)
	})
}

// buildModelEntries はキャッシュからフラットなモデルエントリ一覧を構築する
func (ob *OmniBar) buildModelEntries(state *editor.EditorState) {
	ob.modelEntries = ob.modelEntries[:0]

	// プロバイダの並び順を安定化
	providerOrder := []ai.ProviderType{
		ai.ProviderOpenAI, ai.ProviderAnthropic, ai.ProviderGemini,
		ai.ProviderCopilot, ai.ProviderOllama,
	}

	for _, pt := range providerOrder {
		models, ok := ob.modelCache[pt]
		if !ok || len(models) == 0 {
			continue
		}
		// プロバイダヘッダー
		ob.modelEntries = append(ob.modelEntries, modelEntry{
			Provider: pt,
			IsHeader: true,
			Label:    providerDisplayName(pt),
		})
		// モデル一覧をソート
		sort.Slice(models, func(i, j int) bool {
			return models[i].ID < models[j].ID
		})
		for _, m := range models {
			ob.modelEntries = append(ob.modelEntries, modelEntry{
				Provider: pt,
				Model:    m,
				Label:    m.Name,
			})
		}
	}

	// クリックボタンを必要数分確保
	if len(ob.modelBtns) < len(ob.modelEntries) {
		ob.modelBtns = make([]widget.Clickable, len(ob.modelEntries))
	}
}

// layoutModelGroupHeader はプロバイダ名のグループヘッダーを描画する
func (ob *OmniBar) layoutModelGroupHeader(gtx C, th *material.Theme, label string) D {
	return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
		lbl := material.Label(th, unit.Sp(9), label)
		lbl.Color = ob.theme.TextMuted
		return lbl.Layout(gtx)
	})
}

// layoutModelItem はモデル選択アイテムを描画する
func (ob *OmniBar) layoutModelItem(gtx C, th *material.Theme, btn *widget.Clickable, entry modelEntry, active bool) D {
	return btn.Layout(gtx, func(gtx C) D {
		bgCol := color.NRGBA{}
		textCol := ob.theme.Text
		if active {
			bgCol = ob.theme.AIPurpleBg
			textCol = ob.theme.AIPurple
		} else if btn.Hovered() {
			bgCol = ob.theme.Separator
		}
		return withRoundBg(gtx, bgCol, 4, func(gtx C) D {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(16), Top: unit.Dp(3), Bottom: unit.Dp(3)}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						if active {
							lbl := material.Label(th, unit.Sp(10), "✓")
							lbl.Color = ob.theme.AIPurple
							return lbl.Layout(gtx)
						}
						return D{Size: image.Pt(gtx.Dp(unit.Dp(10)), 0)}
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(10), entry.Label)
							lbl.Color = textCol
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

// layoutFallbackProviders はモデル一覧が取得できない場合のフォールバック表示
func (ob *OmniBar) layoutFallbackProviders(gtx C, state *editor.EditorState, th *material.Theme) D {
	providers := state.AI.ConfiguredProviders()
	children := make([]layout.FlexChild, 0, len(providers))
	// ボタンを確保
	if len(ob.modelBtns) < len(providers) {
		ob.modelBtns = make([]widget.Clickable, len(providers))
	}
	for i, pt := range providers {
		idx := i
		provType := pt
		for ob.modelBtns[idx].Clicked(gtx) {
			_ = state.AI.SetActive(provType)
			ob.showModelMenu = false
		}
		isActive := state.AI.ActiveType() == pt
		entry := modelEntry{Provider: pt, Label: providerDisplayName(pt)}
		children = append(children, layout.Rigid(func(gtx C) D {
			return ob.layoutModelItem(gtx, th, &ob.modelBtns[idx], entry, isActive)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func providerDisplayName(pt ai.ProviderType) string {
	switch pt {
	case ai.ProviderOpenAI:
		return "OpenAI"
	case ai.ProviderAnthropic:
		return "Anthropic"
	case ai.ProviderCopilot:
		return "Copilot"
	case ai.ProviderOllama:
		return "Ollama"
	case ai.ProviderGemini:
		return "Gemini"
	default:
		return string(pt)
	}
}

func (ob *OmniBar) layoutInput(gtx C, state *editor.EditorState, th *material.Theme) D {
	surfaceAlpha := ob.theme.Surface
	surfaceAlpha.A = 204
	return withFlatBg(gtx, surfaceAlpha, func(gtx C) D {
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
							return DrawTerminalIcon(gtx, iconSize, ob.theme.AIGreen)
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
						return withRoundBg(gtx, ob.theme.AIPurpleBg, 8, func(gtx C) D {
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
	if len(state.AIChatHistory) == 0 {
		return D{}
	}

	// AIチャット表示エリア（ウィンドウの半分まで）
	maxH := gtx.Constraints.Max.Y / 2
	gtx.Constraints.Max.Y = maxH
	gtx.Constraints.Min.Y = 0

	count := len(state.AIChatHistory)

	// ストリーミング中は再描画を要求
	if state.AIStreaming {
		gtx.Execute(op.InvalidateCmd{})
	}

	// 新しいメッセージ追加時 or ストリーミング中に自動スクロール
	if count != ob.aiLastCount || (state.AIStreaming && !ob.aiLastStreaming) {
		ob.aiScrollEnd = true
	}
	ob.aiLastCount = count
	ob.aiLastStreaming = state.AIStreaming

	if ob.aiScrollEnd {
		ob.aiChatList.Position.First = count - 1
		ob.aiChatList.Position.Offset = 1 << 20
		ob.aiChatList.Position.BeforeEnd = false
		ob.aiScrollEnd = false
	}

	return withBg(gtx, func(gtx C, sz image.Point) {
		fillBackground(gtx, ob.theme.SurfaceDark, sz)
		defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
		fillBackground(gtx, ob.theme.Separator, image.Pt(sz.X, 1))
	}, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// チャットメッセージリスト
			layout.Flexed(1, func(gtx C) D {
				return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
					listStyle := material.List(th, &ob.aiChatList)
					listStyle.AnchorStrategy = material.Overlay
					return listStyle.Layout(gtx, count, func(gtx C, i int) D {
						msg := state.AIChatHistory[i]
						var inset layout.Inset
						if i > 0 {
							inset.Top = unit.Dp(16)
						}
						return inset.Layout(gtx, func(gtx C) D {
							if msg.Role == ai.RoleUser {
								return ob.layoutUserMsg(gtx, th, msg)
							}
							return ob.layoutAIMsg(gtx, state, th, msg)
						})
					})
				})
			}),
			// クリアボタン
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Bottom: unit.Dp(6), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
					return layout.E.Layout(gtx, func(gtx C) D {
						return ob.btnAIClear.Layout(gtx, func(gtx C) D {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return DrawTrashIcon(gtx, gtx.Dp(unit.Dp(13)), ob.theme.TextMuted)
								}),
								layout.Rigid(func(gtx C) D {
									return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
										lbl := material.Label(th, unit.Sp(11), "クリア")
										lbl.Color = ob.theme.TextMuted
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
		fillBackground(gtx, ob.theme.SurfaceDark, sz)
		defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
		fillBackground(gtx, ob.theme.Separator, image.Pt(sz.X, 1))
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
						fillRoundRect(gtx, ob.theme.AIGreenBg, image.Pt(sz, sz), sz/2)
						return layout.Stack{}.Layout(gtx,
							layout.Stacked(func(gtx C) D {
								gtx.Constraints.Min = image.Pt(sz, sz)
								gtx.Constraints.Max = image.Pt(sz, sz)
								return layout.Center.Layout(gtx, func(gtx C) D {
									return DrawTerminalIcon(gtx, gtx.Dp(unit.Dp(14)), ob.theme.AIGreen)
								})
							}),
						)
					})
				}),
				// コマンドテキスト
				layout.Flexed(1, func(gtx C) D {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(14), "$ "+entry.Command)
						lbl.Color = ob.theme.AIGreen
						return lbl.Layout(gtx)
					})
				}),
			)
		}),
		// 出力行（実行中 or 完了）
		layout.Rigid(func(gtx C) D {
			if entry.Running {
				return layout.Inset{Left: unit.Dp(40), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					return withRoundBg(gtx, ob.theme.SubtleBg, 6, func(gtx C) D {
						return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(12), "⏳ 実行中...")
							lbl.Color = ob.theme.WarningText
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
				borderColor := ob.theme.AIGreenBg
				statusLabel := "✓"
				statusColor := ob.theme.SuccessText
				if !entry.ExitOK {
					borderColor = ob.theme.ErrorBg
					statusLabel = "✗"
					statusColor = ob.theme.ErrorText
				}
				return layout.Inset{Left: unit.Dp(40), Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
					// ボーダー付き出力ボックス
					return withRoundBg(gtx, borderColor, 7, func(gtx C) D {
						return layout.UniformInset(unit.Dp(1)).Layout(gtx, func(gtx C) D {
							surfBg := ob.theme.Surface
							surfBg.A = 230
							return withRoundBg(gtx, surfBg, 6, func(gtx C) D {
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
											fillBackground(gtx, ob.theme.Separator, image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1))))
											return D{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}
										})
									}),
									// 出力テキスト
									layout.Rigid(func(gtx C) D {
										return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(6), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
											lbl := material.Label(th, unit.Sp(12), output)
											lbl.Color = ob.theme.Text
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

// layoutUserMsg はユーザーメッセージを描画する
func (ob *OmniBar) layoutUserMsg(gtx C, th *material.Theme, msg *editor.AIChatMessage) D {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		// ユーザーアバター
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
				sz := gtx.Dp(unit.Dp(28))
				fillRoundRect(gtx, ob.theme.TextVeryDark, image.Pt(sz, sz), sz/2)
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
				lbl := material.Label(th, unit.Sp(14), msg.Content)
				lbl.Color = ob.theme.Text
				return lbl.Layout(gtx)
			})
		}),
	)
}

// layoutAIMsg はAI応答メッセージを描画する
func (ob *OmniBar) layoutAIMsg(gtx C, state *editor.EditorState, th *material.Theme, msg *editor.AIChatMessage) D {
	// AI応答からdiffブロックを検出
	var diffs []ai.DiffBlock
	if msg.Done || len(msg.Content) > 0 {
		diffs = ai.ExtractDiffBlocks(msg.Content)
	}

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
		// AI応答テキスト + diffプレビュー
		layout.Flexed(1, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// 応答テキスト（diff部分を除去して表示）
				layout.Rigid(func(gtx C) D {
					content := msg.Content
					if content == "" && !msg.Done {
						content = "考え中..."
					}
					// diffブロック以外のテキストを表示
					if len(diffs) > 0 {
						content = removeDiffBlocks(content)
					}
					if content == "" && len(diffs) > 0 {
						return D{} // テキストがdiffのみの場合はスキップ
					}
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(14), content)
						lbl.Color = ob.theme.Text
						return lbl.Layout(gtx)
					})
				}),
				// Diffプレビューブロック
				layout.Rigid(func(gtx C) D {
					if len(diffs) == 0 {
						return D{}
					}
					// Applyボタンの配列を確保
					ob.ensureDiffButtons(len(diffs))

					children := make([]layout.FlexChild, 0, len(diffs))
					for i := range diffs {
						idx := i
						diff := &diffs[i]
						children = append(children, layout.Rigid(func(gtx C) D {
							return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
								return ob.layoutDiffPreview(gtx, state, th, diff, idx)
							})
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
				}),
				// ストリーミングインジケーター
				layout.Rigid(func(gtx C) D {
					if msg.Done {
						return D{}
					}
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						lbl := material.Label(th, unit.Sp(11), "● 生成中...")
						lbl.Color = ob.theme.AIPurple
						return lbl.Layout(gtx)
					})
				}),
			)
		}),
	)
}

// ensureDiffButtons はdiffブロック数に合わせてボタンスライスを拡張する
func (ob *OmniBar) ensureDiffButtons(n int) {
	for len(ob.diffApplyBtns) < n {
		ob.diffApplyBtns = append(ob.diffApplyBtns, widget.Clickable{})
	}
	for len(ob.diffDismiss) < n {
		ob.diffDismiss = append(ob.diffDismiss, widget.Clickable{})
	}
}

// layoutDiffPreview はdiffブロックのプレビューを描画する
func (ob *OmniBar) layoutDiffPreview(gtx C, state *editor.EditorState, th *material.Theme, diff *ai.DiffBlock, idx int) D {
	// クリックイベント処理
	for ob.diffApplyBtns[idx].Clicked(gtx) {
		if !ob.diffApplied[idx] {
			if err := state.AIApplyDiff(diff); err != nil {
				log.Printf("[AI Diff] 適用失敗: %v", err)
			} else {
				ob.diffApplied[idx] = true
			}
		}
	}
	for ob.diffDismiss[idx].Clicked(gtx) {
		ob.diffHidden[idx] = true
	}

	// 非表示
	if ob.diffHidden[idx] {
		return D{}
	}

	applied := ob.diffApplied[idx]

	// ボーダー + 内側背景
	borderColor := ob.theme.BorderLight
	if applied {
		successBorder := ob.theme.SuccessText
		successBorder.A = 102
		borderColor = successBorder
	}
	return withRoundBg(gtx, borderColor, 8, func(gtx C) D {
		return layout.UniformInset(unit.Dp(1)).Layout(gtx, func(gtx C) D {
			return withRoundBg(gtx, ob.theme.Background, 7, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					// ヘッダー
					layout.Rigid(func(gtx C) D {
						return ob.layoutDiffPreviewHeader(gtx, th, diff, idx, applied)
					}),
					// Diff内容
					layout.Rigid(func(gtx C) D {
						return ob.layoutDiffPreviewContent(gtx, th, diff)
					}),
				)
			})
		})
	})
}

// layoutDiffPreviewHeader はdiffプレビューのヘッダーを描画する
func (ob *OmniBar) layoutDiffPreviewHeader(gtx C, th *material.Theme, diff *ai.DiffBlock, idx int, applied bool) D {
	fileName := diff.FilePath
	if fileName == "" {
		fileName = "変更"
	}

	return withFlatBg(gtx, ob.theme.Hover, func(gtx C) D {
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
								lbl := material.Label(th, unit.Sp(12), fileName)
								lbl.Color = ob.theme.TextMuted
								return lbl.Layout(gtx)
							})
						}),
					)
				}),
				layout.Flexed(1, func(gtx C) D { return D{} }),
				// Apply ボタン
				layout.Rigid(func(gtx C) D {
					if applied {
						return withRoundBg(gtx, ob.theme.AIGreenBg, 4, func(gtx C) D {
							return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
								lbl := material.Label(th, unit.Sp(11), "✓ 適用済み")
								lbl.Color = ob.theme.SuccessText
								return lbl.Layout(gtx)
							})
						})
					}
					return ob.diffApplyBtns[idx].Layout(gtx, func(gtx C) D {
						return withRoundBg(gtx, ob.theme.AIPurpleBg, 4, func(gtx C) D {
							return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
								lbl := material.Label(th, unit.Sp(11), "✓ Apply")
								lbl.Color = ob.theme.AIPurple
								return lbl.Layout(gtx)
							})
						})
					})
				}),
				// 閉じるボタン
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return ob.diffDismiss[idx].Layout(gtx, func(gtx C) D {
							return withRoundBg(gtx, ob.theme.Separator, 4, func(gtx C) D {
								return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
									return DrawCloseIcon(gtx, gtx.Dp(unit.Dp(14)), ob.theme.TextMuted)
								})
							})
						})
					})
				}),
			)
		})
	})
}

// layoutDiffPreviewContent はdiffの行内容を描画する
func (ob *OmniBar) layoutDiffPreviewContent(gtx C, th *material.Theme, diff *ai.DiffBlock) D {
	// 全ハンクの行を収集
	type displayLine struct {
		text string
		kind ai.DiffLineType
	}
	var lines []displayLine
	for _, hunk := range diff.Hunks {
		for _, l := range hunk.Lines {
			prefix := " "
			switch l.Type {
			case ai.DiffAdded:
				prefix = "+"
			case ai.DiffDeleted:
				prefix = "-"
			}
			lines = append(lines, displayLine{
				text: prefix + l.Content,
				kind: l.Type,
			})
		}
	}

	if len(lines) == 0 {
		return D{}
	}

	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
		children := make([]layout.FlexChild, len(lines))
		for i := range lines {
			line := lines[i]
			children[i] = layout.Rigid(func(gtx C) D {
				var bgColor color.NRGBA
				var textColor color.NRGBA
				switch line.kind {
				case ai.DiffDeleted:
					bgColor = ob.theme.ErrorBg
					textColor = ob.theme.ErrorText
				case ai.DiffAdded:
					bgColor = ob.theme.AIGreenBg
					textColor = ob.theme.SuccessText
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

// removeDiffBlocks はテキストからdiffコードブロックを除去する
func removeDiffBlocks(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	inDiff := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```diff") {
			inDiff = true
			continue
		}
		if inDiff && trimmed == "```" {
			inDiff = false
			continue
		}
		if !inDiff {
			result = append(result, line)
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
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
		// AIメッセージ送信
		state.AISendMessage(text, ai.ActionChat)
		state.ShowOmniChat = true
		ob.aiScrollEnd = true
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
