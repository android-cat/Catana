// Package dap はDAPクライアント実装を提供する
package dap

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

// DebugState はデバッグセッションの状態
type DebugState int

const (
	DebugStopped    DebugState = iota // セッションなし
	DebugStarting                     // アダプタ起動中
	DebugRunning                      // プログラム実行中
	DebugPaused                       // ブレークポイントで停止中
	DebugTerminated                   // デバッギー終了
)

// Client はDAPクライアント
type Client struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	state        atomic.Int32
	nextSeq      atomic.Int32
	capabilities Capabilities

	// リクエスト/レスポンスのペンディングマップ
	pendingMu sync.Mutex
	pending   map[int]chan *Response

	// ブレークポイント管理
	bpMu        sync.RWMutex
	breakpoints map[string][]Breakpoint // ファイルパス -> ブレークポイント

	// コールバック
	onStopped    func(body StoppedEventBody)
	onOutput     func(body OutputEventBody)
	onTerminated func()

	// 現在のスレッド/フレーム情報
	StoppedThreadID atomic.Int32
	StackFrames     []StackFrame
	stackMu         sync.RWMutex

	cancel context.CancelFunc
}

// NewClient は新しいDAPクライアントを作成する
func NewClient() *Client {
	c := &Client{
		pending:     make(map[int]chan *Response),
		breakpoints: make(map[string][]Breakpoint),
	}
	c.state.Store(int32(DebugStopped))
	return c
}

// State はデバッグセッションの現在の状態を返す
func (c *Client) State() DebugState {
	return DebugState(c.state.Load())
}

// SetStoppedHandler は停止イベントのコールバックを設定する
func (c *Client) SetStoppedHandler(handler func(body StoppedEventBody)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStopped = handler
}

// SetOutputHandler は出力イベントのコールバックを設定する
func (c *Client) SetOutputHandler(handler func(body OutputEventBody)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onOutput = handler
}

// SetTerminatedHandler は終了イベントのコールバックを設定する
func (c *Client) SetTerminatedHandler(handler func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTerminated = handler
}

// Start はデバッグアダプタを起動する
func (c *Client) Start(command string, args []string) error {
	c.mu.Lock()
	if DebugState(c.state.Load()) != DebugStopped {
		c.mu.Unlock()
		return fmt.Errorf("デバッグセッションは既にアクティブです")
	}
	c.state.Store(int32(DebugStarting))
	c.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	c.cmd = exec.CommandContext(ctx, command, args...)
	c.cmd.Stderr = os.Stderr

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		c.state.Store(int32(DebugStopped))
		cancel()
		return fmt.Errorf("stdin パイプ取得失敗: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		c.state.Store(int32(DebugStopped))
		cancel()
		return fmt.Errorf("stdout パイプ取得失敗: %w", err)
	}
	c.stdout = bufio.NewReaderSize(stdoutPipe, 256*1024)

	if err := c.cmd.Start(); err != nil {
		c.state.Store(int32(DebugStopped))
		cancel()
		return fmt.Errorf("デバッグアダプタ起動失敗: %w", err)
	}

	// 読み取りgoroutine起動
	go c.readLoop()

	// 初期化
	if err := c.initialize(); err != nil {
		c.Stop()
		return fmt.Errorf("DAP初期化失敗: %w", err)
	}

	return nil
}

// Stop はデバッグセッションを終了する
func (c *Client) Stop() {
	if DebugState(c.state.Load()) == DebugStopped {
		return
	}

	// disconnect リクエスト
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = c.sendRequestWait(ctx, "disconnect", map[string]bool{"terminateDebuggee": true})

	if c.cancel != nil {
		c.cancel()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Wait()
	}

	// ペンディングクリア
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	c.state.Store(int32(DebugStopped))
}

// ─── 初期化 ───

func (c *Client) initialize() error {
	args := InitializeArguments{
		ClientID:        "catana",
		ClientName:      "Catana Editor",
		AdapterID:       "catana",
		Locale:          "ja",
		LinesStartAt1:   true,
		ColumnsStartAt1: true,
		PathFormat:      "path",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "initialize", args)
	if err != nil {
		return err
	}

	if resp.Body != nil {
		_ = json.Unmarshal(resp.Body, &c.capabilities)
	}

	log.Printf("[DAP] アダプタ初期化完了")
	return nil
}

// ─── セッション操作 ───

