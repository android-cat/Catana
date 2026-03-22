package ui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// アイコン描画関数群
// ui_design.jsx の Lucide React アイコンに相当するベクターアイコン

type C = layout.Context
type D = layout.Dimensions

// DrawFilesIcon はファイル一覧アイコンを描画する
func DrawFilesIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// 後ろのファイル
	drawLine(gtx, f32.Pt(s*0.35, s*0.1), f32.Pt(s*0.85, s*0.1), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.85, s*0.1), f32.Pt(s*0.85, s*0.65), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.35, s*0.1), f32.Pt(s*0.35, s*0.65), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.35, s*0.65), f32.Pt(s*0.85, s*0.65), col, 1.5)

	// 前のファイル
	drawLine(gtx, f32.Pt(s*0.15, s*0.3), f32.Pt(s*0.65, s*0.3), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.65, s*0.3), f32.Pt(s*0.65, s*0.9), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.15, s*0.3), f32.Pt(s*0.15, s*0.9), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.15, s*0.9), f32.Pt(s*0.65, s*0.9), col, 1.5)

	return D{Size: image.Pt(size, size)}
}

// DrawSearchIcon は検索アイコンを描画する
func DrawSearchIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// 円（近似）
	cx, cy, r := s*0.42, s*0.42, s*0.25
	steps := 16
	for i := 0; i < steps; i++ {
		a1 := float64(i) * 2 * math.Pi / float64(steps)
		a2 := float64(i+1) * 2 * math.Pi / float64(steps)
		p1 := f32.Pt(cx+r*float32(math.Cos(a1)), cy+r*float32(math.Sin(a1)))
		p2 := f32.Pt(cx+r*float32(math.Cos(a2)), cy+r*float32(math.Sin(a2)))
		drawLine(gtx, p1, p2, col, 1.5)
	}

	// 持ち手
	drawLine(gtx, f32.Pt(s*0.6, s*0.6), f32.Pt(s*0.85, s*0.85), col, 1.5)

	return D{Size: image.Pt(size, size)}
}

// DrawGitBranchIcon はGitブランチアイコンを描画する
func DrawGitBranchIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// メインライン
	drawLine(gtx, f32.Pt(s*0.35, s*0.2), f32.Pt(s*0.35, s*0.8), col, 1.5)
	// ブランチライン
	drawLine(gtx, f32.Pt(s*0.35, s*0.5), f32.Pt(s*0.65, s*0.3), col, 1.5)

	// ノード（円の近似）
	drawCircle(gtx, f32.Pt(s*0.35, s*0.2), s*0.06, col)
	drawCircle(gtx, f32.Pt(s*0.35, s*0.8), s*0.06, col)
	drawCircle(gtx, f32.Pt(s*0.65, s*0.3), s*0.06, col)

	return D{Size: image.Pt(size, size)}
}

// DrawBlocksIcon は拡張機能アイコンを描画する
func DrawBlocksIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// 4つの四角ブロック
	bsize := s * 0.3
	gap := s * 0.08

	drawRect(gtx, f32.Pt(s*0.2, s*0.15), bsize, bsize, col)
	drawRect(gtx, f32.Pt(s*0.2+bsize+gap, s*0.15), bsize, bsize, col)
	drawRect(gtx, f32.Pt(s*0.2, s*0.15+bsize+gap), bsize, bsize, col)
	drawRect(gtx, f32.Pt(s*0.2+bsize+gap, s*0.15+bsize+gap), bsize, bsize, col)

	return D{Size: image.Pt(size, size)}
}

