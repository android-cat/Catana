package editor

import (
	"catana/internal/core"
	"catana/internal/dap"
	"catana/internal/git"
	"catana/internal/lsp"
	"catana/internal/syntax"
	"catana/internal/terminal"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SidebarTab はサイドバーのアクティブタブ
type SidebarTab int

const (
	TabFiles SidebarTab = iota
	TabSearch
	TabGit
	TabExtensions
)

// OmniMode はオムニバーのモード
type OmniMode int

const (
	ModeAI OmniMode = iota
	ModeCmd
	ModeTerm
)

// Document は1つの開いたファイルを表す
type Document struct {
	FilePath         string
	FileName         string
	Buffer           *core.Buffer
	Modified         bool
	Language         string
	HighlightedLines [][]syntax.Span // 行ごとのハイライト済みスパン
	TSTree           *syntax.TSTree  // Tree-sitterパースツリー
	IsDiff           bool            // Diff表示タブ
	DiffData         []git.FileDiff  // Diffデータ
	DiffStaged       bool            // ステージ済みdiff
}

// TermHistoryEntry はTERMモードのコマンド履歴エントリ
type TermHistoryEntry struct {
	Command string // 実行したコマンド
	Output  string // コマンド出力
	Dir     string // 実行ディレクトリ
	Running bool   // 実行中フラグ
	Done    bool   // 完了フラグ
	ExitOK  bool   // 正常終了フラグ
}

// EditorState はエディタ全体の状態を管理する
type EditorState struct {
	Workspace           string
	Documents           []*Document
	ActiveDocIdx        int
	SidebarOpen         bool
	SidebarTab          SidebarTab
	OmniMode            OmniMode
	ShowOmniChat        bool
	Highlighter         *syntax.Highlighter
	Search              *SearchState
	ScrollCenterRequest bool // 次のスクロールでカーソルを中央に配置する

	// LSP統合
	LSP *lsp.Manager

	// DAP統合
	DAP         *dap.Client
	DebugOutput []string // デバッグ出力ログ

	// ターミナル統合
	Terminal             *terminal.Manager
	ShowTerminal         bool                // ターミナルパネル表示状態
	TerminalFocusRequest bool                // ターミナルへのフォーカス移動要求
	TermHistory          []*TermHistoryEntry // TERMモードコマンド履歴
	TermShell            *TermShell          // TERMモードの永続シェル

	// Git統合
	Git            *git.Repository            // Gitリポジトリ操作
	GitBranch      string                     // 現在のブランチ名
	GitStatus      []git.FileEntry            // ワーキングツリーのステータス
	GitStatusErr   error                      // ステータス取得エラー
	GitCommitMsg   string                     // コミットメッセージ入力
	GitStashList   []git.StashEntry           // stash一覧
	GitLog         []git.LogEntry             // コミット履歴
	GitDiffCache   map[string][]git.FileDiff  // ファイルパス→diff キャッシュ
	GitBlameCache  map[string][]git.BlameLine // ファイルパス→blame キャッシュ
	gitMu          sync.Mutex                 // Git操作の排他制御
	gitLastRefresh time.Time                  // 最後のステータス更新時刻
}

// NewEditorState は初期状態のエディタステートを作成する
func NewEditorState(workspace string) *EditorState {
	repo := git.NewRepository(workspace)
	s := &EditorState{
		Workspace:     workspace,
		Documents:     make([]*Document, 0),
		ActiveDocIdx:  -1,
		SidebarOpen:   true,
		SidebarTab:    TabFiles,
		OmniMode:      ModeAI,
		ShowOmniChat:  true,
		Highlighter:   syntax.NewHighlighter(),
		Search:        NewSearchState(),
		LSP:           lsp.NewManager(workspace),
		DAP:           dap.NewClient(),
		DebugOutput:   make([]string, 0),
		Terminal:      terminal.NewManager(workspace),
		Git:           repo,
		GitDiffCache:  make(map[string][]git.FileDiff),
		GitBlameCache: make(map[string][]git.BlameLine),
	}
	// 初回のGitステータス取得
	s.RefreshGitStatus()
	return s
}

// SetWorkspace はワークスペースを変更し、Git等を再初期化する
func (s *EditorState) SetWorkspace(path string) {
	s.Workspace = path
	s.Git = git.NewRepository(path)
	s.GitBranch = ""
	s.GitStatus = nil
	s.GitStatusErr = nil
	s.GitDiffCache = make(map[string][]git.FileDiff)
	s.GitBlameCache = make(map[string][]git.BlameLine)
	s.GitStashList = nil
	s.GitLog = nil
	s.RefreshGitStatus()
}

// EnsureTermShell は永続シェルが起動していなければ起動する
func (s *EditorState) EnsureTermShell() error {
	if s.TermShell != nil && s.TermShell.IsAlive() {
		return nil
	}
	shell, err := NewTermShell(s.Workspace)
	if err != nil {
		return err
	}
	s.TermShell = shell
	return nil
}

// ClearTermHistory はTERMモードの履歴をクリアし、シェルを再起動する
func (s *EditorState) ClearTermHistory() {
	s.TermHistory = nil
	if s.TermShell != nil {
		s.TermShell.Close()
		s.TermShell = nil
	}
}

// isBinary はデータにNULバイトが含まれるかでバイナリ判定する
func isBinary(data []byte) bool {
	// 先頭8KBをチェック
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}

// OpenFile はファイルを開いて新しいタブに追加する
func (s *EditorState) OpenFile(path string) error {
	// 既に開いているファイルがあればそのタブに切替
	for i, doc := range s.Documents {
		if doc.FilePath == path {
			s.ActiveDocIdx = i
			return nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if isBinary(data) {
		return fmt.Errorf("バイナリファイルは開けません: %s", filepath.Base(path))
	}

	content := string(data)
	fileName := filepath.Base(path)
	lang := s.Highlighter.DetectLanguage(fileName)

	doc := &Document{
		FilePath: path,
		FileName: fileName,
		Buffer:   core.NewBuffer(content),
		Modified: false,
		Language: lang,
	}
	doc.UpdateHighlight(s.Highlighter)

	s.Documents = append(s.Documents, doc)
	s.ActiveDocIdx = len(s.Documents) - 1

	// LSP通知: ドキュメントを開いた
	if s.LSP != nil {
		uri := lsp.FilePathToURI(path)
		s.LSP.NotifyDidOpen(uri, lang, content)
	}

	return nil
}

// OpenNewBuffer は新しい空のバッファを開く
func (s *EditorState) OpenNewBuffer(name string, content string) {
	lang := s.Highlighter.DetectLanguage(name)
	doc := &Document{
		FilePath: "",
		FileName: name,
		Buffer:   core.NewBuffer(content),
		Modified: false,
		Language: lang,
	}
	doc.UpdateHighlight(s.Highlighter)
	s.Documents = append(s.Documents, doc)
	s.ActiveDocIdx = len(s.Documents) - 1
}

// OpenDiffTab はGit diffをタブとして表示する
func (s *EditorState) OpenDiffTab(path string, staged bool) {
	diffs := s.GitGetDiff(path, staged)
	tabName := filepath.Base(path) + " (diff)"
	if staged {
		tabName = filepath.Base(path) + " (staged diff)"
	}

	// 既に開いているdiffタブがあればそのタブに切替
	for i, doc := range s.Documents {
		if doc.IsDiff && doc.FileName == tabName {
			// diffデータを更新
			doc.DiffData = diffs
			s.ActiveDocIdx = i
			return
		}
	}

	doc := &Document{
		FileName:   tabName,
		Buffer:     core.NewBuffer(""),
		IsDiff:     true,
		DiffData:   diffs,
		DiffStaged: staged,
	}
	s.Documents = append(s.Documents, doc)
	s.ActiveDocIdx = len(s.Documents) - 1
}

// SaveActiveFile はアクティブなドキュメントを保存する
func (s *EditorState) SaveActiveFile() error {
	doc := s.ActiveDocument()
	if doc == nil || doc.FilePath == "" {
		return nil
	}
	content := doc.Buffer.Text()
	err := os.WriteFile(doc.FilePath, []byte(content), 0644)
	if err != nil {
		return err
	}
	doc.Modified = false

	// LSP通知: 保存
	if s.LSP != nil && doc.Language != "" {
		uri := lsp.FilePathToURI(doc.FilePath)
		s.LSP.NotifyDidSave(uri, doc.Language)
	}

	return nil
}

// MovePath はファイルまたはフォルダを別ディレクトリへ移動し、開いているドキュメントのパスも追従させる
func (s *EditorState) MovePath(srcPath string, dstDir string) (string, error) {
	srcPath = filepath.Clean(srcPath)
	dstDir = filepath.Clean(dstDir)
	if srcPath == "" || dstDir == "" {
		return "", fmt.Errorf("移動元または移動先が不正です")
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}
	dstInfo, err := os.Stat(dstDir)
	if err != nil {
		return "", err
	}
	if !dstInfo.IsDir() {
		return "", fmt.Errorf("移動先はフォルダである必要があります: %s", dstDir)
	}
	if srcInfo.IsDir() && pathWithinBase(srcPath, dstDir) {
		return "", fmt.Errorf("フォルダを自身の配下へ移動できません")
	}

	dstPath := filepath.Join(dstDir, filepath.Base(srcPath))
	if filepath.Clean(dstPath) == srcPath {
		return dstPath, nil
	}
	if _, err := os.Stat(dstPath); err == nil {
		return "", fmt.Errorf("同名のパスが既に存在します: %s", dstPath)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		return "", err
	}
	s.remapDocumentPaths(srcPath, dstPath, srcInfo.IsDir())
	return dstPath, nil
}

// RenamePath はファイルまたはフォルダ名を変更し、開いているドキュメントのパスも追従させる
func (s *EditorState) RenamePath(oldPath string, newName string) (string, error) {
	oldPath = filepath.Clean(oldPath)
	newName = strings.TrimSpace(newName)
	if oldPath == "" || newName == "" {
		return "", fmt.Errorf("変更前または変更後の名前が不正です")
	}
	if strings.Contains(newName, string(filepath.Separator)) {
		return "", fmt.Errorf("名前にパス区切りは含められません")
	}
	info, err := os.Stat(oldPath)
	if err != nil {
		return "", err
	}
	newPath := filepath.Join(filepath.Dir(oldPath), newName)
	if filepath.Clean(newPath) == oldPath {
		return newPath, nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return "", fmt.Errorf("同名のパスが既に存在します: %s", newPath)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", err
	}
	s.remapDocumentPaths(oldPath, newPath, info.IsDir())
	return newPath, nil
}

// DeletePath はファイルまたはフォルダを削除し、開いている関連ドキュメントを閉じる
func (s *EditorState) DeletePath(targetPath string) error {
	targetPath = filepath.Clean(targetPath)
	if targetPath == "" {
		return fmt.Errorf("削除対象が不正です")
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.RemoveAll(targetPath); err != nil {
			return err
		}
	} else {
		if err := os.Remove(targetPath); err != nil {
			return err
		}
	}
	s.removeDocumentsForPath(targetPath, info.IsDir())
	return nil
}

// CloseTab は指定タブを閉じる
func (s *EditorState) CloseTab(idx int) {
	if idx < 0 || idx >= len(s.Documents) {
		return
	}
	doc := s.Documents[idx]

	// LSP通知: ドキュメントを閉じた
	if s.LSP != nil && doc.FilePath != "" && doc.Language != "" {
		uri := lsp.FilePathToURI(doc.FilePath)
		s.LSP.NotifyDidClose(uri, doc.Language)
	}

	s.Documents = append(s.Documents[:idx], s.Documents[idx+1:]...)
	if s.ActiveDocIdx >= len(s.Documents) {
		s.ActiveDocIdx = len(s.Documents) - 1
	}
	if s.ActiveDocIdx < 0 && len(s.Documents) > 0 {
		s.ActiveDocIdx = 0
	}
}

// ActiveDocument はアクティブなドキュメントを返す
func (s *EditorState) ActiveDocument() *Document {
	if s.ActiveDocIdx < 0 || s.ActiveDocIdx >= len(s.Documents) {
		return nil
	}
	return s.Documents[s.ActiveDocIdx]
}

// ToggleSidebar はサイドバーの表示/非表示を切り替える
func (s *EditorState) ToggleSidebar() {
	s.SidebarOpen = !s.SidebarOpen
}

// SetSidebarTab はサイドバーのタブを設定する
func (s *EditorState) SetSidebarTab(tab SidebarTab) {
	if s.SidebarTab == tab && s.SidebarOpen {
		s.SidebarOpen = false
	} else {
		s.SidebarTab = tab
		s.SidebarOpen = true
	}
}

// MarkModified はアクティブドキュメントを変更済みとしてマークする
func (s *EditorState) MarkModified() {
	doc := s.ActiveDocument()
	if doc != nil {
		doc.Modified = true
	}
}

// NotifyDidChange はアクティブドキュメントの変更をLSPに通知する
func (s *EditorState) NotifyDidChange() {
	doc := s.ActiveDocument()
	if doc == nil || doc.FilePath == "" || doc.Language == "" {
		return
	}
	if s.LSP != nil {
		uri := lsp.FilePathToURI(doc.FilePath)
		s.LSP.NotifyDidChange(uri, doc.Language, doc.Buffer.Text())
	}
}

// Cleanup はエディタのリソースを解放する
func (s *EditorState) Cleanup() {
	if s.LSP != nil {
		s.LSP.StopAll()
	}
	if s.DAP != nil {
		s.DAP.Stop()
	}
	if s.Terminal != nil {
		s.Terminal.CloseAll()
	}
}

// RelativePath はワークスペースからの相対パスを返す
func (s *EditorState) RelativePath(absPath string) string {
	rel, err := filepath.Rel(s.Workspace, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

// BreadcrumbParts はブレッドクラム用のパス部品を返す
func (s *EditorState) BreadcrumbParts() []string {
	doc := s.ActiveDocument()
	if doc == nil {
		return nil
	}
	rel := s.RelativePath(doc.FilePath)
	if rel == "" || rel == "." {
		return []string{doc.FileName}
	}
	return strings.Split(rel, string(filepath.Separator))
}

// UpdateHighlight はドキュメントのシンタックスハイライトを更新する
func (d *Document) UpdateHighlight(h *syntax.Highlighter) {
	lineCount := d.Buffer.LineCount()

	// Tree-sitterが対応している場合はフルパースでハイライト
	// 注: tree.Edit()未実装のため、古いツリーを渡すと不正なバイト範囲になる
	if h.TS != nil && h.TS.Supports(d.Language) {
		source := []byte(d.Buffer.Text())
		d.TSTree = h.TS.Parse(source, d.Language, nil)
		if d.TSTree != nil {
			lines := h.TS.HighlightLines(d.TSTree, source, lineCount)
			if lines != nil {
				d.HighlightedLines = lines
				return
			}
		}
	}

	// フォールバック: キーワードベースのハイライト
	d.HighlightedLines = make([][]syntax.Span, lineCount)
	for i := 0; i < lineCount; i++ {
		line := d.Buffer.Line(i)
		d.HighlightedLines[i] = h.HighlightLine(line, d.Language)
	}
}

func (s *EditorState) remapDocumentPaths(oldPath string, newPath string, movedDir bool) {
	for _, doc := range s.Documents {
		if doc.FilePath == "" {
			continue
		}
		if movedDir {
			if !pathWithinBase(oldPath, doc.FilePath) {
				continue
			}
			rel, err := filepath.Rel(oldPath, doc.FilePath)
			if err != nil {
				continue
			}
			doc.FilePath = filepath.Join(newPath, rel)
			doc.FileName = filepath.Base(doc.FilePath)
			continue
		}
		if filepath.Clean(doc.FilePath) == oldPath {
			doc.FilePath = newPath
			doc.FileName = filepath.Base(newPath)
		}
	}
}

func (s *EditorState) removeDocumentsForPath(targetPath string, isDir bool) {
	filtered := make([]*Document, 0, len(s.Documents))
	newActive := -1
	activeDoc := s.ActiveDocument()
	for _, doc := range s.Documents {
		if doc.FilePath == "" {
			filtered = append(filtered, doc)
			continue
		}
		remove := filepath.Clean(doc.FilePath) == targetPath
		if isDir && pathWithinBase(targetPath, doc.FilePath) {
			remove = true
		}
		if remove {
			continue
		}
		filtered = append(filtered, doc)
		if activeDoc == doc {
			newActive = len(filtered) - 1
		}
	}
	s.Documents = filtered
	if len(s.Documents) == 0 {
		s.ActiveDocIdx = -1
		return
	}
	if newActive >= 0 {
		s.ActiveDocIdx = newActive
		return
	}
	if s.ActiveDocIdx >= len(s.Documents) {
		s.ActiveDocIdx = len(s.Documents) - 1
		return
	}
	if s.ActiveDocIdx < 0 {
		s.ActiveDocIdx = 0
	}
}

func pathWithinBase(base string, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// --- Git操作メソッド ---

// RefreshGitStatus はGitステータスを非同期で更新する
func (s *EditorState) RefreshGitStatus() {
	if s.Git == nil || !s.Git.IsGitRepo() {
		return
	}
	go func() {
		s.gitMu.Lock()
		defer s.gitMu.Unlock()

		s.GitBranch = s.Git.Branch()
		status, err := s.Git.Status()
		s.GitStatus = status
		s.GitStatusErr = err
		s.gitLastRefresh = time.Now()
	}()
}

// RefreshGitStatusIfNeeded はインターバル経過時にGitステータスを更新する
func (s *EditorState) RefreshGitStatusIfNeeded() {
	if s.Git == nil {
		return
	}
	s.gitMu.Lock()
	elapsed := time.Since(s.gitLastRefresh)
	s.gitMu.Unlock()
	if elapsed > 3*time.Second {
		s.RefreshGitStatus()
	}
}

// GitStage はファイルをステージに追加する
func (s *EditorState) GitStage(path string) error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.Stage(path); err != nil {
		return err
	}
	s.RefreshGitStatus()
	return nil
}

// GitUnstage はファイルをステージから外す
func (s *EditorState) GitUnstage(path string) error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.Unstage(path); err != nil {
		return err
	}
	s.RefreshGitStatus()
	return nil
}

// GitStageAll は全ての変更をステージに追加する
func (s *EditorState) GitStageAll() error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.StageAll(); err != nil {
		return err
	}
	s.RefreshGitStatus()
	return nil
}

// GitUnstageAll は全てのステージ済み変更を戻す
func (s *EditorState) GitUnstageAll() error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.UnstageAll(); err != nil {
		return err
	}
	s.RefreshGitStatus()
	return nil
}

// GitCommit はステージ済みの変更をコミットする
func (s *EditorState) GitCommit() error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	msg := strings.TrimSpace(s.GitCommitMsg)
	if msg == "" {
		return fmt.Errorf("コミットメッセージが空です")
	}
	if err := s.Git.Commit(msg); err != nil {
		return err
	}
	s.GitCommitMsg = ""
	s.RefreshGitStatus()
	return nil
}

// GitDiscardChanges はファイルの変更を破棄する
func (s *EditorState) GitDiscardChanges(path string) error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.DiscardChanges(path); err != nil {
		return err
	}
	// 開いているバッファがあれば再読み込み
	fullPath := filepath.Join(s.Workspace, path)
	for _, doc := range s.Documents {
		if doc.FilePath == fullPath {
			data, err := os.ReadFile(fullPath)
			if err == nil {
				doc.Buffer = core.NewBuffer(string(data))
				doc.Modified = false
				doc.UpdateHighlight(s.Highlighter)
			}
			break
		}
	}
	s.RefreshGitStatus()
	return nil
}