// Launch はプログラムのデバッグを開始する
func (c *Client) Launch(program string, args []string, cwd string) error {
	launchArgs := LaunchRequestArguments{
		Program: program,
		Args:    args,
		Cwd:     cwd,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.sendRequestWait(ctx, "launch", launchArgs); err != nil {
		return err
	}

	// configurationDone
	if c.capabilities.SupportsConfigurationDoneRequest {
		if err := c.sendRequestWait(ctx, "configurationDone", nil); err != nil {
			return err
		}
	}

	c.state.Store(int32(DebugRunning))
	return nil
}

// ─── ブレークポイント ───

// SetBreakpoints はファイルのブレークポイントを設定する
func (c *Client) SetBreakpoints(filePath string, lines []int) ([]Breakpoint, error) {
	srcBPs := make([]SourceBreakpoint, len(lines))
	for i, line := range lines {
		srcBPs[i] = SourceBreakpoint{Line: line}
	}

	args := SetBreakpointsArguments{
		Source:      Source{Path: filePath},
		Breakpoints: srcBPs,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "setBreakpoints", args)
	if err != nil {
		return nil, err
	}

	var result SetBreakpointsResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("ブレークポイントレスポンス解析失敗: %w", err)
	}

	c.bpMu.Lock()
	c.breakpoints[filePath] = result.Breakpoints
	c.bpMu.Unlock()

	return result.Breakpoints, nil
}

// GetBreakpoints は指定ファイルのブレークポイントを返す
func (c *Client) GetBreakpoints(filePath string) []Breakpoint {
	c.bpMu.RLock()
	defer c.bpMu.RUnlock()
	return c.breakpoints[filePath]
}

// ─── 実行制御 ───

// Continue は実行を再開する
func (c *Client) Continue(threadID int) error {
	args := ContinueArguments{ThreadID: threadID}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.sendRequestWait(ctx, "continue", args); err != nil {
		return err
	}
	c.state.Store(int32(DebugRunning))
	return nil
}

// StepOver はステップオーバーを実行する
func (c *Client) StepOver(threadID int) error {
	args := StepArguments{ThreadID: threadID}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.sendRequestWait(ctx, "next", args)
}

// StepInto はステップインする
func (c *Client) StepInto(threadID int) error {
	args := StepArguments{ThreadID: threadID}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.sendRequestWait(ctx, "stepIn", args)
}

// StepOut はステップアウトする
func (c *Client) StepOut(threadID int) error {
	args := StepArguments{ThreadID: threadID}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.sendRequestWait(ctx, "stepOut", args)
}

// Pause は実行を一時停止する
func (c *Client) Pause(threadID int) error {
	args := map[string]int{"threadId": threadID}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.sendRequestWait(ctx, "pause", args)
}

// ─── スレッド/スタック ───

// Threads はスレッドリストを取得する
func (c *Client) Threads() ([]Thread, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "threads", nil)
	if err != nil {
		return nil, err
	}

	var result ThreadsResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return result.Threads, nil
}

// StackTrace はスタックトレースを取得する
func (c *Client) StackTrace(threadID int) ([]StackFrame, error) {
	args := StackTraceArguments{
		ThreadID: threadID,
		Levels:   50,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "stackTrace", args)
	if err != nil {
		return nil, err
	}

	var result StackTraceResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}

	c.stackMu.Lock()
	c.StackFrames = result.StackFrames
	c.stackMu.Unlock()

	return result.StackFrames, nil
}

// GetStackFrames は最後のスタックフレーム情報を返す
func (c *Client) GetStackFrames() []StackFrame {
	c.stackMu.RLock()
	defer c.stackMu.RUnlock()
	return c.StackFrames
}

// ─── 変数 ───

// Scopes はスコープを取得する
func (c *Client) Scopes(frameID int) ([]Scope, error) {
	args := ScopesArguments{FrameID: frameID}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "scopes", args)
	if err != nil {
		return nil, err
	}

	var result ScopesResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return result.Scopes, nil
}

// Variables は変数一覧を取得する
func (c *Client) Variables(ref int) ([]Variable, error) {
	args := VariablesArguments{VariablesReference: ref}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "variables", args)
	if err != nil {
		return nil, err
	}

	var result VariablesResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return result.Variables, nil
}

