package git

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FileStatus はGitファイルのステータスを表す
type FileStatus int

const (
	StatusUnmodified    FileStatus = iota
	StatusModified                 // 変更あり
	StatusAdded                    // 新規追加（ステージ済み）
	StatusDeleted                  // 削除
	StatusRenamed                  // リネーム
	StatusUntracked                // 未追跡
	StatusConflicted               // コンフリクト
	StatusStaged                   // ステージ済み（変更）
	StatusStagedNew                // ステージ済み（新規）
	StatusStagedDeleted            // ステージ済み（削除）
)

// FileEntry はGitステータス付きファイルエントリ
type FileEntry struct {
	Path       string     // ワークスペース相対パス
	Status     FileStatus // ワーキングツリー側のステータス
	Staged     FileStatus // ステージ側のステータス
	OldPath    string     // リネーム前のパス
	IsConflict bool       // コンフリクト状態
}

// DiffLine はdiffの1行を表す
type DiffLine struct {
	Type    DiffLineType // 行タイプ
	Content string       // 行内容（改行なし）
	OldNum  int          // 変更前の行番号（0=該当なし）
	NewNum  int          // 変更後の行番号（0=該当なし）
}

// DiffLineType はdiff行の種別
type DiffLineType int

const (
	DiffContext  DiffLineType = iota // コンテキスト（変更なし）
	DiffAdded                        // 追加行
	DiffDeleted                      // 削除行
	DiffHunkLine                     // ハンクヘッダー
	DiffHeader                       // ファイルヘッダー
)

// DiffHunk はdiffのハンク（変更ブロック）
type DiffHunk struct {
	OldStart int        // 変更前開始行
	OldCount int        // 変更前行数
	NewStart int        // 変更後開始行
	NewCount int        // 変更後行数
	Lines    []DiffLine // ハンク内の行
}

// FileDiff はファイル単位のdiff
type FileDiff struct {
	Path     string     // ファイルパス
	OldPath  string     // リネーム前のパス（空なら非リネーム）
	Hunks    []DiffHunk // ハンク一覧
	IsBinary bool       // バイナリファイル
}

// LogEntry はGitログのエントリ
type LogEntry struct {
	Hash    string    // コミットハッシュ（短縮形）
	Author  string    // 著者名
	Date    time.Time // 日時
	Message string    // コミットメッセージ（1行目）
}

// BlameLine はblameの1行情報
type BlameLine struct {
	Hash    string // コミットハッシュ
	Author  string // 著者名
	Date    string // 日付文字列
	LineNum int    // 行番号
	Content string // 行内容
}

// StashEntry はstashのエントリ
type StashEntry struct {
	Index   int    // stash@{n}
	Message string // stashメッセージ
}

// Repository はGitリポジトリの操作を提供する
type Repository struct {
	workDir      string // ワークスペースディレクトリ
	mu           sync.Mutex
	cachedBranch string
	cacheTime    time.Time
}

// NewRepository は新しいRepositoryを作成する
func NewRepository(workDir string) *Repository {
	return &Repository{
		workDir: workDir,
	}
}

// IsGitRepo はワークスペースがGitリポジトリかどうかを判定する
func (r *Repository) IsGitRepo() bool {
	_, err := r.runGit("rev-parse", "--is-inside-work-tree")
	return err == nil
}

// Branch は現在のブランチ名を返す
func (r *Repository) Branch() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 1秒間キャッシュ
	if time.Since(r.cacheTime) < time.Second && r.cachedBranch != "" {
		return r.cachedBranch
	}

	out, err := r.runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(out)
	r.cachedBranch = branch
	r.cacheTime = time.Now()
	return branch
}

// Status はワーキングツリーのGitステータスを返す
func (r *Repository) Status() ([]FileEntry, error) {
	out, err := r.runGit("status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, fmt.Errorf("git status 失敗: %w", err)
	}
	return parseStatusOutput(out), nil
}

// Diff はステージされていない変更のdiffを返す
func (r *Repository) Diff(path string) ([]FileDiff, error) {
	args := []string{"diff", "--no-color", "-U3"}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := r.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("git diff 失敗: %w", err)
	}
	return parseDiffOutput(out), nil
}

