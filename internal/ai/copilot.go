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

// CopilotProvider はGitHub Copilot互換APIプロバイダ
// Copilot SDKのHTTPエンドポイントを利用する
type CopilotProvider struct {
	token    string
	model    string
	endpoint string
	client   *http.Client
}

// NewCopilotProvider は新しいCopilotプロバイダを作成する
func NewCopilotProvider(token, endpoint string) *CopilotProvider {
	if endpoint == "" {
		endpoint = "https://api.githubcopilot.com"
	}
	return &CopilotProvider{
		token:    token,
		model:    "gpt-4.1",
		endpoint: endpoint,
		client:   &http.Client{},
	}
}

func (p *CopilotProvider) Name() string { return "Copilot" }

func (p *CopilotProvider) IsConfigured() bool {
	return p.token != ""
}

func (p *CopilotProvider) CurrentModel() string { return p.model }

func (p *CopilotProvider) SetModel(model string) { p.model = model }

func (p *CopilotProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	// GitHub Copilot利用可能モデル（2026年3月時点）
	return []ModelInfo{
		{ID: "gpt-4.1", Name: "GPT-4.1"},
		{ID: "gpt-5-mini", Name: "GPT-5 mini"},
		{ID: "gpt-5.1", Name: "GPT-5.1"},
		{ID: "gpt-5.2", Name: "GPT-5.2"},
		{ID: "gpt-5.4", Name: "GPT-5.4"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini"},
		{ID: "claude-opus-4.6", Name: "Claude Opus 4.6"},
		{ID: "claude-sonnet-4.6", Name: "Claude Sonnet 4.6"},
		{ID: "claude-sonnet-4.5", Name: "Claude Sonnet 4.5"},
		{ID: "claude-haiku-4.5", Name: "Claude Haiku 4.5"},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro"},
		{ID: "gemini-3-flash", Name: "Gemini 3 Flash"},
		{ID: "gemini-3-pro", Name: "Gemini 3 Pro"},
	}, nil
}

// copilotChatRequest はCopilot Chat APIのリクエスト形式
type copilotChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
}

func (p *CopilotProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Copilotトークンが設定されていません")
	}

	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{Role: string(m.Role), Content: m.Content}
	}

	body := copilotChatRequest{
		Model:    p.model,
		Messages: messages,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.token)
	httpReq.Header.Set("Editor-Version", "Catana/1.0")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Copilot APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("レスポンスのデコード失敗: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("レスポンスにchoicesがありません")
	}

	return &ChatResponse{
		Content:      result.Choices[0].Message.Content,
		FinishReason: result.Choices[0].FinishReason,
		TokensUsed:   result.Usage.TotalTokens,
	}, nil
}

func (p *CopilotProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Copilotトークンが設定されていません")
	}

	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{Role: string(m.Role), Content: m.Content}
	}

	body := copilotChatRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   true,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.token)
	httpReq.Header.Set("Editor-Version", "Catana/1.0")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Copilot APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamDelta, 32)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.readSSEStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

func (p *CopilotProvider) readSSEStream(ctx context.Context, r io.Reader, ch chan<- StreamDelta) {
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
		if data == "[DONE]" {
			ch <- StreamDelta{Done: true}
			return
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			ch <- StreamDelta{Content: chunk.Choices[0].Delta.Content}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamDelta{Done: true, Error: err}
	}
}

func (p *CopilotProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Copilotトークンが設定されていません")
	}

	systemPrompt := "あなたはコード補完アシスタントです。カーソル位置に続くコードのみを出力してください。説明やマークダウンは不要です。"
	userPrompt := fmt.Sprintf("以下の%s コードの続きを補完してください:\n\n```\n%s\n```\n\nカーソル後:\n```\n%s\n```", req.Language, req.Prefix, req.Suffix)

	chatReq := &ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: systemPrompt},
			{Role: RoleUser, Content: userPrompt},
		},
		MaxTokens:   128,
		Temperature: 0.2,
	}

	resp, err := p.Chat(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	return &CompletionResponse{Text: resp.Content}, nil
}