// DrawSettingsIcon は設定アイコン（歯車）を描画する
func DrawSettingsIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	cx, cy := s*0.5, s*0.5
	// 外側ギア
	outerR := s * 0.38
	innerR := s * 0.28
	steps := 8
	for i := 0; i < steps; i++ {
		a1 := float64(i) * 2 * math.Pi / float64(steps)
		a2 := float64(i)*2*math.Pi/float64(steps) + math.Pi/float64(steps)
		p1 := f32.Pt(cx+outerR*float32(math.Cos(a1)), cy+outerR*float32(math.Sin(a1)))
		p2 := f32.Pt(cx+innerR*float32(math.Cos(a2)), cy+innerR*float32(math.Sin(a2)))
		drawLine(gtx, p1, p2, col, 1.5)
	}
	// 中心の円
	drawCircle(gtx, f32.Pt(cx, cy), s*0.1, col)

	return D{Size: image.Pt(size, size)}
}

// DrawSunIcon は太陽アイコンを描画する
func DrawSunIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	cx, cy := s*0.5, s*0.5
	// 中心の円
	drawCircle(gtx, f32.Pt(cx, cy), s*0.12, col)
	// 光線
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		inner := s * 0.2
		outer := s * 0.35
		p1 := f32.Pt(cx+inner*float32(math.Cos(a)), cy+inner*float32(math.Sin(a)))
		p2 := f32.Pt(cx+outer*float32(math.Cos(a)), cy+outer*float32(math.Sin(a)))
		drawLine(gtx, p1, p2, col, 1.5)
	}

	return D{Size: image.Pt(size, size)}
}

// DrawMoonIcon は月アイコンを描画する（ライトモード時に表示、クリックでダークへ）
func DrawMoonIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// 三日月形：外側の円弧（右向き凸）
	cx, cy, r := s*0.5, s*0.5, s*0.3
	steps := 24
	// 外側の弧：-120°〜+120°（右半分＋少し）
	for i := 0; i < steps; i++ {
		a1 := (-120.0 + float64(i)*240.0/float64(steps)) * math.Pi / 180.0
		a2 := (-120.0 + float64(i+1)*240.0/float64(steps)) * math.Pi / 180.0
		p1 := f32.Pt(cx+r*float32(math.Cos(a1)), cy+r*float32(math.Sin(a1)))
		p2 := f32.Pt(cx+r*float32(math.Cos(a2)), cy+r*float32(math.Sin(a2)))
		drawLine(gtx, p1, p2, col, 1.5)
	}
	// 内側の弧（中心を右にずらして三日月の空き部分を形成）
	icx := cx + s*0.1
	ir := r * 0.82
	for i := 0; i < steps; i++ {
		a1 := (-120.0 + float64(i)*240.0/float64(steps)) * math.Pi / 180.0
		a2 := (-120.0 + float64(i+1)*240.0/float64(steps)) * math.Pi / 180.0
		p1 := f32.Pt(icx+ir*float32(math.Cos(a1)), cy+ir*float32(math.Sin(a1)))
		p2 := f32.Pt(icx+ir*float32(math.Cos(a2)), cy+ir*float32(math.Sin(a2)))
		drawLine(gtx, p1, p2, col, 1.5)
	}

	return D{Size: image.Pt(size, size)}
}

// DrawChevronRight は右向き矢印を描画する
func DrawChevronRight(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()
	drawLine(gtx, f32.Pt(s*0.35, s*0.2), f32.Pt(s*0.65, s*0.5), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.65, s*0.5), f32.Pt(s*0.35, s*0.8), col, 1.5)
	return D{Size: image.Pt(size, size)}
}

// DrawChevronDown は下向き矢印を描画する
func DrawChevronDown(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()
	drawLine(gtx, f32.Pt(s*0.2, s*0.35), f32.Pt(s*0.5, s*0.65), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.5, s*0.65), f32.Pt(s*0.8, s*0.35), col, 1.5)
	return D{Size: image.Pt(size, size)}
}