// DiffStaged はステージ済み変更のdiffを返す
func (r *Repository) DiffStaged(path string) ([]FileDiff, error) {
	args := []string{"diff", "--cached", "--no-color", "-U3"}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := r.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("git diff --cached 失敗: %w", err)
	}
	return parseDiffOutput(out), nil
}

// DiffFile は特定ファイルのHEADからの差分を返す
func (r *Repository) DiffFile(path string) ([]FileDiff, error) {
	args := []string{"diff", "HEAD", "--no-color", "-U3", "--", path}
	out, err := r.runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("git diff HEAD 失敗: %w", err)
	}
	return parseDiffOutput(out), nil
}

// Stage はファイルをステージに追加する
func (r *Repository) Stage(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := r.runGit(args...)
	if err != nil {
		return fmt.Errorf("git add 失敗: %w", err)
	}
	return nil
}

// StageAll は全ての変更をステージに追加する
func (r *Repository) StageAll() error {
	_, err := r.runGit("add", "-A")
	if err != nil {
		return fmt.Errorf("git add -A 失敗: %w", err)
	}
	return nil
}

// Unstage はファイルをステージから外す
func (r *Repository) Unstage(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"restore", "--staged", "--"}, paths...)
	_, err := r.runGit(args...)
	if err != nil {
		return fmt.Errorf("git restore --staged 失敗: %w", err)
	}
	return nil
}

// UnstageAll は全てのステージ済み変更を戻す
func (r *Repository) UnstageAll() error {
	_, err := r.runGit("reset", "HEAD")
	if err != nil {
		return fmt.Errorf("git reset HEAD 失敗: %w", err)
	}
	return nil
}

// DiscardChanges はファイルの変更を破棄する
func (r *Repository) DiscardChanges(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"checkout", "--"}, paths...)
	_, err := r.runGit(args...)
	if err != nil {
		return fmt.Errorf("git checkout 失敗: %w", err)
	}
	return nil
}

// Commit はステージ済みの変更をコミットする
func (r *Repository) Commit(message string) error {
	if message == "" {
		return fmt.Errorf("コミットメッセージが空です")
	}
	_, err := r.runGit("commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit 失敗: %w", err)
	}
	return nil
}

// Log はコミット履歴を返す
func (r *Repository) Log(limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	out, err := r.runGit("log",
		fmt.Sprintf("--max-count=%d", limit),
		"--format=%h\x00%an\x00%aI\x00%s",
	)
	if err != nil {
		return nil, fmt.Errorf("git log 失敗: %w", err)
	}
	return parseLogOutput(out), nil
}

// Blame はファイルのblame情報を返す
func (r *Repository) Blame(path string) ([]BlameLine, error) {
	out, err := r.runGit("blame", "--porcelain", "--", path)
	if err != nil {
		return nil, fmt.Errorf("git blame 失敗: %w", err)
	}
	return parseBlameOutput(out), nil
}

// Stash は現在の変更をstashする
func (r *Repository) Stash(message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := r.runGit(args...)
	if err != nil {
		return fmt.Errorf("git stash 失敗: %w", err)
	}
	return nil
}

// StashPop はstashから最新の変更を復元する
func (r *Repository) StashPop() error {
	_, err := r.runGit("stash", "pop")
	if err != nil {
		return fmt.Errorf("git stash pop 失敗: %w", err)
	}
	return nil
}

// StashList はstash一覧を返す
func (r *Repository) StashList() ([]StashEntry, error) {
	out, err := r.runGit("stash", "list", "--format=%gd\x00%s")
	if err != nil {
		return nil, fmt.Errorf("git stash list 失敗: %w", err)
	}
	return parseStashOutput(out), nil
}

// StashDrop は指定のstashを削除する
func (r *Repository) StashDrop(index int) error {
	_, err := r.runGit("stash", "drop", fmt.Sprintf("stash@{%d}", index))
	if err != nil {
		return fmt.Errorf("git stash drop 失敗: %w", err)
	}
	return nil
}

