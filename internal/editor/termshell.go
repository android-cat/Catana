package editor

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
)

// isInteractiveCmd はインタラクティブ（TTY必須）なコマンドかどうかを判定する
// nano, vim, top, codex など PTY が必要なコマンドを検出する
// alwaysInteractive: 引数の有無に関わらず常にインタラクティブ
var alwaysInteractive = map[string]bool{
	// エディタ
	"nano": true, "vim": true, "vi": true, "nvim": true, "emacs": true,
	"micro": true, "helix": true, "hx": true, "kakoune": true, "kak": true,
	"joe": true, "pico": true, "ne": true, "zile": true,
	// ページャ
	"less": true, "more": true, "most": true,
	// システムモニタ
	"top": true, "htop": true, "btop": true, "atop": true,
	"iotop": true, "nmon": true, "glances": true, "bpytop": true,
	"gotop": true, "ytop": true, "zenith": true,
	// ファイルマネージャ
	"mc": true, "ranger": true, "nnn": true, "lf": true,
	"vifm": true, "fff": true, "yazi": true, "broot": true,
	// ターミナルマルチプレクサ
	"tmux": true, "screen": true, "byobu": true, "zellij": true,
	// リモート接続
	"ssh": true, "telnet": true, "mosh": true,
	// man / info
	"man": true, "info": true,
	// Git TUI
	"tig": true, "lazygit": true, "lazydocker": true,
	// ディスク
	"ncdu": true, "gdu": true,
	// ネットワークモニタ
	"nethogs": true, "iftop": true, "bmon": true, "nload": true,
	// AI CLI
	"codex": true, "claude": true, "aider": true, "gpt": true,
	// その他TUI
	"fzf": true, "dialog": true, "whiptail": true, "cmus": true,
	"newsboat": true, "w3m": true, "lynx": true, "links": true,
}

// replOnlyInteractive: 引数なし（REPL起動）の場合のみインタラクティブ
var replOnlyInteractive = map[string]bool{
	// Python
	"python": true, "python3": true, "python2": true,
	"ipython": true, "ipython3": true, "bpython": true, "pypy": true,
	// JavaScript / TypeScript
	"node": true, "deno": true, "bun": true,
	// Ruby
	"irb": true, "pry": true,
	// PHP
	"php": true,
	// Lua
	"lua": true, "luajit": true,
	// R / Julia / Swift
	"R": true, "julia": true, "swift": true,
	// Elixir / Erlang
	"iex": true, "erl": true,
	// Haskell / Scala / Clojure
	"ghci": true, "scala": true, "clj": true, "lein": true,
	// シェル（引数なしでインタラクティブシェル起動）
	"bash": true, "zsh": true, "fish": true, "sh": true,
	// データベースCLI
	"mysql": true, "psql": true, "sqlite3": true, "sqlite": true,
	"mongosh": true, "mongo": true, "redis-cli": true,
	"clickhouse-client": true, "influx": true, "cqlsh": true,
}

// IsInteractiveCommand はコマンド文字列がインタラクティブなプログラムかを判定する
// TTY必須コマンド、REPL起動、長時間フォアグラウンド実行コマンドを検出する
func IsInteractiveCommand(cmd string) bool {
	fields := strings.Fields(cmd)
	// 先頭の環境変数指定(VAR=val)やsudo等をスキップしてコマンド名のインデックスを取得
	cmdIdx := -1
	for i, f := range fields {
		if strings.Contains(f, "=") {
			continue // 環境変数指定
		}
		if f == "sudo" || f == "env" || f == "command" {
			continue
		}
		cmdIdx = i
		break
	}
	if cmdIdx < 0 {
		return false
	}
	// パス付きコマンドからベース名を取得
	base := fields[cmdIdx]
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	args := fields[cmdIdx+1:]

	if alwaysInteractive[base] {
		return true
	}
	if replOnlyInteractive[base] {
		// コマンド名の後に引数がなければREPL起動
		return len(args) == 0
	}
	// 長時間フォアグラウンド実行パターンを検出
	return isLongRunning(base, args)
}

// isLongRunning はコマンド＋引数の組み合わせで長時間実行になるパターンを検出する
func isLongRunning(base string, args []string) bool {
	switch base {
	case "docker":
		// docker compose up / docker-compose up（-d なし）
		return matchDockerUp(args)
	case "docker-compose":
		return matchSubcmdWithoutFlag(args, "up", "-d")
	case "tail":
		return hasFlag(args, "-f", "-F", "--follow")
	case "ping":
		// -c（回数指定）がなければ永続
		return !hasFlag(args, "-c")
	case "watch":
		return true
	case "npm", "npx", "yarn", "pnpm", "bun":
		return matchNpmLongRunning(args)
	case "cargo":
		return matchSubcmd(args, "watch")
	case "go":
		return matchSubcmd(args, "run") // go run でサーバー起動の可能性
	case "flask", "uvicorn", "gunicorn", "rails", "serve":
		return true
	case "kubectl":
		return matchSubcmd(args, "logs") && hasFlag(args, "-f", "--follow")
	}
	return false
}

