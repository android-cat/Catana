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

// OllamaProvider はOllama ローカルLLMプロバイダ
type OllamaProvider struct {
	model    string
	endpoint string
	client   *http.Client
}

// NewOllamaProvider は新しいOllamaプロバイダを作成する
func NewOllamaProvider(model, endpoint string) *OllamaProvider {
	if model == "" {
		model = "codellama"
	}
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &OllamaProvider{
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{},
	}
}

func (p *OllamaProvider) Name() string { return "Ollama" }

func (p *OllamaProvider) IsConfigured() bool {
	// Ollamaはローカルサーバーなのでキー不要、常にtrue
	return true
}

func (p *OllamaProvider) CurrentModel() string { return p.model }

func (p *OllamaProvider) SetModel(model string) { p.model = model }

// ollamaTagsResponse はOllama Tags APIのレスポンス形式
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (p *OllamaProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Ollamaサーバーへの接続失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama APIエラー (HTTP %d)", resp.StatusCode)
	}

	var result ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Models {
		name := m.Name
		// ":latest" サフィックスを除去して表示名に
		displayName := strings.TrimSuffix(name, ":latest")
		models = append(models, ModelInfo{ID: name, Name: displayName})
	}
	return models, nil
}

// ollamaChatRequest はOllama Chat APIのリクエスト形式
type ollamaChatRequest struct {
	Model    string             `json:"model"`
	Messages []ollamaChatMsg    `json:"messages"`
	Stream   bool               `json:"stream"`
	Options  *ollamaChatOptions `json:"options,omitempty"`
}

type ollamaChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaChatResponse はOllama Chat APIのレスポンス形式
type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done            bool `json:"done"`
	TotalDuration   int  `json:"total_duration"`
	PromptEvalCount int  `json:"prompt_eval_count"`
	EvalCount       int  `json:"eval_count"`
}

func (p *OllamaProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	messages := make([]ollamaChatMsg, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ollamaChatMsg{Role: string(m.Role), Content: m.Content}
	}

	var opts *ollamaChatOptions
	if req.Temperature > 0 || req.MaxTokens > 0 {
		opts = &ollamaChatOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		}
	}

	body := ollamaChatRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   false,
		Options:  opts,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Ollamaサーバーへの接続失敗（サーバーが起動しているか確認してください）: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("レスポンスのデコード失敗: %w", err)
	}

	return &ChatResponse{
		Content:      result.Message.Content,
		FinishReason: "stop",
		TokensUsed:   result.PromptEvalCount + result.EvalCount,
	}, nil
}

func (p *OllamaProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error) {
	messages := make([]ollamaChatMsg, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ollamaChatMsg{Role: string(m.Role), Content: m.Content}
	}

	var opts *ollamaChatOptions
	if req.Temperature > 0 || req.MaxTokens > 0 {
		opts = &ollamaChatOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		}
	}

	body := ollamaChatRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   true,
		Options:  opts,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Ollamaサーバーへの接続失敗: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Ollama APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamDelta, 32)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.readNDJSONStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

// readNDJSONStream はOllamaのNDJSON（改行区切りJSON）ストリームを読み取る
func (p *OllamaProvider) readNDJSONStream(ctx context.Context, r io.Reader, ch chan<- StreamDelta) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamDelta{Done: true, Error: ctx.Err()}
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var resp ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		if resp.Message.Content != "" {
			ch <- StreamDelta{Content: resp.Message.Content}
		}
		if resp.Done {
			ch <- StreamDelta{Done: true}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamDelta{Done: true, Error: err}
	}
}

func (p *OllamaProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// FIMプロンプトを構築（CodeLlama形式）
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
