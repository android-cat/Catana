// Package terminal はPTYベースのターミナルエミュレータを提供する
package terminal

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ANSIカラーコード定数
const (
	ColorBlack   = 0
	ColorRed     = 1
	ColorGreen   = 2
	ColorYellow  = 3
	ColorBlue    = 4
	ColorMagenta = 5
	ColorCyan    = 6
	ColorWhite   = 7
)

// Cell はターミナルの1セル（1文字分）
type Cell struct {
	Char rune
	FG   int // 前景色 (0-255, -1=デフォルト)
	BG   int // 背景色 (0-255, -1=デフォルト)
	Bold bool
	Dim  bool
}

// defaultCell はデフォルトのセルを返す
func defaultCell() Cell {
	return Cell{Char: ' ', FG: -1, BG: -1}
}

// Terminal はPTYターミナルセッション
type Terminal struct {
	mu            sync.Mutex
	rows          int
	cols          int
	cells         [][]Cell // 画面バッファ [行][列]
	cursorR       int      // カーソル行
	cursorC       int      // カーソル列
	scrollback    [][]Cell // スクロールバックバッファ
	maxScrollback int      // スクロールバック最大行数
	// PTY 管理
	pty   *os.File
	cmd   *exec.Cmd
	alive bool
	// ANSIパーサー状態
	escState escapeState
	escBuf   []byte
	curAttr  Cell // 現在の属性
	// 出力通知
	onUpdate func()
}

type escapeState int

const (
	escNone escapeState = iota
	escEsc              // ESC を受信
	escCSI              // CSI シーケンス中
)

// New は新しいターミナルを作成する
func New(rows, cols int) *Terminal {
	t := &Terminal{
		rows:          rows,
		cols:          cols,
		maxScrollback: 10000,
		curAttr:       defaultCell(),
	}
	t.cells = t.makeScreen()
	return t
}

// SetUpdateCallback は画面更新通知のコールバックを設定する
func (t *Terminal) SetUpdateCallback(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onUpdate = fn
}

func (t *Terminal) makeScreen() [][]Cell {
	screen := make([][]Cell, t.rows)
	for i := range screen {
		screen[i] = make([]Cell, t.cols)
		for j := range screen[i] {
			screen[i][j] = defaultCell()
		}
	}
	return screen
}

// Start はシェルプロセスを起動する
func (t *Terminal) Start(shell string, cwd string, env []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.alive {
		return fmt.Errorf("ターミナルは既に起動中")
	}

	// シェルの検出
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
	}

	// PTYを開く
	ptmx, tty, err := openPTY()
	if err != nil {
		return fmt.Errorf("PTYオープン失敗: %w", err)
	}

	// ウィンドウサイズ設定
	_ = setWinSize(ptmx, t.rows, t.cols)

	// シェルプロセス起動
	t.cmd = exec.Command(shell, "-l") // ログインシェル
	t.cmd.Stdin = tty
	t.cmd.Stdout = tty
	t.cmd.Stderr = tty
	t.cmd.Dir = cwd

	// 環境変数設定
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, "TERM=xterm-256color")
	cmdEnv = append(cmdEnv, fmt.Sprintf("COLUMNS=%d", t.cols))
	cmdEnv = append(cmdEnv, fmt.Sprintf("LINES=%d", t.rows))
	if env != nil {
		cmdEnv = append(cmdEnv, env...)
	}
	t.cmd.Env = cmdEnv

	// 新しいセッションで起動（制御端末を設定）
	t.cmd.SysProcAttr = &unix.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := t.cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return fmt.Errorf("シェル起動失敗: %w", err)
	}

	tty.Close() // 親プロセスではttyを閉じる
	t.pty = ptmx
	t.alive = true

	// 出力読み取りgoroutine
	go t.readLoop()

	// プロセス終了監視
	go func() {
		_ = t.cmd.Wait()
		t.mu.Lock()
		t.alive = false
		t.mu.Unlock()
		t.notifyUpdate()
	}()

	return nil
}