// FileLog は特定ファイルの履歴を返す
func (r *Repository) FileLog(path string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	out, err := r.runGit("log",
		fmt.Sprintf("--max-count=%d", limit),
		"--format=%h\x00%an\x00%aI\x00%s",
		"--follow",
		"--", path,
	)
	if err != nil {
		return nil, fmt.Errorf("git log 失敗: %w", err)
	}
	return parseLogOutput(out), nil
}

// runGit はgitコマンドを実行して出力を返す
func (r *Repository) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.workDir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", err, string(ee.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// --- パーサー ---

// parseStatusOutput は git status --porcelain=v1 の出力をパースする
func parseStatusOutput(output string) []FileEntry {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var entries []FileEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 4 {
			continue
		}
		x := line[0] // ステージ側
		y := line[1] // ワーキングツリー側
		path := line[3:]

		// リネーム検出
		oldPath := ""
		if idx := strings.Index(path, " -> "); idx >= 0 {
			oldPath = path[:idx]
			path = path[idx+4:]
		}

		entry := FileEntry{
			Path:    path,
			OldPath: oldPath,
			Staged:  charToStatus(x, true),
			Status:  charToStatus(y, false),
		}

		// コンフリクト判定
		if (x == 'U' || y == 'U') || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			entry.IsConflict = true
			entry.Status = StatusConflicted
		}

		entries = append(entries, entry)
	}
	return entries
}

// charToStatus はporcelain形式の文字をFileStatusに変換する
func charToStatus(c byte, staged bool) FileStatus {
	switch c {
	case 'M':
		if staged {
			return StatusStaged
		}
		return StatusModified
	case 'A':
		if staged {
			return StatusStagedNew
		}
		return StatusAdded
	case 'D':
		if staged {
			return StatusStagedDeleted
		}
		return StatusDeleted
	case 'R':
		return StatusRenamed
	case '?':
		return StatusUntracked
	case 'U':
		return StatusConflicted
	default:
		return StatusUnmodified
	}
}

// parseDiffOutput は git diff の出力をパースする
func parseDiffOutput(output string) []FileDiff {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var diffs []FileDiff
	var current *FileDiff
	var currentHunk *DiffHunk
	oldNum, newNum := 0, 0

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		// 新しいファイルdiff開始
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				if currentHunk != nil {
					current.Hunks = append(current.Hunks, *currentHunk)
					currentHunk = nil
				}
				diffs = append(diffs, *current)
			}
			current = &FileDiff{}
			parts := strings.SplitN(line, " b/", 2)
			if len(parts) == 2 {
				current.Path = parts[1]
			}
			continue
		}

		if current == nil {
			continue
		}

		// リネーム検出
		if strings.HasPrefix(line, "rename from ") {
			current.OldPath = strings.TrimPrefix(line, "rename from ")
			continue
		}
		if strings.HasPrefix(line, "rename to ") {
			current.Path = strings.TrimPrefix(line, "rename to ")
			continue
		}

		// バイナリファイル
		if strings.HasPrefix(line, "Binary files") {
			current.IsBinary = true
			continue
		}

		// ヘッダー行
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}

		// ハンクヘッダー
		if strings.HasPrefix(line, "@@ ") {
			if currentHunk != nil {
				current.Hunks = append(current.Hunks, *currentHunk)
			}
			currentHunk = parseHunkHeader(line)
			if currentHunk != nil {
				oldNum = currentHunk.OldStart
				newNum = currentHunk.NewStart
			}
			continue
		}

		if currentHunk == nil {
			continue
		}

		// diff行パース
		if len(line) == 0 {
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffContext,
				Content: "",
				OldNum:  oldNum,
				NewNum:  newNum,
			})
			oldNum++
			newNum++
		} else {
			switch line[0] {
			case '+':
				currentHunk.Lines = append(currentHunk.Lines, DiffLine{
					Type:    DiffAdded,
					Content: line[1:],
					NewNum:  newNum,
				})
				newNum++
			case '-':
				currentHunk.Lines = append(currentHunk.Lines, DiffLine{
					Type:    DiffDeleted,
					Content: line[1:],
					OldNum:  oldNum,
				})
				oldNum++
			case '\\':
				// "\ No newline at end of file" は無視
			default:
				content := line
				if len(content) > 0 && content[0] == ' ' {
					content = content[1:]
				}
				currentHunk.Lines = append(currentHunk.Lines, DiffLine{
					Type:    DiffContext,
					Content: content,
					OldNum:  oldNum,
					NewNum:  newNum,
				})
				oldNum++
				newNum++
			}
		}
	}

	// 最後のファイルとハンクをフラッシュ
	if current != nil {
		if currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
		}
		diffs = append(diffs, *current)
	}

	return diffs
}

