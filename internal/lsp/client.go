// Package lsp はLSPクライアント実装を提供する
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ServerState はLSPサーバーの状態
type ServerState int

const (
	ServerStopped      ServerState = iota
	ServerStarting                 // プロセス起動中
	ServerInitializing             // initialize リクエスト送信中
	ServerReady                    // 初期化完了、リクエスト受付可能
	ServerShuttingDown             // shutdown 送信済み
)

// Client はLSPクライアント
type Client struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	state        atomic.Int32
	nextID       atomic.Int32
	rootURI      string
	capabilities ServerCapabilities

	// リクエスト/レスポンスのペンディングマップ
	pendingMu sync.Mutex
	pending   map[int]chan *ResponseMessage

	// 通知コールバック
	onDiagnostics func(params PublishDiagnosticsParams)
	onLog         func(msg string)

	// ドキュメントバージョン管理
	docVersions map[string]int

	// パフォーマンス計測
	LastLatency atomic.Int64 // 最後のリクエスト応答時間（ミリ秒）

	cancel context.CancelFunc
}

// NewClient は新しいLSPクライアントを作成する
func NewClient() *Client {
	c := &Client{
		pending:     make(map[int]chan *ResponseMessage),
		docVersions: make(map[string]int),
	}
	c.state.Store(int32(ServerStopped))
	return c
}

// SetDiagnosticsHandler は診断通知のコールバックを設定する
func (c *Client) SetDiagnosticsHandler(handler func(params PublishDiagnosticsParams)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDiagnostics = handler
}

// State はサーバーの現在の状態を返す
func (c *Client) State() ServerState {
	return ServerState(c.state.Load())
}

// IsReady はサーバーが利用可能かを返す
func (c *Client) IsReady() bool {
	return c.State() == ServerReady
}

// Capabilities はサーバーの機能を返す
func (c *Client) Capabilities() ServerCapabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capabilities
}

// Start はLSPサーバープロセスを起動し初期化する
func (c *Client) Start(command string, args []string, rootPath string) error {
	c.mu.Lock()
	if ServerState(c.state.Load()) != ServerStopped {
		c.mu.Unlock()
		return fmt.Errorf("LSPサーバーは既に起動中です")
	}
	c.state.Store(int32(ServerStarting))
	c.rootURI = FilePathToURI(rootPath)
	c.mu.Unlock()

	// プロセス起動
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	c.cmd = exec.CommandContext(ctx, command, args...)
	c.cmd.Stderr = os.Stderr

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		c.state.Store(int32(ServerStopped))
		cancel()
		return fmt.Errorf("stdin パイプ取得失敗: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		c.state.Store(int32(ServerStopped))
		cancel()
		return fmt.Errorf("stdout パイプ取得失敗: %w", err)
	}
	c.stdout = bufio.NewReaderSize(stdoutPipe, 1024*1024) // 1MBバッファ

	if err := c.cmd.Start(); err != nil {
		c.state.Store(int32(ServerStopped))
		cancel()
		return fmt.Errorf("LSPサーバー起動失敗: %w", err)
	}

	// 読み取りgoroutine起動
	go c.readLoop()

	// 初期化リクエスト送信
	c.state.Store(int32(ServerInitializing))
	if err := c.initialize(); err != nil {
		c.Stop()
		return fmt.Errorf("LSP初期化失敗: %w", err)
	}

	return nil
}

// Stop はLSPサーバーを停止する
func (c *Client) Stop() {
	if ServerState(c.state.Load()) == ServerStopped {
		return
	}
	c.state.Store(int32(ServerShuttingDown))

	// shutdown リクエスト送信（タイムアウト付き）
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = c.sendRequestWithContext(ctx, "shutdown", nil)
	c.sendNotification("exit", nil)

	// プロセス停止
	if c.cancel != nil {
		c.cancel()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Wait()
	}

	// ペンディングリクエストをクリーンアップ
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	c.state.Store(int32(ServerStopped))
}

// ─── 初期化 ───

func (c *Client) initialize() error {
	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   c.rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Completion: &CompletionClientCapabilities{
					CompletionItem: &CompletionItemCapabilities{
						SnippetSupport: false,
					},
				},
				Definition:  &DefinitionClientCapabilities{},
				References:  &ReferencesClientCapabilities{},
				Rename:      &RenameClientCapabilities{PrepareSupport: false},
				PublishDiag: &PublishDiagnosticsCapabilities{},
				CodeAction:  &CodeActionClientCapabilities{},
				Formatting:  &FormattingClientCapabilities{},
				Synchronization: &TextDocSyncClientCapabilities{
					DidSave: true,
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.sendRequestWait(ctx, "initialize", params)
	if err != nil {
		return err
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("initialize レスポンス解析失敗: %w", err)
	}

	c.mu.Lock()
	c.capabilities = result.Capabilities
	c.mu.Unlock()

	// initialized 通知
	c.sendNotification("initialized", struct{}{})

	c.state.Store(int32(ServerReady))
	log.Printf("[LSP] サーバー初期化完了")
	return nil
}

// ─── ドキュメント同期 ───

// DidOpen はドキュメントを開いた通知を送信する
func (c *Client) DidOpen(uri string, languageID string, text string) {
	if !c.IsReady() {
		return
	}
	c.mu.Lock()
	c.docVersions[uri] = 1
	c.mu.Unlock()

	c.sendNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       text,
		},
	})
}