// GitStash は現在の変更をstashする
func (s *EditorState) GitStash(message string) error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.Stash(message); err != nil {
		return err
	}
	s.RefreshGitStatus()
	s.RefreshGitStashList()
	return nil
}

// GitStashPop はstashから最新の変更を復元する
func (s *EditorState) GitStashPop() error {
	if s.Git == nil {
		return fmt.Errorf("Gitリポジトリではありません")
	}
	if err := s.Git.StashPop(); err != nil {
		return err
	}
	s.RefreshGitStatus()
	s.RefreshGitStashList()
	return nil
}

// RefreshGitStashList はstash一覧を更新する
func (s *EditorState) RefreshGitStashList() {
	if s.Git == nil {
		return
	}
	list, _ := s.Git.StashList()
	s.GitStashList = list
}

// RefreshGitLog はコミット履歴を更新する
func (s *EditorState) RefreshGitLog() {
	if s.Git == nil {
		return
	}
	entries, _ := s.Git.Log(100)
	s.GitLog = entries
}

// GitGetDiff はファイルのdiffを取得する（キャッシュ付き）
func (s *EditorState) GitGetDiff(path string, staged bool) []git.FileDiff {
	if s.Git == nil {
		return nil
	}
	key := path
	if staged {
		key = "staged:" + path
	}
	if cached, ok := s.GitDiffCache[key]; ok {
		return cached
	}
	var diffs []git.FileDiff
	var err error
	if staged {
		diffs, err = s.Git.DiffStaged(path)
	} else {
		diffs, err = s.Git.Diff(path)
	}
	if err != nil {
		return nil
	}
	s.GitDiffCache[key] = diffs
	return diffs
}

