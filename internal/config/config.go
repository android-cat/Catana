package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ─── 設定ファイルパス ───

const (
	configDirName  = ".catana"
	configFileName = "config.json"
)

// configPath は設定ファイルのフルパスを返す
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName, configFileName), nil
}

// ─── 設定構造体 ───

// Config はアプリケーション全体の永続設定
type Config struct {
	mu sync.RWMutex `json:"-"`

	// 一般設定
	General GeneralConfig `json:"general"`
	// AIプロバイダ設定
	AI AIConfig `json:"ai"`
}

// GeneralConfig は一般設定
type GeneralConfig struct {
	FontSize    int    `json:"fontSize"`    // エディタのフォントサイズ (sp)
	TabSize     int    `json:"tabSize"`     // タブ幅（スペース数）
	DarkMode    bool   `json:"darkMode"`    // ダークモード
	WordWrap    bool   `json:"wordWrap"`    // 折り返し表示
	Minimap     bool   `json:"minimap"`     // ミニマップ表示
	LineNumbers bool   `json:"lineNumbers"` // 行番号表示
	AutoSave    bool   `json:"autoSave"`    // 自動保存
	Shell       string `json:"shell"`       // ターミナルのシェル
}

// AIProviderConfig は個別のAIプロバイダ設定
type AIProviderConfig struct {
	APIKey   string `json:"apiKey,omitempty"`
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// AIConfig はAI統合設定
type AIConfig struct {
	ActiveProvider string                      `json:"activeProvider"` // "openai" / "anthropic" / "copilot" / "ollama" / "gemini"
	ActiveModel    string                      `json:"activeModel"`    // 現在選択中のモデルID
	Providers      map[string]AIProviderConfig `json:"providers"`
}

// ─── デフォルト設定 ───

// DefaultConfig はデフォルト設定を返す
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			FontSize:    14,
			TabSize:     4,
			DarkMode:    true,
			WordWrap:    false,
			Minimap:     true,
			LineNumbers: true,
			AutoSave:    false,
			Shell:       defaultShell(),
		},
		AI: AIConfig{
			ActiveProvider: "ollama",
			Providers: map[string]AIProviderConfig{
				"openai":    {Model: "gpt-4.1"},
				"anthropic": {Model: "claude-sonnet-4-6"},
				"copilot":   {Endpoint: "https://api.githubcopilot.com"},
				"ollama":    {Model: "codellama", Endpoint: "http://localhost:11434"},
				"gemini":    {Model: "gemini-2.5-flash"},
			},
		},
	}
}

func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/zsh"
}

// ─── 読込・保存 ───

// Load は設定ファイルを読み込む。ファイルが無ければデフォルト設定を返す。
func Load() *Config {
	cfg := DefaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return DefaultConfig()
	}
	// Providers マップが nil の場合デフォルトで埋める
	if cfg.AI.Providers == nil {
		cfg.AI.Providers = DefaultConfig().AI.Providers
	}
	return cfg
}

// Save は設定をファイルに永続化する
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ─── ヘルパー ───

// GetAIProvider は指定プロバイダの設定を取得する
func (c *Config) GetAIProvider(name string) AIProviderConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p, ok := c.AI.Providers[name]; ok {
		return p
	}
	return AIProviderConfig{}
}

// SetAIProvider はプロバイダ設定を更新する
func (c *Config) SetAIProvider(name string, pc AIProviderConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AI.Providers == nil {
		c.AI.Providers = make(map[string]AIProviderConfig)
	}
	c.AI.Providers[name] = pc
}
