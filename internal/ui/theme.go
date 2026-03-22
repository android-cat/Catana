package ui

import (
	"catana/internal/syntax"
	"image/color"
)

// Theme はエディタのカラーテーマを定義する
// ui_design.jsx のダークテーマカラーに準拠
type Theme struct {
	// 背景色
	Background  color.NRGBA // #050505
	Surface     color.NRGBA // #0A0A0A
	SurfaceAlt  color.NRGBA // #111111
	SurfaceDark color.NRGBA // #080808

	// ボーダー
	Border       color.NRGBA // #1A1A1A
	BorderLight  color.NRGBA // #222222
	BorderSubtle color.NRGBA // #111111

	// テキスト
	Text         color.NRGBA // gray-300 (#D1D5DB)
	TextMuted    color.NRGBA // gray-500 (#6B7280)
	TextDark     color.NRGBA // gray-600 (#4B5563)
	TextVeryDark color.NRGBA // gray-700 (#374151)

	// アクセントカラー
	Accent       color.NRGBA // indigo-400 (#818CF8)
	AccentBg     color.NRGBA // indigo-500/10
	AccentBorder color.NRGBA // indigo-500/10

	// ステータスバー
	StatusBar     color.NRGBA // #007ACC
	StatusBarText color.NRGBA // white

	// セレクション
	Selection color.NRGBA // indigo-900/50

	// オムニバー
	OmniBarBg     color.NRGBA // #111/85
	OmniBarBorder color.NRGBA // white/10

	// AIカラー
	AIPurple   color.NRGBA // purple-400 (#C084FC)
	AIPurpleBg color.NRGBA // purple-500/20
	AIGreen    color.NRGBA // green-400 (#4ADE80)
	AIGreenBg  color.NRGBA // green-500/15

	// ホバー
	Hover color.NRGBA // #151515

	// シンタックスカラー
	SynKeyword  color.NRGBA // pink-500 (#EC4899)
	SynType     color.NRGBA // yellow-200 (#FDE68A)
	SynString   color.NRGBA // green-400 (#4ADE80)
	SynComment  color.NRGBA // gray-500 (#6B7280)
	SynNumber   color.NRGBA // orange-300 (#FDBA74)
	SynFunction color.NRGBA // blue-400 (#60A5FA)
	SynOperator color.NRGBA // gray-400 (#9CA3AF)
	SynPunct    color.NRGBA // gray-400 (#9CA3AF)
	SynBuiltin  color.NRGBA // purple-400 (#C084FC)
	SynPlain    color.NRGBA // gray-300 (#D1D5DB)

	// 行番号
	LineNumber       color.NRGBA // gray-600 (#4B5563)
	LineNumberActive color.NRGBA // gray-400 (#9CA3AF)

	// カーソル
	Cursor color.NRGBA // white

	// ゴーストテキスト（AIインライン補完）
	GhostText color.NRGBA // gray-500 alpha

	// 検索マッチ
	SearchMatch       color.NRGBA // 検索結果ハイライト背景
	SearchMatchActive color.NRGBA // 現在の検索結果ハイライト背景

	// Git カラー
	GitAdded    color.NRGBA // green-500 (#22C55E)
	GitModified color.NRGBA // yellow-500 (#EAB308)
	GitDeleted  color.NRGBA // red-500 (#EF4444)

	// ファイルアイコンカラー
	FileIconGo      color.NRGBA // blue-400
	FileIconRust    color.NRGBA // orange-500
	FileIconPy      color.NRGBA // yellow-500
	FileIconTs      color.NRGBA // blue-500
	FileIconJs      color.NRGBA // yellow-400
	FileIconDefault color.NRGBA // gray-400

	// オーバーレイ・ポップアップ
	OverlayBg color.NRGBA // モーダル背景オーバーレイ
	PopupBg   color.NRGBA // ポップアップ・ドロップダウン背景
	Separator color.NRGBA // セパレータ / 薄い境界線
	SubtleBg  color.NRGBA // 微妙な背景ハイライト（ホバーより薄い）
	PanelBg   color.NRGBA // パネル背景（ターミナルヘッダ等）

	// カレント行
	CurrentLineBg color.NRGBA // カレント行ハイライト

	// 診断色
	DiagError   color.NRGBA // LSP エラー
	DiagWarning color.NRGBA // LSP 警告
	DiagInfo    color.NRGBA // LSP 情報

	// 差分表示色
	DiffHunkBg    color.NRGBA // diff ハンクヘッダ背景
	DiffAddedBg   color.NRGBA // diff 追加行背景
	DiffDeletedBg color.NRGBA // diff 削除行背景

	// エラー・成功・警告テキスト
	ErrorText   color.NRGBA
	ErrorBg     color.NRGBA
	SuccessText color.NRGBA
	WarningText color.NRGBA

	// ステータスバーパフォーマンス表示
	PerfGreen  color.NRGBA // FPS
	PerfYellow color.NRGBA // メモリ
	PerfPurple color.NRGBA // LSP

	// ターミナル
	TerminalBg   color.NRGBA     // ターミナル背景
	TerminalANSI [16]color.NRGBA // ANSI 16色パレット
}

