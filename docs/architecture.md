# Catana アーキテクチャ設計書

## 1. システム概要

Catana は Go + Gio (GPU UI) で構築された次世代ネイティブコードエディタです。
Electron を排除し、GPU アクセラレーテッドレンダリングにより最大240fpsの描画性能を実現します。

## 2. レイヤーアーキテクチャ

```
┌─────────────────────────────────────────────────┐
│              Presentation Layer                  │
│  (Gio GPU Rendering / UI Widgets / Theme)        │
├─────────────────────────────────────────────────┤
│              Application Layer                   │
│  (Editor State / Document / Workspace / Commands)│
├─────────────────────────────────────────────────┤
│                Core Layer                        │
│  (Rope / Buffer / Syntax / Undo Tree)            │
├─────────────────────────────────────────────────┤
│              Integration Layer                   │
│  (LSP Client / DAP Client / MCP / Git / PTY)     │
├─────────────────────────────────────────────────┤
│              Platform Layer                      │
│  (File I/O / OS API / GPU Backend)               │
└─────────────────────────────────────────────────┘
```

## 3. 技術スタック

| コンポーネント | 技術 | 理由 |
|:---|:---|:---|
| 言語 | Go 1.22+ | ネイティブ性能、並行処理、クロスプラットフォーム |
| GPU UI | Gio (gioui.org) | Go純正GPUレンダリング、Metal/Vulkan/D3D対応 |
| テキストバッファ | Rope構造 | O(log n) の挿入・削除、大容量ファイル対応 |
| 構文解析 (Phase 2+) | Tree-sitter | 高速インクリメンタルパース |
| フォント | Go Mono (gofont) | モノスペース、Gio組込み |

## 4. パッケージ構成

```
catana/
├── cmd/catana/           # エントリポイント
│   └── main.go
├── cmd/catana-mcp/       # デバッグ用MCPサーバー
│   └── main.go
├── internal/
│   ├── core/             # コアデータ構造
│   │   ├── rope.go       # Rope データ構造
│   │   ├── rope_test.go  # Rope テスト
│   │   └── buffer.go     # テキストバッファ（Rope + カーソル + Undo）
│   ├── debug/            # デバッグ用ソケットサーバー
│   │   └── server.go     # Unixソケットで状態公開
│   ├── editor/           # エディタロジック
│   │   └── editor.go     # エディタ状態管理・ドキュメント管理
│   ├── git/              # Git CLIラッパー (Phase 4)
│   │   └── git.go        # Repository構造体・Git操作・出力パーサー
│   ├── lsp/              # LSPクライアント
│   │   ├── protocol.go   # LSPプロトコル型定義
│   │   ├── client.go     # LSPクライアント（JSON-RPC over stdio）
│   │   └── manager.go    # 複数言語サーバー管理
│   ├── dap/              # DAPクライアント
│   │   ├── protocol.go   # DAPプロトコル型定義
│   │   └── client.go     # DAPクライアント
│   ├── terminal/         # ターミナルエミュレータ
│   │   ├── terminal.go   # PTYベースANSIターミナル
│   │   └── manager.go    # 複数ターミナルセッション管理
│   ├── syntax/           # シンタックスハイライト
│   │   └── highlight.go  # トークナイザ・言語定義
│   └── ui/               # Gio UIレイヤー
│       ├── app.go        # アプリケーションウィンドウ・メインレイアウト
│       ├── theme.go      # テーマ・カラー定義
│       ├── icons.go      # ベクターアイコン描画
│       ├── activitybar.go # アクティビティバー
│       ├── sidebar.go    # サイドバー
│       ├── filetree.go   # ファイルツリーウィジェット
│       ├── tabbar.go     # タブバー・ブレッドクラム
│       ├── editorview.go # コードエディタビュー
│       ├── completion.go # LSP補完ポップアップ
│       ├── terminalview.go # ターミナルパネルUI
│       ├── gitpanel.go   # Gitソースコントロールパネル (Phase 4)
│       ├── diffview.go   # Diff表示ビュー (Phase 4)
│       ├── statusbar.go  # ステータスバー
│       └── omnibar.go    # オムニバー（AI/CMD/TERM）
├── .vscode/mcp.json      # MCPサーバー設定
├── docs/                 # 設計書
├── go.mod
└── go.sum
```