// Write はターミナルにデータを書き込む（キー入力）
func (t *Terminal) Write(data []byte) error {
	t.mu.Lock()
	pty := t.pty
	alive := t.alive
	t.mu.Unlock()

	if !alive || pty == nil {
		return fmt.Errorf("ターミナルが起動していません")
	}
	_, err := pty.Write(data)
	return err
}

// WriteString は文字列をターミナルに書き込む
func (t *Terminal) WriteString(s string) error {
	return t.Write([]byte(s))
}

// IsAlive はプロセスが生きているかを返す
func (t *Terminal) IsAlive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alive
}

// Resize はターミナルサイズを変更する
func (t *Terminal) Resize(rows, cols int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if rows <= 0 || cols <= 0 {
		return
	}
	if rows == t.rows && cols == t.cols {
		return
	}

	oldScreen := t.cells
	oldRows := t.rows
	oldCols := t.cols
	t.rows = rows
	t.cols = cols
	t.cells = t.makeScreen()

	// 古い画面の内容をコピー
	copyRows := oldRows
	if copyRows > rows {
		copyRows = rows
	}
	copyCols := oldCols
	if copyCols > cols {
		copyCols = cols
	}
	for r := 0; r < copyRows; r++ {
		for c := 0; c < copyCols; c++ {
			t.cells[r][c] = oldScreen[r][c]
		}
	}

	// カーソル位置を調整
	if t.cursorR >= rows {
		t.cursorR = rows - 1
	}
	if t.cursorC >= cols {
		t.cursorC = cols - 1
	}

	// PTYウィンドウサイズ通知
	if t.pty != nil {
		_ = setWinSize(t.pty, rows, cols)
	}
}

// GetScreen は現在の画面バッファのスナップショットを返す
func (t *Terminal) GetScreen() (cells [][]Cell, cursorR, cursorC int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := make([][]Cell, t.rows)
	for i := range t.cells {
		snapshot[i] = make([]Cell, t.cols)
		copy(snapshot[i], t.cells[i])
	}
	return snapshot, t.cursorR, t.cursorC
}

// GetScrollback はスクロールバックバッファを返す
func (t *Terminal) GetScrollback() [][]Cell {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([][]Cell, len(t.scrollback))
	for i := range t.scrollback {
		result[i] = make([]Cell, len(t.scrollback[i]))
		copy(result[i], t.scrollback[i])
	}
	return result
}

// Size はターミナルのサイズ(行, 列)を返す
func (t *Terminal) Size() (rows, cols int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rows, t.cols
}

// Close はターミナルを閉じる
func (t *Terminal) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.alive = false
	if t.pty != nil {
		t.pty.Close()
		t.pty = nil
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
}

// ─── PTY読み取り ───

func (t *Terminal) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		t.mu.Lock()
		pty := t.pty
		alive := t.alive
		t.mu.Unlock()

		if !alive || pty == nil {
			return
		}
		n, err := pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("[ターミナル] 読み取りエラー: %v", err)
			}
			return
		}
		if n > 0 {
			t.mu.Lock()
			t.processOutput(buf[:n])
			t.mu.Unlock()
			t.notifyUpdate()
		}
	}
}

