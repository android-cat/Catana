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

// OpenAIProvider はOpenAI APIプロバイダ
type OpenAIProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAIProvider は新しいOpenAIプロバイダを作成する
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4.1"
	}
	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: "https://api.openai.com/v1",
		client:   &http.Client{},
	}
}

func (p *OpenAIProvider) Name() string { return "OpenAI" }

func (p *OpenAIProvider) IsConfigured() bool {
	return p.apiKey != ""
}

func (p *OpenAIProvider) CurrentModel() string { return p.model }

func (p *OpenAIProvider) SetModel(model string) { p.model = model }

// openAIModelsResponse はOpenAI Models APIのレスポンス形式
type openAIModelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI APIキーが設定されていません")
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("モデル一覧の取得失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI APIエラー (HTTP %d)", resp.StatusCode)
	}

	var result openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Data {
		// チャット対応モデルのみフィルタ
		id := strings.ToLower(m.ID)
		if strings.HasPrefix(id, "gpt-") ||
			strings.HasPrefix(id, "chatgpt-") ||
			strings.HasPrefix(id, "o1") ||
			strings.HasPrefix(id, "o3") ||
			strings.HasPrefix(id, "o4") {
			models = append(models, ModelInfo{ID: m.ID, Name: m.ID})
		}
	}
	return models, nil
}

// openAIMessage はOpenAI APIのメッセージ形式
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatRequest はOpenAI Chat API のリクエスト形式
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

// openAIChatResponse はOpenAI Chat API のレスポンス形式
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// openAIStreamChunk はストリーミングレスポンスのチャンク
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI APIキーが設定されていません")
	}

	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{Role: string(m.Role), Content: m.Content}
	}

	body := openAIChatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
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
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
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

func (p *OpenAIProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI APIキーが設定されていません")
	}

	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{Role: string(m.Role), Content: m.Content}
	}

	body := openAIChatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
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
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("OpenAI APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamDelta, 32)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.readSSEStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

// readSSEStream はServer-Sent Eventsストリームを読み取る
func (p *OpenAIProvider) readSSEStream(ctx context.Context, r io.Reader, ch chan<- StreamDelta) {
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

func (p *OpenAIProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("OpenAI APIキーが設定されていません")
	}

	// FIM（Fill-in-the-Middle）プロンプトを構築
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