## 5. GPUレンダリングパイプライン

```
ユーザー入力 → 状態更新 → レイアウト計算 → Gio Op生成 → GPU描画
     ↑                                                    ↓
     └──────────── VSync / InvalidateOp ←──────────────────┘
```

- **即時描画 (Instant Redraw):** `op.InvalidateOp` によるフレーム要求で連続描画
- **FPS制御:** VSync同期 or `app.MaxFPS()` による上限設定
- **差分描画:** Gio内部でGPUコマンドキャッシュ、変更部分のみ再描画

## 6. メモリ設計

| 方針 | 実装 |
|:---|:---|
| Rope構造 | テキストをツリーノードに分割、編集時はノード単位の操作 |
| Lazy Loading | ファイルツリーは展開時のみ読込 |
| 行キャッシュ | 改行位置をキャッシュし、行アクセスを O(1) に近似 |
| Gio Op再利用 | フレーム間で変更のないOp群を再利用 |

## 7. 並行処理設計

- **UIスレッド:** Gio イベントループ（メインgoroutine）
- **ファイルI/O:** 別goroutineで非同期読み書き
- **シンタックスハイライト:** 編集後のバックグラウンド再解析
- **LSP通信:** 専用goroutineでJSON-RPC 2.0プロトコル処理（Content-Length ヘッダー）
- **DAP通信:** 専用goroutineでDAPプロトコル処理
- **PTYターミナル:** 読み取りgoroutineでANSIエスケープシーケンス解析

## 8. プロトコル統合 (Phase 3)

### 8.1 LSPクライアント
- **トランスポート:** JSON-RPC 2.0 over stdio（Content-Lengthヘッダー）
- **同期モード:** textDocumentSync = 1（Full Sync）
- **対応サーバー:** gopls (Go), rust-analyzer (Rust), pylsp (Python), tsserver (TypeScript)
- **機能:** Completion, Definition, References, Rename, CodeAction, Format, Diagnostics
- **UI統合:** CompletionPopup（補完候補表示）、診断アンダーライン（赤=Error, 黄=Warning, 青=Info）

### 8.2 DAPクライアント
- **トランスポート:** Content-Length ヘッダー付きJSON over stdio
- **機能:** Initialize, Launch/Attach, Breakpoints, Step Execution, Threads, StackTrace, Variables, Evaluate
- **イベント:** Stopped, Output, Terminated, Breakpoint

### 8.3 PTYターミナル
- **PTY:** macOS /dev/ptmx（TIOCPTYGRANT/TIOCPTYUNLK syscalls）
- **ANSIパーサー:** xterm-256color互換（カーソル移動、画面クリア、SGR色属性）
- **セルバッファ:** 各セルに文字/FG色/BG色/太字/暗字属性
- **スクロールバック:** 最大10,000行保持
- **マルチセッション:** TerminalManager による複数ターミナル管理

## 9. Git統合設計 (Phase 4)

### 9.1 Git CLIラッパー
- **トランスポート:** `os/exec` によるgitコマンド実行（go-gitライブラリ不使用）
- **キャッシュ:** ブランチ名は1秒間キャッシュ、ステータスは5秒間隔でリフレッシュ
- **パーサー:** porcelain v1（status）、unified diff（diff）、--porcelain（blame）、カスタムformat（log/stash）
- **スレッドセーフ:** Repository.mu による排他制御

### 9.2 Git UIコンポーネント
- **GitPanel:** サイドバーのソースコントロールタブ（コミットメッセージ入力、ステージ/アンステージ一覧、履歴、stash）
- **DiffView:** インラインdiffとside-by-side diffの切替表示（色分け付き行番号表示）
- **StatusBar:** 動的ブランチ名表示

### 9.3 対応操作
- ブランチ情報取得、ステータス取得、Diff（ワーキングツリー/ステージ/HEAD比較）
- Stage/Unstage（個別/全体）、Commit、変更破棄
- Blame、Log（リポジトリ全体/ファイル単位）
- Stash（push/pop/list/drop）

## 10. 対応プラットフォーム

| OS | GPUバックエンド |
|:---|:---|
| macOS | Metal |
| Linux | Vulkan / OpenGL |
| Windows | Direct3D 11 |