// DidChange はドキュメント変更通知を送信する（Full Sync）
func (c *Client) DidChange(uri string, text string) {
	if !c.IsReady() {
		return
	}
	c.mu.Lock()
	c.docVersions[uri]++
	version := c.docVersions[uri]
	c.mu.Unlock()

	c.sendNotification("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			URI:     uri,
			Version: version,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: text},
		},
	})
}

// DidSave は保存通知を送信する
func (c *Client) DidSave(uri string) {
	if !c.IsReady() {
		return
	}
	c.sendNotification("textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
}

// DidClose はドキュメントを閉じた通知を送信する
func (c *Client) DidClose(uri string) {
	if !c.IsReady() {
		return
	}
	c.mu.Lock()
	delete(c.docVersions, uri)
	c.mu.Unlock()

	c.sendNotification("textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
}

// ─── 補完 ───

// Completion は補完候補を取得する
func (c *Client) Completion(ctx context.Context, uri string, line, character int) ([]CompletionItem, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("LSPサーバーが利用不可")
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	resp, err := c.sendRequestWait(ctx, "textDocument/completion", params)
	if err != nil {
		return nil, err
	}

	// レスポンスがリストか配列かを判定
	var items []CompletionItem
	if err := json.Unmarshal(resp.Result, &items); err != nil {
		var list CompletionList
		if err2 := json.Unmarshal(resp.Result, &list); err2 != nil {
			return nil, fmt.Errorf("補完レスポンス解析失敗: %w", err)
		}
		items = list.Items
	}
	return items, nil
}

// ─── 定義ジャンプ ───

// Definition は定義位置を取得する
func (c *Client) Definition(ctx context.Context, uri string, line, character int) ([]Location, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("LSPサーバーが利用不可")
	}

	params := DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	resp, err := c.sendRequestWait(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	// 単体Locationか配列かを判定
	var locations []Location
	if err := json.Unmarshal(resp.Result, &locations); err != nil {
		var single Location
		if err2 := json.Unmarshal(resp.Result, &single); err2 != nil {
			return nil, fmt.Errorf("定義レスポンス解析失敗: %w", err)
		}
		locations = []Location{single}
	}
	return locations, nil
}

// ─── 参照検索 ───

// References は参照位置を取得する
func (c *Client) References(ctx context.Context, uri string, line, character int) ([]Location, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("LSPサーバーが利用不可")
	}

	params := ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
		Context:      ReferenceContext{IncludeDeclaration: true},
	}

	resp, err := c.sendRequestWait(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	var locations []Location
	if err := json.Unmarshal(resp.Result, &locations); err != nil {
		return nil, fmt.Errorf("参照レスポンス解析失敗: %w", err)
	}
	return locations, nil
}

// ─── リネーム ───

// Rename はシンボルのリネームを実行する
func (c *Client) Rename(ctx context.Context, uri string, line, character int, newName string) (*WorkspaceEdit, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("LSPサーバーが利用不可")
	}

	params := RenameParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
		NewName:      newName,
	}

	resp, err := c.sendRequestWait(ctx, "textDocument/rename", params)
	if err != nil {
		return nil, err
	}

	var edit WorkspaceEdit
	if err := json.Unmarshal(resp.Result, &edit); err != nil {
		return nil, fmt.Errorf("リネームレスポンス解析失敗: %w", err)
	}
	return &edit, nil
}

// ─── コードアクション ───

// CodeAction はコードアクションを取得する
func (c *Client) CodeAction(ctx context.Context, uri string, r Range, diagnostics []Diagnostic) ([]CodeAction, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("LSPサーバーが利用不可")
	}

	params := CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range:        r,
		Context:      CodeActionContext{Diagnostics: diagnostics},
	}

	resp, err := c.sendRequestWait(ctx, "textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}

	var actions []CodeAction
	if err := json.Unmarshal(resp.Result, &actions); err != nil {
		return nil, fmt.Errorf("コードアクションレスポンス解析失敗: %w", err)
	}
	return actions, nil
}

// ─── フォーマット ───