// DrawFileIcon はファイルアイコンを描画する
func DrawFileIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// ファイル外形
	drawLine(gtx, f32.Pt(s*0.25, s*0.1), f32.Pt(s*0.55, s*0.1), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.55, s*0.1), f32.Pt(s*0.75, s*0.3), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.75, s*0.3), f32.Pt(s*0.75, s*0.9), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.75, s*0.9), f32.Pt(s*0.25, s*0.9), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.25, s*0.9), f32.Pt(s*0.25, s*0.1), col, 1.2)
	// 角の折り返し
	drawLine(gtx, f32.Pt(s*0.55, s*0.1), f32.Pt(s*0.55, s*0.3), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.55, s*0.3), f32.Pt(s*0.75, s*0.3), col, 1.2)

	return D{Size: image.Pt(size, size)}
}

// DrawFolderIcon はフォルダアイコンを描画する
func DrawFolderIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	drawLine(gtx, f32.Pt(s*0.1, s*0.25), f32.Pt(s*0.4, s*0.25), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.4, s*0.25), f32.Pt(s*0.5, s*0.35), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.5, s*0.35), f32.Pt(s*0.9, s*0.35), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.9, s*0.35), f32.Pt(s*0.9, s*0.8), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.9, s*0.8), f32.Pt(s*0.1, s*0.8), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.1, s*0.8), f32.Pt(s*0.1, s*0.25), col, 1.2)

	return D{Size: image.Pt(size, size)}
}

// DrawCloseIcon は閉じるアイコン（X）を描画する
func DrawCloseIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()
	drawLine(gtx, f32.Pt(s*0.25, s*0.25), f32.Pt(s*0.75, s*0.75), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.75, s*0.25), f32.Pt(s*0.25, s*0.75), col, 1.5)
	return D{Size: image.Pt(size, size)}
}

// DrawSparklesIcon はAIスパークルアイコンを描画する
func DrawSparklesIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// 大きな星
	drawStar(gtx, f32.Pt(s*0.45, s*0.45), s*0.3, col)
	// 小さな星
	drawStar(gtx, f32.Pt(s*0.8, s*0.2), s*0.12, col)

	return D{Size: image.Pt(size, size)}
}

// DrawCommandIcon はコマンドアイコンを描画する
func DrawCommandIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// ⌘ 記号の簡略化
	drawLine(gtx, f32.Pt(s*0.3, s*0.3), f32.Pt(s*0.7, s*0.3), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.3, s*0.7), f32.Pt(s*0.7, s*0.7), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.3, s*0.3), f32.Pt(s*0.3, s*0.7), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.7, s*0.3), f32.Pt(s*0.7, s*0.7), col, 1.5)

	// 角の丸み
	drawCircle(gtx, f32.Pt(s*0.3, s*0.3), s*0.06, col)
	drawCircle(gtx, f32.Pt(s*0.7, s*0.3), s*0.06, col)
	drawCircle(gtx, f32.Pt(s*0.3, s*0.7), s*0.06, col)
	drawCircle(gtx, f32.Pt(s*0.7, s*0.7), s*0.06, col)

	return D{Size: image.Pt(size, size)}
}

// DrawTerminalIcon はターミナルアイコンを描画する
func DrawTerminalIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// > プロンプト
	drawLine(gtx, f32.Pt(s*0.2, s*0.3), f32.Pt(s*0.45, s*0.5), col, 1.8)
	drawLine(gtx, f32.Pt(s*0.45, s*0.5), f32.Pt(s*0.2, s*0.7), col, 1.8)
	// _ アンダーライン
	drawLine(gtx, f32.Pt(s*0.5, s*0.7), f32.Pt(s*0.8, s*0.7), col, 1.8)

	return D{Size: image.Pt(size, size)}
}

// DrawSidebarCloseIcon はサイドバー閉じるアイコンを描画する
func DrawSidebarCloseIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// 左パネル
	drawRect(gtx, f32.Pt(s*0.15, s*0.2), s*0.3, s*0.6, col)
	// 右エリア
	drawLine(gtx, f32.Pt(s*0.55, s*0.2), f32.Pt(s*0.85, s*0.2), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.85, s*0.2), f32.Pt(s*0.85, s*0.8), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.85, s*0.8), f32.Pt(s*0.55, s*0.8), col, 1.2)

	return D{Size: image.Pt(size, size)}
}

