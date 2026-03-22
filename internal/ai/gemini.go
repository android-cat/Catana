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

// GeminiProvider はGoogle Gemini APIプロバイダ
type GeminiProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewGeminiProvider は新しいGeminiプロバイダを作成する
func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: "https://generativelanguage.googleapis.com/v1beta",
		client:   &http.Client{},
	}
}

func (p *GeminiProvider) Name() string          { return "Gemini" }
func (p *GeminiProvider) IsConfigured() bool    { return p.apiKey != "" }
func (p *GeminiProvider) CurrentModel() string  { return p.model }
func (p *GeminiProvider) SetModel(model string) { p.model = model }

// ─── Gemini API 型定義 ───

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiCandidate struct {
	Content struct {
		Parts []geminiPart `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

type geminiModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
}

// ─── API メソッド ───

func (p *GeminiProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Gemini APIキーが設定されていません")
	}

	url := p.endpoint + "/models?key=" + p.apiKey
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("モデル一覧の取得失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini APIエラー (HTTP %d)", resp.StatusCode)
	}

	var result geminiModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Models {
		// generateContent をサポートするモデルのみ
		supported := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supported = true
				break
			}
		}
		if !supported {
			continue
		}
		// "models/" プレフィックスを除去
		id := strings.TrimPrefix(m.Name, "models/")
		models = append(models, ModelInfo{ID: id, Name: m.DisplayName})
	}
	return models, nil
}

func (p *GeminiProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Gemini APIキーが設定されていません")
	}

	gemReq := p.buildRequest(req)
	data, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.endpoint, p.model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("レスポンスのデコード失敗: %w", err)
	}

	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("レスポンスにcandidatesがありません")
	}

	var content string
	for _, part := range result.Candidates[0].Content.Parts {
		content += part.Text
	}

	return &ChatResponse{
		Content:      content,
		FinishReason: result.Candidates[0].FinishReason,
		TokensUsed:   result.UsageMetadata.TotalTokenCount,
	}, nil
}

func (p *GeminiProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamDelta, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Gemini APIキーが設定されていません")
	}

	gemReq := p.buildRequest(req)
	data, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("リクエストのエンコード失敗: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", p.endpoint, p.model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("APIリクエスト失敗: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Gemini APIエラー (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamDelta, 32)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		p.readSSEStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

func (p *GeminiProvider) readSSEStream(ctx context.Context, r io.Reader, ch chan<- StreamDelta) {
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

		var resp geminiResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		if len(resp.Candidates) > 0 {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.Text != "" {
					ch <- StreamDelta{Content: part.Text}
				}
			}
			if resp.Candidates[0].FinishReason == "STOP" {
				ch <- StreamDelta{Done: true}
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamDelta{Done: true, Error: err}
	} else {
		ch <- StreamDelta{Done: true}
	}
}

func (p *GeminiProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("Gemini APIキーが設定されていません")
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

// buildRequest はChatRequestをGemini API形式に変換する
func (p *GeminiProvider) buildRequest(req *ChatRequest) geminiRequest {
	var contents []geminiContent
	var systemInstruction *geminiContent

	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
			continue
		}
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	gemReq := geminiRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction,
	}

	if req.MaxTokens > 0 || req.Temperature > 0 {
		gemReq.GenerationConfig = &geminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		}
	}

	return gemReq
}
