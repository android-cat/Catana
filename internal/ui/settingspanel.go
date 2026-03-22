package ui

import (
	"catana/internal/config"
	"catana/internal/editor"
	"fmt"
	"image"
	"image/color"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// ─── 設定パネル ───

// SettingsPanel はサイドバーの設定パネル
type SettingsPanel struct {
	theme *Theme
	list  widget.List

	// 一般設定
	edFontSize  widget.Editor
	edTabSize   widget.Editor
	edShell     widget.Editor
	btnWordWrap widget.Clickable
	btnMinimap  widget.Clickable
	btnLineNums widget.Clickable
	btnAutoSave widget.Clickable

	// AIプロバイダ設定
	edOpenAIKey       widget.Editor
	edOpenAIModel     widget.Editor
	edAnthropicKey    widget.Editor
	edAnthropicModel  widget.Editor
	edCopilotToken    widget.Editor
	edCopilotEndpoint widget.Editor
	edOllamaModel     widget.Editor
	edOllamaEndpoint  widget.Editor
	edGeminiKey       widget.Editor
	edGeminiModel     widget.Editor
	btnSave           widget.Clickable

	// 初期化済みフラグ
	initialized bool
	// 保存結果メッセージ
	saveMsg   string
	saveMsgOK bool
}

// NewSettingsPanel は新しいSettingsPanelを作成する
func NewSettingsPanel(theme *Theme) *SettingsPanel {
	sp := &SettingsPanel{theme: theme}
	sp.list.Axis = layout.Vertical
	sp.edFontSize.SingleLine = true
	sp.edTabSize.SingleLine = true
	sp.edShell.SingleLine = true
	sp.edOpenAIKey.SingleLine = true
	sp.edOpenAIModel.SingleLine = true
	sp.edAnthropicKey.SingleLine = true
	sp.edAnthropicModel.SingleLine = true
	sp.edCopilotToken.SingleLine = true
	sp.edCopilotEndpoint.SingleLine = true
	sp.edOllamaModel.SingleLine = true
	sp.edOllamaEndpoint.SingleLine = true
	sp.edGeminiKey.SingleLine = true
	sp.edGeminiModel.SingleLine = true
	// パスワード風にマスクしない（APIキーは見える方が便利）
	return sp
}

// initFromConfig は設定値をエディタに反映する（初回のみ）
func (sp *SettingsPanel) initFromConfig(cfg *config.Config) {
	if sp.initialized || cfg == nil {
		return
	}
	sp.initialized = true

	sp.edFontSize.SetText(strconv.Itoa(cfg.General.FontSize))
	sp.edTabSize.SetText(strconv.Itoa(cfg.General.TabSize))
	sp.edShell.SetText(cfg.General.Shell)

	openai := cfg.GetAIProvider("openai")
	sp.edOpenAIKey.SetText(openai.APIKey)
	sp.edOpenAIModel.SetText(openai.Model)

	anthropic := cfg.GetAIProvider("anthropic")
	sp.edAnthropicKey.SetText(anthropic.APIKey)
	sp.edAnthropicModel.SetText(anthropic.Model)

	copilot := cfg.GetAIProvider("copilot")
	sp.edCopilotToken.SetText(copilot.APIKey)
	sp.edCopilotEndpoint.SetText(copilot.Endpoint)

	ollama := cfg.GetAIProvider("ollama")
	sp.edOllamaModel.SetText(ollama.Model)
	sp.edOllamaEndpoint.SetText(ollama.Endpoint)

	gemini := cfg.GetAIProvider("gemini")
	sp.edGeminiKey.SetText(gemini.APIKey)
	sp.edGeminiModel.SetText(gemini.Model)
}

// Layout は設定パネルを描画する
func (sp *SettingsPanel) Layout(gtx C, state *editor.EditorState, th *material.Theme) D {
	sp.initFromConfig(state.Config)

	// 保存ボタンクリック
	for sp.btnSave.Clicked(gtx) {
		sp.applyAndSave(state)
	}
	// トグルクリック
	for sp.btnWordWrap.Clicked(gtx) {
		if state.Config != nil {
			state.Config.General.WordWrap = !state.Config.General.WordWrap
		}
	}
	for sp.btnMinimap.Clicked(gtx) {
		if state.Config != nil {
			state.Config.General.Minimap = !state.Config.General.Minimap
		}
	}
	for sp.btnLineNums.Clicked(gtx) {
		if state.Config != nil {
			state.Config.General.LineNumbers = !state.Config.General.LineNumbers
		}
	}
	for sp.btnAutoSave.Clicked(gtx) {
		if state.Config != nil {
			state.Config.General.AutoSave = !state.Config.General.AutoSave
		}
	}

	return material.List(th, &sp.list).Layout(gtx, 1, func(gtx C, _ int) D {
		return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// ── 一般設定セクション ──
				sp.sectionHeader(th, "一般設定"),
				sp.settingInput(gtx, th, "フォントサイズ (sp)", &sp.edFontSize),
				sp.settingInput(gtx, th, "タブ幅", &sp.edTabSize),
				sp.settingInput(gtx, th, "シェル", &sp.edShell),
				sp.settingToggle(gtx, th, "折返し表示", &sp.btnWordWrap, state.Config != nil && state.Config.General.WordWrap),
				sp.settingToggle(gtx, th, "ミニマップ", &sp.btnMinimap, state.Config != nil && state.Config.General.Minimap),
				sp.settingToggle(gtx, th, "行番号", &sp.btnLineNums, state.Config != nil && state.Config.General.LineNumbers),
				sp.settingToggle(gtx, th, "自動保存", &sp.btnAutoSave, state.Config != nil && state.Config.General.AutoSave),

				// ── AIプロバイダセクション ──
				sp.sectionHeader(th, "OpenAI"),
				sp.settingInput(gtx, th, "API Key", &sp.edOpenAIKey),
				sp.settingInput(gtx, th, "モデル", &sp.edOpenAIModel),

				sp.sectionHeader(th, "Anthropic"),
				sp.settingInput(gtx, th, "API Key", &sp.edAnthropicKey),
				sp.settingInput(gtx, th, "モデル", &sp.edAnthropicModel),

				sp.sectionHeader(th, "GitHub Copilot"),
				sp.settingInput(gtx, th, "Token", &sp.edCopilotToken),
				sp.settingInput(gtx, th, "Endpoint", &sp.edCopilotEndpoint),

				sp.sectionHeader(th, "Ollama"),
				sp.settingInput(gtx, th, "モデル", &sp.edOllamaModel),
				sp.settingInput(gtx, th, "Endpoint", &sp.edOllamaEndpoint),

				sp.sectionHeader(th, "Google Gemini"),
				sp.settingInput(gtx, th, "API Key", &sp.edGeminiKey),
				sp.settingInput(gtx, th, "モデル", &sp.edGeminiModel),

				// ── 保存ボタン ──
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return sp.btnSave.Layout(gtx, func(gtx C) D {
									bgColor := sp.theme.Accent
									if sp.btnSave.Hovered() {
										bgColor = brighten(bgColor, 20)
									}
									return withRoundBg(gtx, bgColor, 6, func(gtx C) D {
										return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
											lbl := material.Label(th, unit.Sp(13), "保存して適用")
											lbl.Color = sp.theme.Background
											return lbl.Layout(gtx)
										})
									})
								})
							}),
							// 保存メッセージ
							layout.Rigid(func(gtx C) D {
								if sp.saveMsg == "" {
									return D{}
								}
								return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
									col := sp.theme.ErrorText
									if sp.saveMsgOK {
										col = sp.theme.SuccessText
									}
									lbl := material.Label(th, unit.Sp(11), sp.saveMsg)
									lbl.Color = col
									return lbl.Layout(gtx)
								})
							}),
						)
					})
				}),
			)
		})
	})
}