// DrawSidebarOpenIcon はサイドバー開くアイコンを描画する
func DrawSidebarOpenIcon(gtx C, size int, col color.NRGBA) D {
	return DrawSidebarCloseIcon(gtx, size, col)
}

// DrawEnterIcon はCornerDownLeft（送信）アイコンを描画する
func DrawEnterIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	// 上から下への縦線
	drawLine(gtx, f32.Pt(s*0.7, s*0.2), f32.Pt(s*0.7, s*0.65), col, s*0.08)
	// 右から左への横線
	drawLine(gtx, f32.Pt(s*0.7, s*0.65), f32.Pt(s*0.25, s*0.65), col, s*0.08)
	// 矢印の先端
	drawLine(gtx, f32.Pt(s*0.25, s*0.65), f32.Pt(s*0.4, s*0.5), col, s*0.08)
	drawLine(gtx, f32.Pt(s*0.25, s*0.65), f32.Pt(s*0.4, s*0.8), col, s*0.08)
	return D{Size: image.Pt(size, size)}
}

// DrawActivityIcon はアクティビティ（波形）アイコンを描画する
func DrawActivityIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	mid := s * 0.5
	// 簡易的な波形（3つの山谷）
	drawLine(gtx, f32.Pt(s*0.1, mid), f32.Pt(s*0.3, s*0.2), col, s*0.08)
	drawLine(gtx, f32.Pt(s*0.3, s*0.2), f32.Pt(s*0.5, s*0.8), col, s*0.08)
	drawLine(gtx, f32.Pt(s*0.5, s*0.8), f32.Pt(s*0.7, s*0.3), col, s*0.08)
	drawLine(gtx, f32.Pt(s*0.7, s*0.3), f32.Pt(s*0.9, mid), col, s*0.08)
	return D{Size: image.Pt(size, size)}
}

// DrawCpuIcon はCPUアイコンを描画する
func DrawCpuIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	// 中央の四角
	drawRect(gtx, f32.Pt(s*0.3, s*0.3), s*0.4, s*0.4, col)
	// 上下左右のピン
	drawLine(gtx, f32.Pt(s*0.4, s*0.15), f32.Pt(s*0.4, s*0.3), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.6, s*0.15), f32.Pt(s*0.6, s*0.3), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.4, s*0.7), f32.Pt(s*0.4, s*0.85), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.6, s*0.7), f32.Pt(s*0.6, s*0.85), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.15, s*0.4), f32.Pt(s*0.3, s*0.4), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.15, s*0.6), f32.Pt(s*0.3, s*0.6), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.7, s*0.4), f32.Pt(s*0.85, s*0.4), col, s*0.06)
	drawLine(gtx, f32.Pt(s*0.7, s*0.6), f32.Pt(s*0.85, s*0.6), col, s*0.06)
	return D{Size: image.Pt(size, size)}
}

// DrawZapIcon は稲妻アイコンを描画する
func DrawZapIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	drawLine(gtx, f32.Pt(s*0.55, s*0.1), f32.Pt(s*0.35, s*0.5), col, s*0.08)
	drawLine(gtx, f32.Pt(s*0.35, s*0.5), f32.Pt(s*0.6, s*0.5), col, s*0.08)
	drawLine(gtx, f32.Pt(s*0.6, s*0.5), f32.Pt(s*0.4, s*0.9), col, s*0.08)
	return D{Size: image.Pt(size, size)}
}

