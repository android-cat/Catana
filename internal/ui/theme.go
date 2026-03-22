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
	}
}

// LightTheme はライトテーマを返す
func LightTheme() *Theme {
	return &Theme{
		Background:  hexColor(0xF5F5F5),
		Surface:     hexColor(0xFFFFFF),
		SurfaceAlt:  hexColor(0xEFEFEF),
		SurfaceDark: hexColor(0xE8E8E8),

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
