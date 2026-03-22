package lsp

import (
	"log"
	"os/exec"
	"sync"
)

// LanguageServer は言語ごとのLSPサーバー設定
type LanguageServer struct {
	Command   string   // 実行コマンド
	Args      []string // コマンド引数
	Languages []string // 対応言語
}

// デフォルトのLSPサーバー設定
var defaultServers = []LanguageServer{
	{Command: "gopls", Args: []string{}, Languages: []string{"Go"}},
	{Command: "rust-analyzer", Args: []string{}, Languages: []string{"Rust"}},
	{Command: "pylsp", Args: []string{}, Languages: []string{"Python"}},
	{Command: "typescript-language-server", Args: []string{"--stdio"}, Languages: []string{"TypeScript", "JavaScript"}},
}

// Manager は複数のLSPクライアントを管理する
type Manager struct {
	mu       sync.RWMutex
	clients  map[string]*Client // 言語名 -> クライアント
	rootPath string
	configs  []LanguageServer

	// 診断情報の統合管理
	diagMu      sync.RWMutex
	diagnostics map[string][]Diagnostic // URI -> 診断リスト
	diagVersion int                     // 変更検出用バージョン
}

// NewManager は新しいLSPマネージャーを作成する
func NewManager(rootPath string) *Manager {
	return &Manager{
		clients:     make(map[string]*Client),
		rootPath:    rootPath,
		configs:     defaultServers,
		diagnostics: make(map[string][]Diagnostic),
	}
}

// StartForLanguage は指定言語のLSPサーバーを起動する
func (m *Manager) StartForLanguage(language string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 既に起動中なら何もしない
	if client, ok := m.clients[language]; ok {
		if client.IsReady() {
			return nil
		}
		// 停止状態なら再起動
		client.Stop()
	}

	// 言語に対応するサーバーを検索
	var config *LanguageServer
	for i := range m.configs {
		for _, lang := range m.configs[i].Languages {
			if lang == language {
				config = &m.configs[i]
				break
			}
		}
		if config != nil {
			break
		}
	}
	if config == nil {
		return nil // 対応サーバーなし（エラーにはしない）
	}

	// コマンドが存在するか確認
	if _, err := exec.LookPath(config.Command); err != nil {
		log.Printf("[LSP] %s が見つかりません: %v", config.Command, err)
		return nil // エラーにはしない
	}

	client := NewClient()

	// 診断ハンドラー設定
	client.SetDiagnosticsHandler(func(params PublishDiagnosticsParams) {
		m.diagMu.Lock()
		m.diagnostics[params.URI] = params.Diagnostics
		m.diagVersion++
		m.diagMu.Unlock()
	})

	// サーバー起動（バックグラウンド）
	go func() {
		if err := client.Start(config.Command, config.Args, m.rootPath); err != nil {
			log.Printf("[LSP] %s 起動失敗: %v", config.Command, err)
		}
	}()

	m.clients[language] = client
	return nil
}

// ClientForLanguage は指定言語のクライアントを返す
func (m *Manager) ClientForLanguage(language string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[language]
}

// GetDiagnostics は指定URIの診断情報を返す
func (m *Manager) GetDiagnostics(uri string) []Diagnostic {
	m.diagMu.RLock()
	defer m.diagMu.RUnlock()
	return m.diagnostics[uri]
}

// GetAllDiagnostics は全URIの診断情報を返す
func (m *Manager) GetAllDiagnostics() map[string][]Diagnostic {
	m.diagMu.RLock()
	defer m.diagMu.RUnlock()
	result := make(map[string][]Diagnostic, len(m.diagnostics))
	for k, v := range m.diagnostics {
		result[k] = v
	}
	return result
}

// DiagnosticSummary はエラー数と警告数を返す
func (m *Manager) DiagnosticSummary() (errors, warnings int) {
	m.diagMu.RLock()
	defer m.diagMu.RUnlock()
	for _, diags := range m.diagnostics {
		for _, d := range diags {
			switch d.Severity {
			case SeverityError:
				errors++
			case SeverityWarning:
				warnings++
			}
		}
	}
	return
}

// DiagVersion は診断情報の変更バージョンを返す
func (m *Manager) DiagVersion() int {
	m.diagMu.RLock()
	defer m.diagMu.RUnlock()
	return m.diagVersion
}

// LatencyMs は現在アクティブなLSPクライアントの最新レイテンシをミリ秒で返す
func (m *Manager) LatencyMs() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// アクティブなクライアントの最大レイテンシを返す
	var maxLatency int64
	for _, c := range m.clients {
		if c.IsReady() {
			lat := c.LastLatency.Load()
			if lat > maxLatency {
				maxLatency = lat
			}
		}
	}
	return maxLatency
}

// StopAll は全LSPサーバーを停止する
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for lang, client := range m.clients {
		client.Stop()
		delete(m.clients, lang)
	}
}

// NotifyDidOpen はドキュメントを開いた通知を適切なクライアントに送信する
func (m *Manager) NotifyDidOpen(uri, language, text string) {
	client := m.ClientForLanguage(language)
	if client == nil {
		// サーバーが未起動なら自動起動を試行
		_ = m.StartForLanguage(language)
		client = m.ClientForLanguage(language)
	}
	if client != nil {
		langID := languageToID(language)
		client.DidOpen(uri, langID, text)
	}
}

// NotifyDidChange はドキュメント変更通知を送信する
func (m *Manager) NotifyDidChange(uri, language, text string) {
	client := m.ClientForLanguage(language)
	if client != nil {
		client.DidChange(uri, text)
	}
}

// NotifyDidSave は保存通知を送信する
func (m *Manager) NotifyDidSave(uri, language string) {
	client := m.ClientForLanguage(language)
	if client != nil {
		client.DidSave(uri)
	}
}

// NotifyDidClose はドキュメントを閉じた通知を送信する
func (m *Manager) NotifyDidClose(uri, language string) {
	client := m.ClientForLanguage(language)
	if client != nil {
		client.DidClose(uri)
	}
}

// languageToID はCatanaの言語名をLSP言語IDに変換する
func languageToID(language string) string {
	switch language {
	case "Go":
		return "go"
	case "Rust":
		return "rust"
	case "Python":
		return "python"
	case "TypeScript":
		return "typescript"
	case "JavaScript":
		return "javascript"
	default:
		return "plaintext"
	}
}