// DrawNewFileIcon はファイル新規作成アイコン（ファイル＋プラス）を描画する
func DrawNewFileIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// ファイル外形
	drawLine(gtx, f32.Pt(s*0.2, s*0.1), f32.Pt(s*0.48, s*0.1), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.48, s*0.1), f32.Pt(s*0.65, s*0.27), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.65, s*0.27), f32.Pt(s*0.65, s*0.55), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.2, s*0.85), f32.Pt(s*0.2, s*0.1), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.2, s*0.85), f32.Pt(s*0.65, s*0.85), col, 1.2)
	// 折り返し
	drawLine(gtx, f32.Pt(s*0.48, s*0.1), f32.Pt(s*0.48, s*0.27), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.48, s*0.27), f32.Pt(s*0.65, s*0.27), col, 1.2)
	// プラス記号
	drawLine(gtx, f32.Pt(s*0.75, s*0.55), f32.Pt(s*0.75, s*0.9), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.58, s*0.725), f32.Pt(s*0.92, s*0.725), col, 1.5)

	return D{Size: image.Pt(size, size)}
}

// DrawNewFolderIcon はフォルダ新規作成アイコン（フォルダ＋プラス）を描画する
func DrawNewFolderIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// フォルダ外形
	drawLine(gtx, f32.Pt(s*0.08, s*0.22), f32.Pt(s*0.35, s*0.22), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.35, s*0.22), f32.Pt(s*0.45, s*0.32), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.45, s*0.32), f32.Pt(s*0.82, s*0.32), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.82, s*0.32), f32.Pt(s*0.82, s*0.58), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.08, s*0.75), f32.Pt(s*0.82, s*0.75), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.08, s*0.75), f32.Pt(s*0.08, s*0.22), col, 1.2)
	// プラス記号
	drawLine(gtx, f32.Pt(s*0.75, s*0.58), f32.Pt(s*0.75, s*0.92), col, 1.5)
	drawLine(gtx, f32.Pt(s*0.58, s*0.75), f32.Pt(s*0.92, s*0.75), col, 1.5)

	return D{Size: image.Pt(size, size)}
}

// DrawOpenFolderIcon は開いたフォルダアイコンを描画する
func DrawOpenFolderIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()

	// フォルダ背面
	drawLine(gtx, f32.Pt(s*0.08, s*0.2), f32.Pt(s*0.35, s*0.2), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.35, s*0.2), f32.Pt(s*0.45, s*0.32), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.08, s*0.2), f32.Pt(s*0.08, s*0.8), col, 1.2)
	// 開いた前面
	drawLine(gtx, f32.Pt(s*0.08, s*0.8), f32.Pt(s*0.75, s*0.8), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.75, s*0.8), f32.Pt(s*0.92, s*0.42), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.92, s*0.42), f32.Pt(s*0.25, s*0.42), col, 1.2)
	drawLine(gtx, f32.Pt(s*0.25, s*0.42), f32.Pt(s*0.08, s*0.8), col, 1.2)

	return D{Size: image.Pt(size, size)}
}

// DrawEditIcon は編集アイコンを描画する
func DrawEditIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()
	drawLine(gtx, f32.Pt(s*0.2, s*0.8), f32.Pt(s*0.35, s*0.65), col, 1.4)
	drawLine(gtx, f32.Pt(s*0.35, s*0.65), f32.Pt(s*0.75, s*0.25), col, 1.4)
	drawLine(gtx, f32.Pt(s*0.68, s*0.18), f32.Pt(s*0.82, s*0.32), col, 1.4)
	drawLine(gtx, f32.Pt(s*0.18, s*0.82), f32.Pt(s*0.34, s*0.82), col, 1.4)
	return D{Size: image.Pt(size, size)}
}

// DrawTrashIcon は削除アイコンを描画する
func DrawTrashIcon(gtx C, size int, col color.NRGBA) D {
	s := float32(size)
	defer op.Offset(image.Point{}).Push(gtx.Ops).Pop()
	drawLine(gtx, f32.Pt(s*0.25, s*0.28), f32.Pt(s*0.75, s*0.28), col, 1.3)
	drawLine(gtx, f32.Pt(s*0.34, s*0.2), f32.Pt(s*0.66, s*0.2), col, 1.3)
	drawLine(gtx, f32.Pt(s*0.32, s*0.28), f32.Pt(s*0.38, s*0.82), col, 1.3)
	drawLine(gtx, f32.Pt(s*0.68, s*0.28), f32.Pt(s*0.62, s*0.82), col, 1.3)
	drawLine(gtx, f32.Pt(s*0.38, s*0.82), f32.Pt(s*0.62, s*0.82), col, 1.3)
	drawLine(gtx, f32.Pt(s*0.46, s*0.38), f32.Pt(s*0.46, s*0.72), col, 1.1)
	drawLine(gtx, f32.Pt(s*0.54, s*0.38), f32.Pt(s*0.54, s*0.72), col, 1.1)
	return D{Size: image.Pt(size, size)}
}