// parseHunkHeader は @@ -old,count +new,count @@ をパースする
func parseHunkHeader(line string) *DiffHunk {
	line = strings.TrimPrefix(line, "@@ ")
	idx := strings.Index(line, " @@")
	if idx < 0 {
		return nil
	}
	ranges := line[:idx]
	parts := strings.SplitN(ranges, " ", 2)
	if len(parts) != 2 {
		return nil
	}

	hunk := &DiffHunk{}
	hunk.OldStart, hunk.OldCount = parseRange(strings.TrimPrefix(parts[0], "-"))
	hunk.NewStart, hunk.NewCount = parseRange(strings.TrimPrefix(parts[1], "+"))
	return hunk
}

// parseRange は "start,count" または "start" をパースする
func parseRange(s string) (int, int) {
	parts := strings.SplitN(s, ",", 2)
	start, _ := strconv.Atoi(parts[0])
	count := 1
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	}
	return start, count
}

// parseLogOutput は git log の出力をパースする
func parseLogOutput(output string) []LogEntry {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var entries []LogEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) < 4 {
			continue
		}
		date, _ := time.Parse(time.RFC3339, parts[2])
		entries = append(entries, LogEntry{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    date,
			Message: parts[3],
		})
	}
	return entries
}

// parseBlameOutput は git blame --porcelain の出力をパースする
func parseBlameOutput(output string) []BlameLine {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var lines []BlameLine
	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentHash, currentAuthor, currentDate string
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()

		// ハッシュ行
		if len(line) >= 40 && isHex(line[0]) {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				currentHash = parts[0][:8]
				lineNum, _ = strconv.Atoi(parts[2])
			}
			continue
		}

		if strings.HasPrefix(line, "author ") {
			currentAuthor = strings.TrimPrefix(line, "author ")
			continue
		}
		if strings.HasPrefix(line, "author-time ") {
			ts, _ := strconv.ParseInt(strings.TrimPrefix(line, "author-time "), 10, 64)
			if ts > 0 {
				currentDate = time.Unix(ts, 0).Format("2006-01-02")
			}
			continue
		}

		// コンテンツ行（タブで始まる）
		if strings.HasPrefix(line, "\t") {
			lines = append(lines, BlameLine{
				Hash:    currentHash,
				Author:  currentAuthor,
				Date:    currentDate,
				LineNum: lineNum,
				Content: line[1:],
			})
		}
	}
	return lines
}

// parseStashOutput は git stash list の出力をパースする
func parseStashOutput(output string) []StashEntry {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var entries []StashEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "\x00", 2)
		if len(parts) < 2 {
			continue
		}
		idx := 0
		ref := parts[0]
		if start := strings.Index(ref, "{"); start >= 0 {
			if end := strings.Index(ref, "}"); end > start {
				idx, _ = strconv.Atoi(ref[start+1 : end])
			}
		}
		entries = append(entries, StashEntry{
			Index:   idx,
			Message: parts[1],
		})
	}
	return entries
}

// isHex は文字が16進数かどうかを判定する
func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// StatusLabel はFileStatusの表示文字列を返す
func StatusLabel(s FileStatus) string {
	switch s {
	case StatusModified:
		return "M"
	case StatusAdded, StatusStagedNew:
		return "A"
	case StatusDeleted, StatusStagedDeleted:
		return "D"
	case StatusRenamed:
		return "R"
	case StatusUntracked:
		return "U"
	case StatusConflicted:
		return "C"
	case StatusStaged:
		return "M"
	default:
		return ""
	}
}

// FullPath はワークスペースルートとエントリパスからフルパスを返す
func (e *FileEntry) FullPath(workspace string) string {
	return filepath.Join(workspace, e.Path)
}