// ─── 保存処理 ───

func (sp *SettingsPanel) applyAndSave(state *editor.EditorState) {
	cfg := state.Config
	if cfg == nil {
		sp.saveMsg = "設定が初期化されていません"
		sp.saveMsgOK = false
		return
	}

	// 一般設定の反映
	if v, err := strconv.Atoi(sp.edFontSize.Text()); err == nil && v >= 8 && v <= 48 {
		cfg.General.FontSize = v
	}
	if v, err := strconv.Atoi(sp.edTabSize.Text()); err == nil && v >= 1 && v <= 16 {
		cfg.General.TabSize = v
	}
	if s := sp.edShell.Text(); s != "" {
		cfg.General.Shell = s
	}

	// AIプロバイダ設定
	cfg.SetAIProvider("openai", config.AIProviderConfig{
		APIKey: sp.edOpenAIKey.Text(),
		Model:  sp.edOpenAIModel.Text(),
	})
	cfg.SetAIProvider("anthropic", config.AIProviderConfig{
		APIKey: sp.edAnthropicKey.Text(),
		Model:  sp.edAnthropicModel.Text(),
	})
	cfg.SetAIProvider("copilot", config.AIProviderConfig{
		APIKey:   sp.edCopilotToken.Text(),
		Endpoint: sp.edCopilotEndpoint.Text(),
	})
	cfg.SetAIProvider("ollama", config.AIProviderConfig{
		Model:    sp.edOllamaModel.Text(),
		Endpoint: sp.edOllamaEndpoint.Text(),
	})
	cfg.SetAIProvider("gemini", config.AIProviderConfig{
		APIKey: sp.edGeminiKey.Text(),
		Model:  sp.edGeminiModel.Text(),
	})

	// ファイルに保存
	if err := cfg.Save(); err != nil {
		sp.saveMsg = fmt.Sprintf("保存失敗: %v", err)
		sp.saveMsgOK = false
		return
	}

	// AIプロバイダを再初期化するシグナル
	state.ConfigChanged = true

	sp.saveMsg = "✓ 設定を保存しました"
	sp.saveMsgOK = true
}

