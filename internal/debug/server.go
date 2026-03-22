package debug

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
)

// SocketPath はデバッグ用Unixソケットのパス
const SocketPath = "/tmp/catana-debug.sock"

// DebugServer はアプリ内デバッグソケットサーバー
type DebugServer struct {
	listener net.Listener
	mu       sync.RWMutex
	snapshot *AppSnapshot
	done     chan struct{}
}

// AppSnapshot はアプリのUI状態スナップショット
type AppSnapshot struct {
	// ウィンドウ情報
	WindowWidth  int `json:"window_width"`
	WindowHeight int `json:"window_height"`

	// レイアウトツリー
	Widgets []WidgetInfo `json:"widgets"`

	// エディタ状態
	Editor EditorSnapshot `json:"editor"`

	// パフォーマンス
	FPS      float64 `json:"fps"`
	MemoryMB uint64  `json:"memory_mb"`
}

// WidgetInfo はウィジェットのレイアウト情報
type WidgetInfo struct {
	Name     string       `json:"name"`
	X        int          `json:"x"`
	Y        int          `json:"y"`
	Width    int          `json:"width"`
	Height   int          `json:"height"`
	Visible  bool         `json:"visible"`
	Children []WidgetInfo `json:"children,omitempty"`
}

// EditorSnapshot はエディタ状態のスナップショット
type EditorSnapshot struct {
	Workspace      string   `json:"workspace"`
	OpenFiles      []string `json:"open_files"`
	ActiveFile     string   `json:"active_file"`
	ActiveLanguage string   `json:"active_language"`
	SidebarOpen    bool     `json:"sidebar_open"`
	SidebarTab     string   `json:"sidebar_tab"`
	OmniMode       string   `json:"omni_mode"`
	ShowOmniChat   bool     `json:"show_omni_chat"`
	CursorLine     int      `json:"cursor_line"`
	CursorCol      int      `json:"cursor_col"`
	Modified       bool     `json:"modified"`
}

// NewDebugServer はデバッグサーバーを作成する
func NewDebugServer() *DebugServer {
	return &DebugServer{
		snapshot: &AppSnapshot{},
		done:     make(chan struct{}),
	}
}

// Start はUnixソケットでリッスン開始する
func (ds *DebugServer) Start() error {
	// 既存ソケットを削除
	os.Remove(SocketPath)

	l, err := net.Listen("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("デバッグソケット起動失敗: %w", err)
	}
	ds.listener = l
	log.Printf("[debug] ソケットサーバー起動: %s", SocketPath)

	go ds.acceptLoop()
	return nil
}

// Stop はサーバーを停止する
func (ds *DebugServer) Stop() {
	close(ds.done)
	if ds.listener != nil {
		ds.listener.Close()
	}
	os.Remove(SocketPath)
}

// UpdateSnapshot はスナップショットを更新する（UIスレッドから呼ばれる）
func (ds *DebugServer) UpdateSnapshot(snap *AppSnapshot) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.snapshot = snap
}

func (ds *DebugServer) acceptLoop() {
	for {
		select {
		case <-ds.done:
			return
		default:
		}
		conn, err := ds.listener.Accept()
		if err != nil {
			select {
			case <-ds.done:
				return
			default:
				log.Printf("[debug] accept エラー: %v", err)
				continue
			}
		}
		go ds.handleConn(conn)
	}
}

// リクエスト/レスポンスのプロトコル（改行区切りJSON）
type debugRequest struct {
	Method string `json:"method"`
}

type debugResponse struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data,omitempty"`
	Err  string          `json:"error,omitempty"`
}

func (ds *DebugServer) handleConn(conn net.Conn) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req debugRequest
		if err := dec.Decode(&req); err != nil {
			return // 接続切断
		}

		resp := ds.handleRequest(req)
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