// Evaluate は式を評価する
func (c *Client) Evaluate(expression string, frameID int, evalContext string) (*EvaluateResponse, error) {
	args := EvaluateArguments{
		Expression: expression,
		FrameID:    frameID,
		Context:    evalContext,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.sendRequestWaitResp(ctx, "evaluate", args)
	if err != nil {
		return nil, err
	}

	var result EvaluateResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ─── DAP トランスポート ───

func (c *Client) sendRequestWait(ctx context.Context, command string, args interface{}) error {
	_, err := c.sendRequestWaitResp(ctx, command, args)
	return err
}

func (c *Client) sendRequestWaitResp(ctx context.Context, command string, args interface{}) (*Response, error) {
	seq := int(c.nextSeq.Add(1))

	var argsData json.RawMessage
	if args != nil {
		data, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		argsData = data
	}

	req := Request{
		ProtocolMessage: ProtocolMessage{Seq: seq, Type: "request"},
		Command:         command,
		Arguments:       argsData,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// レスポンスチャネル登録
	respCh := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[seq] = respCh
	c.pendingMu.Unlock()

	// リクエスト送信
	c.mu.Lock()
	if c.stdin == nil {
		c.mu.Unlock()
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("デバッグアダプタ接続なし")
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	_, err = c.stdin.Write([]byte(header))
	if err == nil {
		_, err = c.stdin.Write(body)
	}
	c.mu.Unlock()

	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("リクエスト送信失敗: %w", err)
	}

	// レスポンス待機
	select {
	case resp, ok := <-respCh:
		if !ok {
			return nil, fmt.Errorf("デバッグアダプタ切断")
		}
		if !resp.Success {
			return nil, fmt.Errorf("DAPエラー: %s", resp.Message)
		}
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, seq)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// readLoop はDAPアダプタからのメッセージを読み続けるgoroutine
func (c *Client) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DAP] readLoop パニック: %v", r)
		}
		prevState := DebugState(c.state.Load())
		c.state.Store(int32(DebugStopped))
		if prevState != DebugStopped {
			c.mu.Lock()
			handler := c.onTerminated
			c.mu.Unlock()
			if handler != nil {
				handler()
			}
		}
	}()

	for {
		msg, err := c.readMessage()
		if err != nil {
			if DebugState(c.state.Load()) == DebugStopped {
				return
			}
			log.Printf("[DAP] メッセージ読み取りエラー: %v", err)
			return
		}
		c.handleMessage(msg)
	}
}

func (c *Client) readMessage() (json.RawMessage, error) {
	contentLength := 0
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
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

	body := make([]byte, contentLength)
	_, err := io.ReadFull(c.stdout, body)
	if err != nil {
		return nil, fmt.Errorf("ボディ読み取り失敗: %w", err)
	}

	return json.RawMessage(body), nil
}

func (c *Client) handleMessage(raw json.RawMessage) {
	var probe struct {
		Type string `json:"type"`
		Seq  int    `json:"seq"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return
	}

	switch probe.Type {
	case "response":
		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			return
		}
		c.pendingMu.Lock()
		ch, ok := c.pending[resp.RequestSeq]
		if ok {
			delete(c.pending, resp.RequestSeq)
		}
		c.pendingMu.Unlock()
		if ok {
			ch <- &resp
		}

	case "event":
		var evt Event
		if err := json.Unmarshal(raw, &evt); err != nil {
			return
		}
		c.handleEvent(evt)
	}
}

func (c *Client) handleEvent(evt Event) {
	switch evt.Event {
	case "stopped":
		var body StoppedEventBody
		if err := json.Unmarshal(evt.Body, &body); err != nil {
			return
		}
		c.state.Store(int32(DebugPaused))
		c.StoppedThreadID.Store(int32(body.ThreadID))

		// スタックトレース自動取得
		go func() {
			if _, err := c.StackTrace(body.ThreadID); err != nil {
				log.Printf("[DAP] スタックトレース取得失敗: %v", err)
			}
		}()

		c.mu.Lock()
		handler := c.onStopped
		c.mu.Unlock()
		if handler != nil {
			handler(body)
		}

	case "output":
		var body OutputEventBody
		if err := json.Unmarshal(evt.Body, &body); err != nil {
			return
		}
		c.mu.Lock()
		handler := c.onOutput
		c.mu.Unlock()
		if handler != nil {
			handler(body)
		}

	case "terminated":
		c.state.Store(int32(DebugTerminated))
		c.mu.Lock()
		handler := c.onTerminated
		c.mu.Unlock()
		if handler != nil {
			handler()
		}

	case "exited":
		c.state.Store(int32(DebugTerminated))
	}
}
