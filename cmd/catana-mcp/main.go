package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// MCP JSON-RPC メッセージ構造
type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP ツール定義
type mcpTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	InputSchema mcpToolInputSchema `json:"inputSchema"`
}

type mcpToolInputSchema struct {
	Type       string                       `json:"type"`
	Properties map[string]mcpToolProperty   `json:"properties,omitempty"`
}

type mcpToolProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// デバッグソケットのパス
const socketPath = "/tmp/catana-debug.sock"

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg jsonrpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		resp := handleMessage(msg)
		if resp != nil {
			data, _ := json.Marshal(resp)
			fmt.Println(string(data))
		}
	}
}

func handleMessage(msg jsonrpcMessage) *jsonrpcMessage {
	switch msg.Method {
	case "initialize":
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "catana-debug",
					"version": "1.0.0",
				},
			},
		}

	case "notifications/initialized":
		return nil // 通知なので返答不要

	case "tools/list":
		tools := []mcpTool{
			{
				Name:        "catana_snapshot",
				Description: "Catanaアプリの全体スナップショットを取得。ウィンドウサイズ、全ウィジェットの位置・サイズ、エディタ状態、パフォーマンス情報を含む。",
				InputSchema: mcpToolInputSchema{Type: "object"},
			},
			{
				Name:        "catana_widgets",
				Description: "Catanaアプリのウィジェットツリーを取得。各ウィジェットの名前、位置(x,y)、サイズ(width,height)、表示状態、子ウィジェットを含む。",
				InputSchema: mcpToolInputSchema{Type: "object"},
			},
			{
				Name:        "catana_editor_state",
				Description: "Catanaエディタの状態を取得。ワークスペース、開いているファイル、アクティブファイル、サイドバー状態、オムニバーモード、カーソル位置を含む。",
				InputSchema: mcpToolInputSchema{Type: "object"},
			},
			{
				Name:        "catana_performance",
				Description: "Catanaアプリのパフォーマンスメトリクスを取得。FPS、メモリ使用量、ウィンドウサイズを含む。",
				InputSchema: mcpToolInputSchema{Type: "object"},
			},
			{
				Name:        "catana_ping",
				Description: "Catanaアプリとの接続確認。アプリが実行中かどうかを確認する。",
				InputSchema: mcpToolInputSchema{Type: "object"},
			},
			{
				Name:        "catana_screenshot",
				Description: "CatanaアプリのスクリーンショットをPNG画像として取得。実際の描画結果を確認できる。macOSのscreencaptureを使用。",
				InputSchema: mcpToolInputSchema{Type: "object"},
			},
		}
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]interface{}{"tools": tools},
		}

	case "tools/call":
		return handleToolCall(msg)

	default:
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &jsonrpcError{Code: -32601, Message: fmt.Sprintf("不明なメソッド: %s", msg.Method)},
		}
	}
}

func handleToolCall(msg jsonrpcMessage) *jsonrpcMessage {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &jsonrpcError{Code: -32602, Message: "パラメータ解析失敗"},
		}
	}

	// ツール名 → デバッグソケットメソッドのマッピング
	methodMap := map[string]string{
		"catana_snapshot":     "snapshot",
		"catana_widgets":      "widgets",
		"catana_editor_state": "editor_state",
		"catana_performance":  "performance",
		"catana_ping":         "ping",
		"catana_screenshot":   "screenshot",
	}

	debugMethod, ok := methodMap[params.Name]
	if !ok {
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Error:   &jsonrpcError{Code: -32602, Message: fmt.Sprintf("不明なツール: %s", params.Name)},
		}
	}

	result, err := queryDebugSocket(debugMethod)
	if err != nil {
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": fmt.Sprintf("エラー: %v\n\nCatanaアプリが起動しているか確認してください。\nソケットパス: %s", err, socketPath),
					},
				},
				"isError": true,
			},
		}
	}

	// スクリーンショットの場合はイメージコンテンツとして返す
	if params.Name == "catana_screenshot" {
		return handleScreenshotResult(msg.ID, result)
	}

	// JSONを整形して返す
	var parsed interface{}
	json.Unmarshal([]byte(result), &parsed)
	pretty, _ := json.MarshalIndent(parsed, "", "  ")

	return &jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": string(pretty),
				},
			},
		},
	}
}

func handleScreenshotResult(id interface{}, result string) *jsonrpcMessage {
	var screenshotData struct {
		ImageBase64 string `json:"image_base64"`
		MimeType    string `json:"mime_type"`
	}
	if err := json.Unmarshal([]byte(result), &screenshotData); err != nil {
		return &jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": fmt.Sprintf("スクリーンショットデータ解析失敗: %v", err)},
				},
				"isError": true,
			},
		}
	}

	return &jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type":     "image",
					"data":     screenshotData.ImageBase64,
					"mimeType": screenshotData.MimeType,
				},
			},
		},
	}
}

// デバッグソケットに接続してクエリを送信
func queryDebugSocket(method string) (string, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return "", fmt.Errorf("Catanaデバッグソケットに接続できません: %w", err)
	}
	defer conn.Close()

	// リクエスト送信
	req := map[string]string{"method": method}
	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		return "", fmt.Errorf("リクエスト送信失敗: %w", err)
	}

	// レスポンス受信
	dec := json.NewDecoder(conn)
	var resp struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
		Err  string          `json:"error"`
	}
	if err := dec.Decode(&resp); err != nil {
		return "", fmt.Errorf("レスポンス受信失敗: %w", err)
	}

	if !resp.OK {
		return "", fmt.Errorf("デバッグサーバーエラー: %s", resp.Err)
	}

	return string(resp.Data), nil
}