// ─── UIヘルパー ───

func (sp *SettingsPanel) sectionHeader(th *material.Theme, title string) layout.FlexChild {
	return layout.Rigid(func(gtx C) D {
		return layout.Inset{Top: unit.Dp(14), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lbl := material.Label(th, unit.Sp(12), title)
					lbl.Color = sp.theme.Text
					lbl.Font.Weight = 600
					return lbl.Layout(gtx)
				}),
				// 下線
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
						sz := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))
						fillBackground(gtx, sp.theme.Border, sz)
						return D{Size: sz}
					})
				}),
			)
		})
	})
}

func (sp *SettingsPanel) settingInput(gtx C, th *material.Theme, label string, ed *widget.Editor) layout.FlexChild {
	return layout.Rigid(func(gtx C) D {
		return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// ラベル
				layout.Rigid(func(gtx C) D {
					lbl := material.Label(th, unit.Sp(11), label)
					lbl.Color = sp.theme.TextMuted
					return lbl.Layout(gtx)
				}),
				// 入力フィールド
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
						h := gtx.Dp(unit.Dp(28))
						return withBg(gtx, func(gtx C, sz image.Point) {
							fillRoundRect(gtx, sp.theme.Background, sz, 4)
							// ボーダー
							drawBorderRect(gtx, sp.theme.BorderLight, sz, 4)
						}, func(gtx C) D {
							gtx.Constraints.Min.Y = h
							gtx.Constraints.Max.Y = h
							return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
								edStyle := material.Editor(th, ed, "")
								edStyle.Color = sp.theme.Text
								edStyle.HintColor = sp.theme.TextDark
								edStyle.TextSize = unit.Sp(12)
								return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceAround}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										return edStyle.Layout(gtx)
									}),
								)
							})
						})
					})
				}),
			)
		})
	})
}

func (sp *SettingsPanel) settingToggle(gtx C, th *material.Theme, label string, btn *widget.Clickable, value bool) layout.FlexChild {
	return layout.Rigid(func(gtx C) D {
		return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			return btn.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// トグルインジケータ
					layout.Rigid(func(gtx C) D {
						sz := gtx.Dp(unit.Dp(16))
						bgCol := sp.theme.Border
						if value {
							bgCol = sp.theme.Accent
						}
						fillRoundRect(gtx, bgCol, image.Pt(sz, sz), 3)
						if value {
							// チェックマーク
							inset := gtx.Dp(unit.Dp(3))
							defer op.Offset(image.Pt(inset, inset)).Push(gtx.Ops).Pop()
							inner := sz - inset*2
							fillRoundRect(gtx, sp.theme.Background, image.Pt(inner, inner), 1)
						}
						return D{Size: image.Pt(sz, sz)}
					}),
					// ラベル
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
							lbl := material.Label(th, unit.Sp(12), label)
							lbl.Color = sp.theme.Text
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	})
}

// drawBorderRect は角丸のボーダーを描画する
func drawBorderRect(gtx C, col color.NRGBA, sz image.Point, radius int) {
	// 上辺
	fillBackground(gtx, col, image.Pt(sz.X, 1))
	// 下辺
	defer op.Offset(image.Pt(0, sz.Y-1)).Push(gtx.Ops).Pop()
	fillBackground(gtx, col, image.Pt(sz.X, 1))
}

// brighten は色を明るくするヘルパー
func brighten(c color.NRGBA, amount uint8) color.NRGBA {
	add := func(v, a uint8) uint8 {
		sum := int(v) + int(a)
		if sum > 255 {
			return 255
		}
		return uint8(sum)
	}
	return color.NRGBA{R: add(c.R, amount), G: add(c.G, amount), B: add(c.B, amount), A: c.A}
}