func (t *Terminal) notifyUpdate() {
	t.mu.Lock()
	fn := t.onUpdate
	t.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// ─── ANSIエスケープシーケンス解析 ───

func (t *Terminal) processOutput(data []byte) {
	for i := 0; i < len(data); {
		switch t.escState {
		case escNone:
			r, size := utf8.DecodeRune(data[i:])
			if size == 0 {
				i++
				continue
			}
			i += size
			switch {
			case r == 0x1B: // ESC
				t.escState = escEsc
				t.escBuf = t.escBuf[:0]
			case r == '\n':
				t.lineFeed()
			case r == '\r':
				t.cursorC = 0
			case r == '\t':
				// 次のタブストップ（8の倍数）
				t.cursorC = (t.cursorC/8 + 1) * 8
				if t.cursorC >= t.cols {
					t.cursorC = t.cols - 1
				}
			case r == '\b': // Backspace
				if t.cursorC > 0 {
					t.cursorC--
				}
			case r == 0x07: // BEL (無視)
			case r >= 0x20: // 表示可能文字
				t.putChar(r)
			}

		case escEsc:
			i++
			b := data[i-1]
			switch b {
			case '[': // CSI
				t.escState = escCSI
				t.escBuf = t.escBuf[:0]
			case ']': // OSC (簡略処理: ST まで読み飛ばし)
				t.escState = escNone
				// OSCはBELか\x1b\\まで読み飛ばす
				for i < len(data) {
					if data[i] == 0x07 {
						i++
						break
					}
					if data[i] == 0x1B && i+1 < len(data) && data[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			case '(', ')': // 文字セット選択（読み飛ばし）
				if i < len(data) {
					i++
				}
				t.escState = escNone
			case '=', '>': // キーパッドモード（無視）
				t.escState = escNone
			default:
				t.escState = escNone
			}

		case escCSI:
			b := data[i]
			i++
			// CSI終端文字
			if b >= 0x40 && b <= 0x7E {
				t.executeCSI(b)
				t.escState = escNone
			} else {
				t.escBuf = append(t.escBuf, b)
				// バッファオーバーフロー防止
				if len(t.escBuf) > 256 {
					t.escState = escNone
				}
			}
		}
	}
}

func (t *Terminal) putChar(r rune) {
	if t.cursorR < 0 || t.cursorR >= t.rows {
		return
	}
	if t.cursorC >= t.cols {
		t.cursorC = 0
		t.lineFeed()
	}
	if t.cursorR < t.rows && t.cursorC < t.cols {
		t.cells[t.cursorR][t.cursorC] = Cell{
			Char: r,
			FG:   t.curAttr.FG,
			BG:   t.curAttr.BG,
			Bold: t.curAttr.Bold,
			Dim:  t.curAttr.Dim,
		}
		t.cursorC++
	}
}

func (t *Terminal) lineFeed() {
	if t.cursorR < t.rows-1 {
		t.cursorR++
	} else {
		// スクロール
		t.scrollUp()
	}
}

func (t *Terminal) scrollUp() {
	// 先頭行をスクロールバックに追加
	if len(t.scrollback) < t.maxScrollback {
		row := make([]Cell, t.cols)
		copy(row, t.cells[0])
		t.scrollback = append(t.scrollback, row)
	}
	// 画面を1行上にシフト
	for r := 0; r < t.rows-1; r++ {
		t.cells[r], t.cells[r+1] = t.cells[r+1], t.cells[r]
	}
	// 最終行をクリア
	for c := 0; c < t.cols; c++ {
		t.cells[t.rows-1][c] = defaultCell()
	}
}

func (t *Terminal) executeCSI(final byte) {
	params := t.parseCSIParams()
	switch final {
	case 'A': // カーソル上
		n := paramDefault(params, 0, 1)
		t.cursorR -= n
		if t.cursorR < 0 {
			t.cursorR = 0
		}
	case 'B': // カーソル下
		n := paramDefault(params, 0, 1)
		t.cursorR += n
		if t.cursorR >= t.rows {
			t.cursorR = t.rows - 1
		}
	case 'C': // カーソル右
		n := paramDefault(params, 0, 1)
		t.cursorC += n
		if t.cursorC >= t.cols {
			t.cursorC = t.cols - 1
		}
	case 'D': // カーソル左
		n := paramDefault(params, 0, 1)
		t.cursorC -= n
		if t.cursorC < 0 {
			t.cursorC = 0
		}
	case 'H', 'f': // カーソル位置設定
		row := paramDefault(params, 0, 1) - 1
		col := paramDefault(params, 1, 1) - 1
		if row < 0 {
			row = 0
		}
		if row >= t.rows {
			row = t.rows - 1
		}
		if col < 0 {
			col = 0
		}
		if col >= t.cols {
			col = t.cols - 1
		}
		t.cursorR = row
		t.cursorC = col
	case 'J': // 画面クリア
		n := paramDefault(params, 0, 0)
		switch n {
		case 0: // カーソルから画面末尾
			t.clearFromCursor()
		case 1: // 画面先頭からカーソル
			t.clearToCursor()
		case 2, 3: // 画面全体
			t.clearScreen()
		}
	case 'K': // 行クリア
		n := paramDefault(params, 0, 0)
		switch n {
		case 0: // カーソルから行末
			for c := t.cursorC; c < t.cols; c++ {
				t.cells[t.cursorR][c] = defaultCell()
			}
		case 1: // 行頭からカーソル
			for c := 0; c <= t.cursorC && c < t.cols; c++ {
				t.cells[t.cursorR][c] = defaultCell()
			}
		case 2: // 行全体
			for c := 0; c < t.cols; c++ {
				t.cells[t.cursorR][c] = defaultCell()
			}
		}
	case 'm': // SGR（属性設定）
		t.executeSGR(params)
	case 'r': // スクロール領域（簡略：無視）
	case 'h', 'l': // モード設定/リセット（簡略）
	case 'G': // カーソル列移動
		col := paramDefault(params, 0, 1) - 1
		if col < 0 {
			col = 0
		}
		if col >= t.cols {
			col = t.cols - 1
		}
		t.cursorC = col
	case 'd': // カーソル行移動
		row := paramDefault(params, 0, 1) - 1
		if row < 0 {
			row = 0
		}
		if row >= t.rows {
			row = t.rows - 1
		}
		t.cursorR = row
	case 'L': // 行挿入
		n := paramDefault(params, 0, 1)
		for i := 0; i < n && t.cursorR < t.rows; i++ {
			// カーソル行から下をシフト
			for r := t.rows - 1; r > t.cursorR; r-- {
				t.cells[r] = t.cells[r-1]
			}
			t.cells[t.cursorR] = make([]Cell, t.cols)
			for c := range t.cells[t.cursorR] {
				t.cells[t.cursorR][c] = defaultCell()
			}
		}
	case 'M': // 行削除
		n := paramDefault(params, 0, 1)
		for i := 0; i < n && t.cursorR < t.rows; i++ {
			for r := t.cursorR; r < t.rows-1; r++ {
				t.cells[r] = t.cells[r+1]
			}
			t.cells[t.rows-1] = make([]Cell, t.cols)
			for c := range t.cells[t.rows-1] {
				t.cells[t.rows-1][c] = defaultCell()
			}
		}
	case '@': // 文字挿入
		n := paramDefault(params, 0, 1)
		for i := 0; i < n; i++ {
			for c := t.cols - 1; c > t.cursorC; c-- {
				t.cells[t.cursorR][c] = t.cells[t.cursorR][c-1]
			}
			t.cells[t.cursorR][t.cursorC] = defaultCell()
		}
	case 'P': // 文字削除
		n := paramDefault(params, 0, 1)
		for i := 0; i < n; i++ {
			for c := t.cursorC; c < t.cols-1; c++ {
				t.cells[t.cursorR][c] = t.cells[t.cursorR][c+1]
			}
			t.cells[t.cursorR][t.cols-1] = defaultCell()
		}
	}
}

func (t *Terminal) executeSGR(params []int) {
	if len(params) == 0 {
		params = []int{0}
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == 0: // リセット
			t.curAttr = defaultCell()
		case p == 1: // 太字
			t.curAttr.Bold = true
		case p == 2: // 暗い
			t.curAttr.Dim = true
		case p == 22: // 通常
			t.curAttr.Bold = false
			t.curAttr.Dim = false
		case p >= 30 && p <= 37: // 前景色（標準）
			t.curAttr.FG = p - 30
		case p == 38: // 前景色（拡張）
			if i+1 < len(params) && params[i+1] == 5 && i+2 < len(params) {
				t.curAttr.FG = params[i+2]
				i += 2
			}
		case p == 39: // デフォルト前景色
			t.curAttr.FG = -1
		case p >= 40 && p <= 47: // 背景色（標準）
			t.curAttr.BG = p - 40
		case p == 48: // 背景色（拡張）
			if i+1 < len(params) && params[i+1] == 5 && i+2 < len(params) {
				t.curAttr.BG = params[i+2]
				i += 2
			}
		case p == 49: // デフォルト背景色
			t.curAttr.BG = -1
		case p >= 90 && p <= 97: // 明るい前景色
			t.curAttr.FG = p - 90 + 8
		case p >= 100 && p <= 107: // 明るい背景色
			t.curAttr.BG = p - 100 + 8
		}
	}
}

func (t *Terminal) clearScreen() {
	for r := 0; r < t.rows; r++ {
		for c := 0; c < t.cols; c++ {
			t.cells[r][c] = defaultCell()
		}
	}
}

func (t *Terminal) clearFromCursor() {
	// カーソル位置から行末
	for c := t.cursorC; c < t.cols; c++ {
		t.cells[t.cursorR][c] = defaultCell()
	}
	// 以降の行
	for r := t.cursorR + 1; r < t.rows; r++ {
		for c := 0; c < t.cols; c++ {
			t.cells[r][c] = defaultCell()
		}
	}
}

func (t *Terminal) clearToCursor() {
	// 先頭からカーソル前行まで
	for r := 0; r < t.cursorR; r++ {
		for c := 0; c < t.cols; c++ {
			t.cells[r][c] = defaultCell()
		}
	}
	// カーソル行の先頭からカーソルまで
	for c := 0; c <= t.cursorC && c < t.cols; c++ {
		t.cells[t.cursorR][c] = defaultCell()
	}
}

func (t *Terminal) parseCSIParams() []int {
	s := string(t.escBuf)
	// '?' や '>' プレフィックスを除去
	s = strings.TrimLeft(s, "?>=!")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	params := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			params = append(params, 0)
			continue
		}
		n := 0
		for _, ch := range p {
			if ch >= '0' && ch <= '9' {
				n = n*10 + int(ch-'0')
			}
		}
		params = append(params, n)
	}
	return params
}