// Format はドキュメントのフォーマットを実行する
func (c *Client) Format(ctx context.Context, uri string) ([]TextEdit, error) {
	if !c.IsReady() {
		return nil, fmt.Errorf("LSPサーバーが利用不可")
	}

	params := DocumentFormattingParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Options: FormattingOptions{
			TabSize:      4,
			InsertSpaces: false,
		},
	}

	resp, err := c.sendRequestWait(ctx, "textDocument/formatting", params)
	if err != nil {
		return nil, err
	}

	var edits []TextEdit
	if err := json.Unmarshal(resp.Result, &edits); err != nil {
		return nil, fmt.Errorf("フォーマットレスポンス解析失敗: %w", err)
	}
	return edits, nil
}

// ─── JSON-RPC トランスポート ───

func (c *Client) sendNotification(method string, params interface{}) {
	data, err := json.Marshal(params)
	if err != nil {
		return
	}

	msg := NotificationMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stdin == nil {
		return
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	_, _ = c.stdin.Write([]byte(header))
	_, _ = c.stdin.Write(body)
}

func (c *Client) sendRequestWithContext(ctx context.Context, method string, params interface{}) error {
	_, err := c.sendRequestWait(ctx, method, params)
	return err
}

func (c *Client) sendRequestWait(ctx context.Context, method string, params interface{}) (*ResponseMessage, error) {
	id := int(c.nextID.Add(1))

	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	msg := RequestMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  data,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// レスポンスチャネル登録
	respCh := make(chan *ResponseMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	start := time.Now()

	// リクエスト送信
	c.mu.Lock()
	if c.stdin == nil {
		c.mu.Unlock()
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("LSPサーバー接続なし")
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	_, err = c.stdin.Write([]byte(header))
	if err == nil {
		_, err = c.stdin.Write(body)
	}
	c.mu.Unlock()

	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("リクエスト送信失敗: %w", err)
	}

	// レスポンス待機
	select {
	case resp, ok := <-respCh:
		latency := time.Since(start).Milliseconds()
		c.LastLatency.Store(latency)
		if !ok {
			return nil, fmt.Errorf("LSPサーバー切断")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("LSPエラー [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// readLoop はLSPサーバーからのメッセージを読み続けるgoroutine
func (c *Client) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[LSP] readLoop パニック: %v", r)
		}
		c.state.Store(int32(ServerStopped))
	}()

	for {
		msg, err := c.readMessage()
		if err != nil {
			if ServerState(c.state.Load()) == ServerShuttingDown || ServerState(c.state.Load()) == ServerStopped {
				return
			}
			log.Printf("[LSP] メッセージ読み取りエラー: %v", err)
			return
		}

		c.handleMessage(msg)
	}
}

func (c *Client) readMessage() (json.RawMessage, error) {
	// Content-Length ヘッダー読み取り
	contentLength := 0
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // ヘッダー終了
		}
		if strings.HasPrefix(line, "Content-Length:") {
			valStr := strings.TrimSpace(line[len("Content-Length:"):])
			val, err := strconv.Atoi(valStr)
			if err != nil {
				return nil, fmt.Errorf("Content-Length 解析失敗: %w", err)
			}
			contentLength = val
		}
	}

	if contentLength <= 0 {
		return nil, fmt.Errorf("Content-Length が不正: %d", contentLength)
	}

	// ボディ読み取り
	body := make([]byte, contentLength)
	_, err := io.ReadFull(c.stdout, body)
	if err != nil {
		return nil, fmt.Errorf("ボディ読み取り失敗: %w", err)
	}

	return json.RawMessage(body), nil
}

func (c *Client) handleMessage(raw json.RawMessage) {
	// まずIDがあるか確認（レスポンス判定）
	var probe struct {
		ID     *int   `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return
	}

	// レスポンスメッセージ
	if probe.ID != nil && probe.Method == "" {
		var resp ResponseMessage
		if err := json.Unmarshal(raw, &resp); err != nil {
			return
		}

		c.pendingMu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.pendingMu.Unlock()

		if ok {
			ch <- &resp
		}
		return
	}

	// 通知メッセージ
	if probe.Method != "" {
		var notif NotificationMessage
		if err := json.Unmarshal(raw, &notif); err != nil {
			return
		}
		c.handleNotification(notif)
	}
}

func (c *Client) handleNotification(notif NotificationMessage) {
	switch notif.Method {
	case "textDocument/publishDiagnostics":
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(notif.Params, &params); err != nil {
			return
		}
		c.mu.Lock()
		handler := c.onDiagnostics
		c.mu.Unlock()
		if handler != nil {
			handler(params)
		}

	case "window/logMessage", "window/showMessage":
		var params struct {
			Type    int    `json:"type"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(notif.Params, &params); err != nil {
			return
		}
		log.Printf("[LSP server] %s", params.Message)
	}
}
