// Package ai はAIプロバイダ抽象化レイヤーを提供する
package ai

import (
	"context"
	"fmt"
	"sync"
)

// Role はメッセージの送信者ロール
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message はチャットメッセージ
type Message struct {
	Role    Role   // メッセージ送信者
	Content string // メッセージ本文
}

// ChatRequest はチャット補完リクエスト
type ChatRequest struct {
	Messages    []Message // 会話履歴
	MaxTokens   int       // 最大生成トークン数（0=デフォルト）
	Temperature float64   // 生成温度 (0.0-2.0)
	Stream      bool      // ストリーミングレスポンスを使用するか
}

// ChatResponse はチャット補完レスポンス
type ChatResponse struct {
	Content      string // 生成されたテキスト
	FinishReason string // 終了理由（stop, length, etc.）
	TokensUsed   int    // 使用トークン数
}

// StreamDelta はストリーミングレスポンスの差分
type StreamDelta struct {
	Content string // 差分テキスト
	Done    bool   // ストリーム終了フラグ
	Error   error  // エラー（発生時のみ）
}

// CompletionRequest はインライン補完リクエスト
type CompletionRequest struct {
	Prefix      string // カーソル前のテキスト
	Suffix      string // カーソル後のテキスト
	Language    string // プログラミング言語
	MaxTokens   int    // 最大生成トークン数
	Temperature float64
	FilePath    string // ファイルパス（コンテキスト用）
}

// CompletionResponse はインライン補完レスポンス
type CompletionResponse struct {
	Text string // 補完テキスト
}

// ModelInfo はプロバイダが提供するモデルの情報
type ModelInfo struct {
	ID   string // モデルID（APIで使用する名前）
	Name string // 表示名
}

// Provider はAIプロバイダの共通インターフェース
type Provider interface {
	// Name はプロバイダ名を返す
	Name() string
	// Chat はチャット補完を実行する
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	// ChatStream はストリーミングチャット補完を実行する
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error)
	// Complete はインラインコード補完を実行する
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	// IsConfigured は設定済みかを返す
	IsConfigured() bool
	// ListModels は利用可能なモデル一覧を返す
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// SetModel は使用するモデルを変更する
	SetModel(model string)
	// CurrentModel は現在のモデルIDを返す
	CurrentModel() string
}

// ProviderType はプロバイダの種類
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderCopilot   ProviderType = "copilot"
	ProviderOllama    ProviderType = "ollama"
	ProviderGemini    ProviderType = "gemini"
)

// Config はプロバイダ設定
type Config struct {
	Type     ProviderType // プロバイダ種類
	APIKey   string       // APIキー（OpenAI/Anthropic）
	Model    string       // 使用モデル
	Endpoint string       // カスタムエンドポイント（Ollama等）
}

// Manager は複数のAIプロバイダを管理する
type Manager struct {
	mu        sync.RWMutex
	providers map[ProviderType]Provider
	active    ProviderType
}

// NewManager は新しいAIマネージャを作成する
func NewManager() *Manager {
	return &Manager{
		providers: make(map[ProviderType]Provider),
		active:    ProviderOllama, // デフォルトはローカルLLM
	}
}

// RegisterProvider はプロバイダを登録する
func (m *Manager) RegisterProvider(ptype ProviderType, provider Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[ptype] = provider
}

// SetActive はアクティブプロバイダを設定する
func (m *Manager) SetActive(ptype ProviderType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.providers[ptype]; !ok {
		return fmt.Errorf("プロバイダ %s は未登録です", ptype)
	}
	m.active = ptype
	return nil
}

// Active はアクティブプロバイダを返す
func (m *Manager) Active() Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[m.active]
}

// ActiveType はアクティブプロバイダの種類を返す
func (m *Manager) ActiveType() ProviderType {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// ConfiguredProviders は設定済みプロバイダの一覧を返す
func (m *Manager) ConfiguredProviders() []ProviderType {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []ProviderType
	for ptype, p := range m.providers {
		if p.IsConfigured() {
			result = append(result, ptype)
		}
	}
	return result
}

// ListAllModels は全設定済みプロバイダのモデル一覧を返す
func (m *Manager) ListAllModels(ctx context.Context) map[ProviderType][]ModelInfo {
	m.mu.RLock()
	providers := make(map[ProviderType]Provider)
	for pt, p := range m.providers {
		if p.IsConfigured() {
			providers[pt] = p
		}
	}
	m.mu.RUnlock()

	result := make(map[ProviderType][]ModelInfo)
	for pt, p := range providers {
		models, err := p.ListModels(ctx)
		if err != nil {
			continue
		}
		result[pt] = models
	}
	return result
}

// SetActiveModel はプロバイダとモデルを同時に設定する
func (m *Manager) SetActiveModel(ptype ProviderType, modelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.providers[ptype]
	if !ok {
		return fmt.Errorf("プロバイダ %s は未登録です", ptype)
	}
	m.active = ptype
	p.SetModel(modelID)
	return nil
}

// ActiveModel はアクティブプロバイダの現在のモデルIDを返す
func (m *Manager) ActiveModel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.providers[m.active]; ok {
		return p.CurrentModel()
	}
	return ""
}

// Provider は指定タイプのプロバイダを返す
func (m *Manager) Provider(ptype ProviderType) Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[ptype]
}

// Chat はアクティブプロバイダでチャットを実行する
func (m *Manager) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	p := m.Active()
	if p == nil {
		return nil, fmt.Errorf("AIプロバイダが設定されていません")
	}
	return p.Chat(ctx, req)
}

// ChatStream はアクティブプロバイダでストリーミングチャットを実行する
func (m *Manager) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error) {
	p := m.Active()
	if p == nil {
		return nil, fmt.Errorf("AIプロバイダが設定されていません")
	}
	return p.ChatStream(ctx, req)
}

// Complete はアクティブプロバイダでインライン補完を実行する
func (m *Manager) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	p := m.Active()
	if p == nil {
		return nil, fmt.Errorf("AIプロバイダが設定されていません")
	}
	return p.Complete(ctx, req)
}