func paramDefault(params []int, idx int, def int) int {
	if idx >= len(params) || params[idx] == 0 {
		return def
	}
	return params[idx]
}

// ScreenText はスクリーンの内容をテキストとして返す（デバッグ用）
func (t *Terminal) ScreenText() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var sb strings.Builder
	for r := 0; r < t.rows; r++ {
		for c := 0; c < t.cols; c++ {
			ch := t.cells[r][c].Char
			if ch == 0 {
				ch = ' '
			}
			sb.WriteRune(ch)
		}
		if r < t.rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ─── PTY ヘルパー (macOS/Unixのみ) ───

func openPTY() (ptmx *os.File, tty *os.File, err error) {
	ptmx, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}

	// grantpt / unlockpt
	if _, err := unix.IoctlGetInt(int(ptmx.Fd()), unix.TIOCPTYGRANT); err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("grantpt 失敗: %w", err)
	}
	if _, err := unix.IoctlGetInt(int(ptmx.Fd()), unix.TIOCPTYUNLK); err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("unlockpt 失敗: %w", err)
	}

	// スレーブ名取得
	name, err := ptsname(ptmx)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("ptsname 失敗: %w", err)
	}

	tty, err = os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("スレーブPTYオープン失敗: %w", err)
	}

	return ptmx, tty, nil
}

func ptsname(f *os.File) (string, error) {
	// TIOCPTYGNAME はchar[128]バッファにスレーブPTY名を書き込む
	var buf [128]byte
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		f.Fd(),
		uintptr(unix.TIOCPTYGNAME),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if errno != 0 {
		return "", fmt.Errorf("TIOCPTYGNAME失敗: %v", errno)
	}
	// ヌル終端文字列を取得
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n]), nil
}

func setWinSize(f *os.File, rows, cols int) error {
	ws := &unix.Winsize{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, ws)
}