// GitClearDiffCache はdiffキャッシュをクリアする
func (s *EditorState) GitClearDiffCache() {
	s.GitDiffCache = make(map[string][]git.FileDiff)
	s.GitBlameCache = make(map[string][]git.BlameLine)
}

// GitGetBlame はファイルのblameを取得する（キャッシュ付き）
func (s *EditorState) GitGetBlame(path string) []git.BlameLine {
	if s.Git == nil {
		return nil
	}
	if cached, ok := s.GitBlameCache[path]; ok {
		return cached
	}
	lines, err := s.Git.Blame(path)
	if err != nil {
		return nil
	}
	s.GitBlameCache[path] = lines
	return lines
}

// HasGitStagedChanges はステージ済みの変更があるかを返す
func (s *EditorState) HasGitStagedChanges() bool {
	for _, entry := range s.GitStatus {
		if entry.Staged != git.StatusUnmodified {
			return true
		}
	}
	return false
}

// GitStagedFiles はステージ済みのファイル数を返す
func (s *EditorState) GitStagedFiles() int {
	count := 0
	for _, entry := range s.GitStatus {
		if entry.Staged != git.StatusUnmodified {
			count++
		}
	}
	return count
}

// GitUnstagedFiles は未ステージのファイル数を返す
func (s *EditorState) GitUnstagedFiles() int {
	count := 0
	for _, entry := range s.GitStatus {
		if entry.Status != git.StatusUnmodified {
			count++
		}
	}
	return count
}