func (ds *DebugServer) handleRequest(req debugRequest) debugResponse {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	switch req.Method {
	case "snapshot":
		// 全体スナップショット
		data, _ := json.Marshal(ds.snapshot)
		return debugResponse{OK: true, Data: data}

	case "widgets":
		// ウィジェットツリーのみ
		data, _ := json.Marshal(ds.snapshot.Widgets)
		return debugResponse{OK: true, Data: data}

	case "editor_state":
		// エディタ状態のみ
		data, _ := json.Marshal(ds.snapshot.Editor)
		return debugResponse{OK: true, Data: data}

	case "performance":
		// パフォーマンス情報
		perf := map[string]interface{}{
			"fps":       ds.snapshot.FPS,
			"memory_mb": ds.snapshot.MemoryMB,
			"window":    fmt.Sprintf("%dx%d", ds.snapshot.WindowWidth, ds.snapshot.WindowHeight),
		}
		data, _ := json.Marshal(perf)
		return debugResponse{OK: true, Data: data}

	case "ping":
		data, _ := json.Marshal("pong")
		return debugResponse{OK: true, Data: data}

	case "screenshot":
		ds.mu.RUnlock() // screenshotは外部コマンドを使うのでロックを一旦開放
		result := ds.takeScreenshot()
		ds.mu.RLock() // defer RUnlock対応で再取得
		return result

	default:
		return debugResponse{OK: false, Err: fmt.Sprintf("不明なメソッド: %s", req.Method)}
	}
}

// takeScreenshot はmacOSのscreencaptureでCatanaウィンドウをキャプチャしbase64 PNGで返す
func (ds *DebugServer) takeScreenshot() debugResponse {
	tmpFile := "/tmp/catana-screenshot-tmp.png"
	os.Remove(tmpFile)

	// SwiftでCGWindowListからCatanaのメインウィンドウIDを取得
	swiftScript := `
import Foundation
import CoreGraphics

let windowList = CGWindowListCopyWindowInfo([.optionAll], kCGNullWindowID)! as! [[String: Any]]
for dict in windowList {
    let owner = dict["kCGWindowOwnerName"] as? String ?? ""
    let name = dict["kCGWindowName"] as? String ?? ""
    if owner.lowercased() == "catana" && name == "Catana" {
        let wid = dict["kCGWindowNumber"] as? Int ?? 0
        print(wid)
        break
    }
}
`
	widCmd := exec.Command("swift", "-e", swiftScript)
	widOut, err := widCmd.Output()
	if err == nil {
		wid := string(widOut)
		for len(wid) > 0 && (wid[len(wid)-1] == '\n' || wid[len(wid)-1] == '\r') {
			wid = wid[:len(wid)-1]
		}
		if wid != "" {
			cmd := exec.Command("screencapture", "-x", "-l", wid, tmpFile)
			if err2 := cmd.Run(); err2 == nil {
				return ds.readScreenshotFile(tmpFile)
			}
		}
	}

	// フォールバック: プロセス名でwindow ID一致を試行 (名前なしウィンドウも含む)
	swiftFallback := `
import Foundation
import CoreGraphics

let windowList = CGWindowListCopyWindowInfo([.optionAll], kCGNullWindowID)! as! [[String: Any]]
for dict in windowList {
    let owner = dict["kCGWindowOwnerName"] as? String ?? ""
    if owner.lowercased() == "catana" {
        let wid = dict["kCGWindowNumber"] as? Int ?? 0
        print(wid)
        break
    }
}
`
	widCmd2 := exec.Command("swift", "-e", swiftFallback)
	widOut2, err2 := widCmd2.Output()
	if err2 == nil {
		wid := string(widOut2)
		for len(wid) > 0 && (wid[len(wid)-1] == '\n' || wid[len(wid)-1] == '\r') {
			wid = wid[:len(wid)-1]
		}
		if wid != "" {
			cmd := exec.Command("screencapture", "-x", "-l", wid, tmpFile)
			if err3 := cmd.Run(); err3 == nil {
				return ds.readScreenshotFile(tmpFile)
			}
		}
	}

	// 最終フォールバック: 全画面キャプチャ
	cmd := exec.Command("screencapture", "-x", "-C", tmpFile)
	if err := cmd.Run(); err != nil {
		return debugResponse{OK: false, Err: fmt.Sprintf("スクリーンショット失敗: %v", err)}
	}
	return ds.readScreenshotFile(tmpFile)
}

func (ds *DebugServer) readScreenshotFile(path string) debugResponse {
	pngData, err := os.ReadFile(path)
	if err != nil {
		return debugResponse{OK: false, Err: fmt.Sprintf("スクリーンショットファイル読込失敗: %v", err)}
	}
	os.Remove(path)

	b64 := base64.StdEncoding.EncodeToString(pngData)
	result := map[string]string{
		"image_base64": b64,
		"mime_type":    "image/png",
	}
	data, _ := json.Marshal(result)
	return debugResponse{OK: true, Data: data}
}