// --- 描画ヘルパー関数 ---

// drawLine は2点間に線を描画する
func drawLine(gtx C, from, to f32.Point, col color.NRGBA, width float32) {
	dx := to.X - from.X
	dy := to.Y - from.Y
	length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if length < 0.1 {
		return
	}

	// 線を矩形として描画
	angle := float32(math.Atan2(float64(dy), float64(dx)))

	halfW := width / 2
	sin, cos := float32(math.Sin(float64(angle))), float32(math.Cos(float64(angle)))

	// 4隅の座標
	p1 := f32.Pt(from.X-halfW*sin, from.Y+halfW*cos)
	p2 := f32.Pt(from.X+halfW*sin, from.Y-halfW*cos)
	p3 := f32.Pt(to.X+halfW*sin, to.Y-halfW*cos)
	p4 := f32.Pt(to.X-halfW*sin, to.Y+halfW*cos)

	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(p1)
	p.LineTo(p2)
	p.LineTo(p3)
	p.LineTo(p4)
	p.Close()
	defer clip.Outline{Path: p.End()}.Op().Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// drawCircle は円を描画する
func drawCircle(gtx C, center f32.Point, radius float32, col color.NRGBA) {
	r := int(radius + 0.5)
	if r < 1 {
		r = 1
	}
	irect := image.Rect(
		int(center.X-radius),
		int(center.Y-radius),
		int(center.X+radius),
		int(center.Y+radius),
	)
	defer clip.Ellipse(irect).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// drawRect は矩形を描画する（アウトラインのみ）
func drawRect(gtx C, pos f32.Point, w, h float32, col color.NRGBA) {
	tl := pos
	tr := f32.Pt(pos.X+w, pos.Y)
	br := f32.Pt(pos.X+w, pos.Y+h)
	bl := f32.Pt(pos.X, pos.Y+h)
	drawLine(gtx, tl, tr, col, 1.2)
	drawLine(gtx, tr, br, col, 1.2)
	drawLine(gtx, br, bl, col, 1.2)
	drawLine(gtx, bl, tl, col, 1.2)
}

// drawStar は4尖の星を描画する
func drawStar(gtx C, center f32.Point, size float32, col color.NRGBA) {
	cx, cy := center.X, center.Y
	h := size / 2
	// 十字の4方向に伸びる線
	drawLine(gtx, f32.Pt(cx, cy-h), f32.Pt(cx, cy+h), col, 1.2)
	drawLine(gtx, f32.Pt(cx-h, cy), f32.Pt(cx+h, cy), col, 1.2)
	// 対角線（短い）
	dh := h * 0.5
	drawLine(gtx, f32.Pt(cx-dh, cy-dh), f32.Pt(cx+dh, cy+dh), col, 0.8)
	drawLine(gtx, f32.Pt(cx+dh, cy-dh), f32.Pt(cx-dh, cy+dh), col, 0.8)
}

// fillBackground は指定色で矩形背景を塗りつぶす
func fillBackground(gtx C, col color.NRGBA, size image.Point) D {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return D{Size: size}
}

// fillRoundRect は角丸矩形を塗りつぶす
func fillRoundRect(gtx C, col color.NRGBA, size image.Point, radius int) D {
	defer clip.RRect{
		Rect: image.Rectangle{Max: size},
		NE:   radius, NW: radius, SE: radius, SW: radius,
	}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return D{Size: size}
}