// matchDockerUp は docker [compose] up で -d がないパターンを検出
func matchDockerUp(args []string) bool {
	for i, a := range args {
		if a == "compose" && i+1 < len(args) {
			return matchSubcmdWithoutFlag(args[i+1:], "up", "-d")
		}
		if a == "run" {
			return true // docker run（-d なし）
		}
		if a == "logs" {
			return hasFlag(args, "-f", "--follow")
		}
	}
	return false
}

// matchNpmLongRunning は npm/yarn 等の長時間実行サブコマンドを検出
func matchNpmLongRunning(args []string) bool {
	for _, a := range args {
		switch a {
		case "start", "dev", "serve", "watch", "preview":
			return true
		}
		if a == "run" {
			// npm run dev, npm run start 等
			continue
		}
	}
	return false
}

// matchSubcmd はargsの中に指定サブコマンドがあるか
func matchSubcmd(args []string, sub string) bool {
	for _, a := range args {
		if a == sub {
			return true
		}
	}
	return false
}

// matchSubcmdWithoutFlag はサブコマンドがありフラグがないパターン
func matchSubcmdWithoutFlag(args []string, sub, flag string) bool {
	hasSub := false
	hasF := false
	for _, a := range args {
		if a == sub {
			hasSub = true
		}
		if a == flag {
			hasF = true
		}
	}
	return hasSub && !hasF
}

// hasFlag はargs内に指定フラグのいずれかがあるか
func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// TermShell はオムニバーTERMモード用の永続シェルプロセス
type TermShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser
	mu     sync.Mutex
	alive  bool
	cwd    string // 現在の作業ディレクトリ
}

// NewTermShell は新しい永続シェルを起動する
func NewTermShell(workDir string) (*TermShell, error) {
	cmd := exec.Command("sh")
	cmd.Dir = workDir
	// PS1を無効化してプロンプト出力を防ぐ
	cmd.Env = append(cmd.Environ(), "PS1=", "PS2=")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdoutPipe.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	ts := &TermShell{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
		stderr: stderrPipe,
		alive:  true,
		cwd:    workDir,
	}

	return ts, nil
}

// Execute はコマンドをシェルに送信し、出力を返す
// マーカーで出力の終端を検出する
func (ts *TermShell) Execute(command string) (output string, cwd string, exitOK bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if !ts.alive {
		return "シェルが終了しています", ts.cwd, false
	}

	// マーカー文字列（出力と混同しないユニーク文字列）
	marker := "__CATANA_END__"

	// コマンド送信: コマンド実行 → 終了コード保存 → cwd取得 → マーカー出力
	script := fmt.Sprintf(
		"%s\n__catana_ec=$?\necho \"%s\"\necho \"$__catana_ec\"\npwd\necho \"%s\"\n",
		command, marker, marker,
	)

	_, err := io.WriteString(ts.stdin, script)
	if err != nil {
		ts.alive = false
		return fmt.Sprintf("シェル書き込みエラー: %v", err), ts.cwd, false
	}

	// 最初のマーカーまでの出力を読み取る（コマンド出力）
	var outputLines []string
	for {
		line, err := ts.stdout.ReadString('\n')
		if err != nil {
			ts.alive = false
			return fmt.Sprintf("シェル読み取りエラー: %v", err), ts.cwd, false
		}
		line = strings.TrimRight(line, "\n")
		if line == marker {
			break
		}
		outputLines = append(outputLines, line)
	}

	// 終了コード読み取り
	ecLine, err := ts.stdout.ReadString('\n')
	if err != nil {
		ts.alive = false
		return strings.Join(outputLines, "\n"), ts.cwd, false
	}
	ecLine = strings.TrimRight(ecLine, "\n")
	exitOK = ecLine == "0"

	// cwd読み取り
	cwdLine, err := ts.stdout.ReadString('\n')
	if err != nil {
		ts.alive = false
		return strings.Join(outputLines, "\n"), ts.cwd, exitOK
	}
	cwdLine = strings.TrimRight(cwdLine, "\n")
	if cwdLine != "" {
		ts.cwd = cwdLine
	}

	// 2番目のマーカーを読み取る
	endLine, err := ts.stdout.ReadString('\n')
	if err != nil {
		ts.alive = false
	}
	_ = strings.TrimRight(endLine, "\n")

	output = strings.Join(outputLines, "\n")
	cwd = ts.cwd
	return output, cwd, exitOK
}

// Cwd は現在の作業ディレクトリを返す
func (ts *TermShell) Cwd() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.cwd
}

// IsAlive はシェルが生存しているかを返す
func (ts *TermShell) IsAlive() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.alive
}

// Close はシェルプロセスを終了する
func (ts *TermShell) Close() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if !ts.alive {
		return
	}
	ts.alive = false
	ts.stdin.Close()
	if err := ts.cmd.Process.Kill(); err != nil {
		log.Printf("[TermShell] kill error: %v", err)
	}
	ts.cmd.Wait()
}