// DarkTheme はデフォルトのダークテーマを返す
func DarkTheme() *Theme {
	return &Theme{
		Background:  hexColor(0x050505),
		Surface:     hexColor(0x0A0A0A),
		SurfaceAlt:  hexColor(0x111111),
		SurfaceDark: hexColor(0x080808),

		Border:       hexColor(0x1A1A1A),
		BorderLight:  hexColor(0x222222),
		BorderSubtle: hexColor(0x111111),

		Text:         hexColor(0xD1D5DB),
		TextMuted:    hexColor(0x6B7280),
		TextDark:     hexColor(0x4B5563),
		TextVeryDark: hexColor(0x374151),

		Accent:       hexColor(0x34D399),
		AccentBg:     nrgba(0x34, 0xD3, 0x99, 25),
		AccentBorder: nrgba(0x34, 0xD3, 0x99, 25),

		StatusBar:     hexColor(0x142B1E),
		StatusBarText: hexColor(0xD1FAE5),

		Selection: nrgba(0xA8, 0x55, 0xF7, 40),

		OmniBarBg:     nrgba(0x11, 0x11, 0x11, 216),
		OmniBarBorder: nrgba(0xFF, 0xFF, 0xFF, 25),

		AIPurple:   hexColor(0xC084FC),
		AIPurpleBg: nrgba(0xA8, 0x55, 0xF7, 51),
		AIGreen:    hexColor(0x4ADE80),
		AIGreenBg:  nrgba(0x22, 0xC5, 0x5E, 38),

		Hover: hexColor(0x151515),

		SynKeyword:  hexColor(0xEC4899),
		SynType:     hexColor(0xFDE68A),
		SynString:   hexColor(0x4ADE80),
		SynComment:  hexColor(0x6B7280),
		SynNumber:   hexColor(0xFDBA74),
		SynFunction: hexColor(0x7DD3FC),
		SynOperator: hexColor(0x9CA3AF),
		SynPunct:    hexColor(0x9CA3AF),
		SynBuiltin:  hexColor(0xC084FC),
		SynPlain:    hexColor(0xD1D5DB),

		LineNumber:       hexColor(0x4B5563),
		LineNumberActive: hexColor(0x9CA3AF),

		Cursor: hexColor(0xFFFFFF),

		GhostText: nrgba(0x6B, 0x72, 0x80, 128),

		SearchMatch:       nrgba(0xEA, 0xB3, 0x08, 60),
		SearchMatchActive: nrgba(0xEA, 0xB3, 0x08, 120),

		GitAdded:    hexColor(0x22C55E),
		GitModified: hexColor(0xEAB308),
		GitDeleted:  hexColor(0xEF4444),

		FileIconGo:      hexColor(0x7DD3FC),
		FileIconRust:    hexColor(0xF97316),
		FileIconPy:      hexColor(0xEAB308),
		FileIconTs:      hexColor(0x38BDF8),
		FileIconJs:      hexColor(0xFACC15),
		FileIconDefault: hexColor(0x9CA3AF),

		OverlayBg: nrgba(0x00, 0x00, 0x00, 153),
		PopupBg:   nrgba(0x1A, 0x1A, 0x1A, 240),
		Separator: nrgba(0xFF, 0xFF, 0xFF, 13),
		SubtleBg:  nrgba(0xFF, 0xFF, 0xFF, 8),
		PanelBg:   hexColor(0x171717),

		CurrentLineBg: nrgba(0xFF, 0xFF, 0xFF, 5),

		DiagError:   nrgba(0xE0, 0x6C, 0x75, 200),
		DiagWarning: nrgba(0xE5, 0xC0, 0x7B, 200),
		DiagInfo:    nrgba(0x61, 0xAF, 0xEF, 150),

		DiffHunkBg:    nrgba(0x30, 0x30, 0x60, 40),
		DiffAddedBg:   nrgba(0x22, 0xC5, 0x5E, 20),
		DiffDeletedBg: nrgba(0xEF, 0x44, 0x44, 20),

		ErrorText:   hexColor(0xF87171),
		ErrorBg:     nrgba(0xEF, 0x44, 0x44, 25),
		SuccessText: hexColor(0x4ADE80),
		WarningText: hexColor(0xFBBF24),

		PerfGreen:  nrgba(0xBB, 0xF7, 0xD0, 255),
		PerfYellow: nrgba(0xFE, 0xF0, 0x8A, 255),
		PerfPurple: nrgba(0xE9, 0xD5, 0xFF, 255),

		TerminalBg: hexColor(0x0F0F0F),
		TerminalANSI: [16]color.NRGBA{
			nrgba(0x00, 0x00, 0x00, 255), // 0: black
			nrgba(0xCD, 0x3E, 0x45, 255), // 1: red
			nrgba(0x34, 0xD3, 0x99, 255), // 2: green
			nrgba(0xE5, 0xC0, 0x7B, 255), // 3: yellow
			nrgba(0x61, 0xAF, 0xEF, 255), // 4: blue
			nrgba(0xC6, 0x78, 0xDD, 255), // 5: magenta
			nrgba(0x56, 0xB6, 0xC2, 255), // 6: cyan
			nrgba(0xD1, 0xD5, 0xDB, 255), // 7: white
			nrgba(0x5C, 0x63, 0x70, 255), // 8: bright black
			nrgba(0xE0, 0x6C, 0x75, 255), // 9: bright red
			nrgba(0x98, 0xC3, 0x79, 255), // 10: bright green
			nrgba(0xE5, 0xC0, 0x7B, 255), // 11: bright yellow
			nrgba(0x61, 0xAF, 0xEF, 255), // 12: bright blue
			nrgba(0xC6, 0x78, 0xDD, 255), // 13: bright magenta
			nrgba(0x56, 0xB6, 0xC2, 255), // 14: bright cyan
			nrgba(0xFF, 0xFF, 0xFF, 255), // 15: bright white
		},
	}
}

