package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicProvider はAnthropic Claude APIプロバイダ
type AnthropicProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewAnthropicProvider は新しいAnthropicプロバイダを作成する
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &AnthropicProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: "https://api.anthropic.com/v1",
		client:   &http.Client{},
	}
}

func (p *AnthropicProvider) Name() string { return "Anthropic" }

func (p *AnthropicProvider) IsConfigured() bool {
	return p.apiKey != ""
}

func (p *AnthropicProvider) CurrentModel() string { return p.model }

func (p *AnthropicProvider) SetModel(model string) { p.model = model }

func (p *AnthropicProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	// Anthropicは公開モデル一覧APIがないため、主要モデルの静的リストを返す
	// 2026年3月時点の最新モデル
	return []ModelInfo{
		{ID: "claude-opus-4-6", Name: "Claude Opus 4.6"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5"},
		{ID: "claude-opus-4-5", Name: "Claude Opus 4.5"},
		{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5"},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4"},
	}, nil
}

// anthropicMessage はAnthropic APIのメッセージ形式
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicChatRequest はAnthropic Messages APIのリクエスト形式
type anthropicChatRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

// anthropicChatResponse はAnthropic Messages APIのレスポンス形式
type anthropicChatResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// anthropicStreamEvent はストリーミングイベント
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Anthropic APIキーが設定されていません")
	}

	// systemメッセージを分離
	var systemPrompt string
	var messages []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
			continue
		}
		messages = append(messages, anthropicMessage{Role: string(m.Role), Content: m.Content})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := anthropicChatRequest{
		Model:     p.model,
		Messages:  messages,
		MaxTokens: maxTokens,
		System:    systemPrompt,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result anthropicChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("レスポンスのデコード失敗: %w", err)
	}

	var content string
	for _, c := range result.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &ChatResponse{
		Content:      content,
		FinishReason: result.StopReason,
		TokensUsed:   result.Usage.InputTokens + result.Usage.OutputTokens,
	}, nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Anthropic APIキーが設定されていません")
	}

	var systemPrompt string
	var messages []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
			continue
		}
		messages = append(messages, anthropicMessage{Role: string(m.Role), Content: m.Content})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := anthropicChatRequest{
		Model:     p.model,
		Messages:  messages,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Stream:    true,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Anthropic APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamDelta, 32)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.readSSEStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

func (p *AnthropicProvider) readSSEStream(ctx context.Context, r io.Reader, ch chan<- StreamDelta) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamDelta{Done: true, Error: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Text != "" {
				ch <- StreamDelta{Content: event.Delta.Text}
			}
		case "message_stop":
			ch <- StreamDelta{Done: true}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamDelta{Done: true, Error: err}
	}
}

func (p *AnthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Anthropic APIキーが設定されていません")
	}

	systemPrompt := "あなたはコード補完アシスタントです。カーソル位置に続くコードのみを出力してください。説明やマークダウンは不要です。"
	userPrompt := fmt.Sprintf("以下の%s コードの続きを補完してください:\n\n```\n%s\n```\n\nカーソル後:\n```\n%s\n```", req.Language, req.Prefix, req.Suffix)

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 128
	}

	chatReq := &ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: systemPrompt},
			{Role: RoleUser, Content: userPrompt},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.2,
	}

	resp, err := p.Chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	return &CompletionResponse{Text: resp.Content}, nil
}