// LightTheme はライトテーマを返す
func LightTheme() *Theme {
	return &Theme{
		Background:  hexColor(0xFFFFFF),
		Surface:     hexColor(0xFFFFFF),
		SurfaceAlt:  hexColor(0xF5F5F5),
		SurfaceDark: hexColor(0xEFEFEF),

		Border:       hexColor(0xD0D0D0),
		BorderLight:  hexColor(0xC0C0C0),
		BorderSubtle: hexColor(0xE0E0E0),

		Text:         hexColor(0x1F2937),
		TextMuted:    hexColor(0x6B7280),
		TextDark:     hexColor(0x9CA3AF),
		TextVeryDark: hexColor(0xD1D5DB),

		Accent:       hexColor(0x059669),
		AccentBg:     nrgba(0x05, 0x96, 0x69, 25),
		AccentBorder: nrgba(0x05, 0x96, 0x69, 25),

		StatusBar:     hexColor(0x1D7A4A),
		StatusBarText: hexColor(0xF0FDF4),

		Selection: nrgba(0x05, 0x96, 0x69, 50),

		OmniBarBg:     nrgba(0xFF, 0xFF, 0xFF, 230),
		OmniBarBorder: nrgba(0x00, 0x00, 0x00, 25),

		AIPurple:   hexColor(0x7C3AED),
		AIPurpleBg: nrgba(0x7C, 0x3A, 0xED, 30),
		AIGreen:    hexColor(0x059669),
		AIGreenBg:  nrgba(0x05, 0x96, 0x69, 30),

		Hover: hexColor(0xE8E8E8),

		SynKeyword:  hexColor(0xBE185D),
		SynType:     hexColor(0x92400E),
		SynString:   hexColor(0x15803D),
		SynComment:  hexColor(0x6B7280),
		SynNumber:   hexColor(0xC2410C),
		SynFunction: hexColor(0x1D4ED8),
		SynOperator: hexColor(0x374151),
		SynPunct:    hexColor(0x374151),
		SynBuiltin:  hexColor(0x6D28D9),
		SynPlain:    hexColor(0x1F2937),

		LineNumber:       hexColor(0x9CA3AF),
		LineNumberActive: hexColor(0x374151),

		Cursor: hexColor(0x1F2937),

		GhostText: nrgba(0x9C, 0xA3, 0xAF, 128),

		SearchMatch:       nrgba(0xFB, 0xBF, 0x24, 80),
		SearchMatchActive: nrgba(0xFB, 0xBF, 0x24, 160),

		GitAdded:    hexColor(0x16A34A),
		GitModified: hexColor(0xCA8A04),
		GitDeleted:  hexColor(0xDC2626),

		FileIconGo:      hexColor(0x1D4ED8),
		FileIconRust:    hexColor(0xC2410C),
		FileIconPy:      hexColor(0xCA8A04),
		FileIconTs:      hexColor(0x0369A1),
		FileIconJs:      hexColor(0xB45309),
		FileIconDefault: hexColor(0x6B7280),

		OverlayBg: nrgba(0x00, 0x00, 0x00, 0),
		PopupBg:   nrgba(0xFF, 0xFF, 0xFF, 245),
		Separator: nrgba(0x00, 0x00, 0x00, 13),
		SubtleBg:  nrgba(0x00, 0x00, 0x00, 8),
		PanelBg:   hexColor(0xE8E8E8),

		CurrentLineBg: nrgba(0x00, 0x00, 0x00, 8),

		DiagError:   nrgba(0xDC, 0x26, 0x26, 200),
		DiagWarning: nrgba(0xCA, 0x8A, 0x04, 200),
		DiagInfo:    nrgba(0x1D, 0x4E, 0xD8, 150),

		DiffHunkBg:    nrgba(0x1D, 0x4E, 0xD8, 20),
		DiffAddedBg:   nrgba(0x16, 0xA3, 0x4A, 20),
		DiffDeletedBg: nrgba(0xDC, 0x26, 0x26, 20),

		ErrorText:   hexColor(0xDC2626),
		ErrorBg:     nrgba(0xDC, 0x26, 0x26, 25),
		SuccessText: hexColor(0x059669),
		WarningText: hexColor(0xCA8A04),

		PerfGreen:  hexColor(0x059669),
		PerfYellow: hexColor(0xCA8A04),
		PerfPurple: hexColor(0x6D28D9),

		TerminalBg: hexColor(0xF0F0F0),
		TerminalANSI: [16]color.NRGBA{
			nrgba(0x00, 0x00, 0x00, 255), // 0: black
			nrgba(0xDC, 0x26, 0x26, 255), // 1: red
			nrgba(0x16, 0xA3, 0x4A, 255), // 2: green
			nrgba(0xCA, 0x8A, 0x04, 255), // 3: yellow
			nrgba(0x1D, 0x4E, 0xD8, 255), // 4: blue
			nrgba(0x6D, 0x28, 0xD9, 255), // 5: magenta
			nrgba(0x06, 0x91, 0xB7, 255), // 6: cyan
			nrgba(0x1F, 0x29, 0x37, 255), // 7: white
			nrgba(0x6B, 0x72, 0x80, 255), // 8: bright black
			nrgba(0xEF, 0x44, 0x44, 255), // 9: bright red
			nrgba(0x22, 0xC5, 0x5E, 255), // 10: bright green
			nrgba(0xEA, 0xB3, 0x08, 255), // 11: bright yellow
			nrgba(0x38, 0xBD, 0xF8, 255), // 12: bright blue
			nrgba(0x8B, 0x5C, 0xF6, 255), // 13: bright magenta
			nrgba(0x06, 0xB6, 0xD4, 255), // 14: bright cyan
			nrgba(0xF9, 0xFA, 0xFB, 255), // 15: bright white
		},
	}
}

// TokenColor はトークンタイプに対応する色を返す
func (t *Theme) TokenColor(tokenType syntax.TokenType) color.NRGBA {
	switch tokenType {
	case syntax.TokenKeyword:
		return t.SynKeyword
	case syntax.TokenType_:
		return t.SynType
	case syntax.TokenString:
		return t.SynString
	case syntax.TokenComment:
		return t.SynComment
	case syntax.TokenNumber:
		return t.SynNumber
	case syntax.TokenFunction:
		return t.SynFunction
	case syntax.TokenOperator:
		return t.SynOperator
	case syntax.TokenPunctuation:
		return t.SynPunct
	case syntax.TokenBuiltin:
		return t.SynBuiltin
	default:
		return t.SynPlain
	}
}

// FileIconColor はファイル拡張子に対応するアイコン色を返す
func (t *Theme) FileIconColor(filename string) color.NRGBA {
	if len(filename) < 2 {
		return t.FileIconDefault
	}
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			ext := filename[i:]
			switch ext {
			case ".go":
				return t.FileIconGo
			case ".rs":
				return t.FileIconRust
			case ".py":
				return t.FileIconPy
			case ".ts", ".tsx":
				return t.FileIconTs
			case ".js", ".jsx":
				return t.FileIconJs
			}
			break
		}
	}
	return t.FileIconDefault
}

// hexColor は24bitカラーコードからNRGBAに変換する
func hexColor(hex int) color.NRGBA {
	return color.NRGBA{
		R: uint8((hex >> 16) & 0xFF),
		G: uint8((hex >> 8) & 0xFF),
		B: uint8(hex & 0xFF),
		A: 255,
	}
}

// nrgba はRGBA個別値からNRGBAを生成する
func nrgba(r, g, b, a uint8) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: a}
}
