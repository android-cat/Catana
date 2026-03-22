# Catana 開発タスクリスト

## フェーズ一覧

| フェーズ | 名称 | 状態 |
|:---|:---|:---|
| Phase 1 | コアエディタ基盤 | ✅ 完了 |
| Phase 2 | 高度な編集機能 | ✅ 完了 |
| Phase 3 | プロトコル統合 (LSP/DAP/PTY) | ✅ 完了 |
| Phase 4 | バージョン管理統合 (Git) | ✅ 完了 |
| Phase 5 | AI統合 | 📋 計画済 |
| Phase 6 | 拡張システム・互換性 | 📋 計画済 |

---

## Phase 1: コアエディタ基盤 ← 今回実装

### 1.1 プロジェクト基盤
- [x] Go モジュール初期化 (`go.mod`)
- [x] パッケージ構成の確立
- [x] 設計書の作成 (`docs/`)

### 1.2 コアデータ構造
- [x] Rope データ構造の実装 (`internal/core/rope.go`)
  - 挿入 O(log n)、削除 O(log n)
  - split / merge 操作
  - 行ベースアクセス (Line, LineCount, LineStart)
- [x] Rope ユニットテスト (`internal/core/rope_test.go`)
- [x] テキストバッファ (`internal/core/buffer.go`)
  - Rope ラッパー
  - カーソル管理 (行/列)
  - 選択範囲
  - Undo/Redo スタック
  - 行キャッシュ

### 1.3 エディタ状態管理
- [x] エディタ状態 (`internal/editor/editor.go`)
  - ドキュメント管理（開く/閉じる/切替）
  - ワークスペース管理
  - サイドバー/オムニバー状態

### 1.4 シンタックスハイライト
- [x] キーワードベースハイライト (`internal/syntax/highlight.go`)
  - Go 言語対応
  - Rust 言語対応
  - Python 言語対応
  - TypeScript/JavaScript 言語対応
  - トークンタイプ: キーワード、型、文字列、コメント、数値、関数

### 1.5 Gio UI 実装
- [x] テーマ/カラー定義 (`internal/ui/theme.go`)
  - ダークテーマ（ui_design.jsx 準拠）
- [x] アイコン描画 (`internal/ui/icons.go`)
  - ベクターパスによるアイコン群
- [x] アクティビティバー (`internal/ui/activitybar.go`)
  - Files / Search / Git / Extensions / Settings ボタン
  - アクティブ状態のハイライト
- [x] サイドバー (`internal/ui/sidebar.go`)
  - ヘッダー（タイトル + 閉じるボタン）
  - タブ切替によるパネル切替
- [x] ファイルツリー (`internal/ui/filetree.go`)
  - 実ファイルシステムの読込
  - ディレクトリ展開/折畳
  - ファイル選択 → ドキュメントオープン
  - ドラッグ&ドロップによるファイル/フォルダ移動
  - ドラッグ中のフローティングプレビューと自動スクロール
  - 右クリックメニューによるリネーム/削除
- [x] タブバー (`internal/ui/tabbar.go`)
  - タブ表示（アクティブ/非アクティブ）
  - 変更マーカー
  - ブレッドクラム表示
- [x] コードエディタビュー (`internal/ui/editorview.go`)
  - 行番号表示
  - シンタックスハイライト付きコード描画
  - カーソル描画（点滅）
  - スクロール（widget.List）
  - キーボード入力処理
  - マウスクリックによるカーソル配置
- [x] ステータスバー (`internal/ui/statusbar.go`)
  - Git ブランチ表示
  - FPS / メモリ / LSP応答時間
  - 言語 / エンコーディング
- [x] オムニバー (`internal/ui/omnibar.go`)
  - AI / CMD / TERM モード切替
  - 入力フィールド
  - AIチャット表示領域（モックデータ）
  - コンテキストピル

### 1.6 基本操作
- [x] テキスト入力（通常文字挿入）
- [x] Backspace / Delete
- [x] カーソル移動（矢印キー）
- [x] ファイル保存 (Ctrl+S)
- [x] ファイルオープン（ファイルツリーから）
- [x] ファイル/フォルダのドラッグ&ドロップ移動
- [x] 右クリックでのリネーム/削除
- [x] タブ切替
- [x] サイドバートグル (Ctrl+B)
- [x] オムニバーモード切替 (Ctrl+I/K/J)

---

## Phase 2: 高度な編集機能

### 2.1 Tree-sitter 統合
- [x] Tree-sitter Go バインディング (smacker/go-tree-sitter)
- [x] インクリメンタルパース
- [x] 正確なシンタックスハイライト (Go/Rust/Python/TypeScript/JavaScript)

### 2.2 コードフォールディング
- [x] インデントベースの折畳判定
- [x] 折畳/展開UI

### 2.3 ミニマップ
- [x] コード全体のサムネイル描画
- [x] クリックによるスクロール

### 2.4 検索・置換
- [x] ファイル内検索 (Ctrl+F)
- [x] 正規表現検索
- [x] ワークスペース全体検索
- [x] 置換

---

## Phase 3: プロトコル統合

### 3.1 LSP クライアント
- [x] LSP プロトコル型定義 (`internal/lsp/protocol.go`)
- [x] LSP クライアント実装 (JSON-RPC 2.0 over stdio, Content-Length) (`internal/lsp/client.go`)
- [x] 複数言語サーバー管理 (`internal/lsp/manager.go`) - gopls/rust-analyzer/pylsp/tsserver
- [x] Completion (補完) - CompletionPopup UI (`internal/ui/completion.go`)
- [x] Go to Definition
- [x] Find References
- [x] Rename
- [x] Diagnostics (エラー/警告 アンダーライン表示)
- [x] Code Actions
- [x] Formatting
- [x] EditorView 統合 (didOpen/didChange/didSave 通知, 補完トリガー)

### 3.2 DAP クライアント
- [x] DAP プロトコル型定義 (`internal/dap/protocol.go`)
- [x] DAP クライアント実装 (`internal/dap/client.go`)
- [x] Breakpoints (Set/Clear)
- [x] Step Execution (Continue/StepOver/StepIn)
- [x] Threads / StackTrace / Scopes / Variables
- [x] Evaluate

### 3.3 ターミナル (PTY)
- [x] PTY 割り当て (macOS /dev/ptmx) (`internal/terminal/terminal.go`)
- [x] ANSI エスケープシーケンスパーサー (xterm-256color)
- [x] セルバッファ (文字/FG/BG/Bold/Dim属性)
- [x] スクロールバック (最大10,000行)
- [x] 複数ターミナルセッション管理 (`internal/terminal/manager.go`)
- [x] ターミナルUI描画 (`internal/ui/terminalview.go`)
- [x] Cmd+J トグルショートカット
- [x] キー入力フォワーディング (矢印キー, Ctrl+C等)

---

## Phase 4: バージョン管理統合 (Git)

### 4.1 Git CLI 連携
- [x] `git` CLI 実行ラッパー (`internal/git/git.go`)
- [x] ブランチ情報の取得
- [x] ステータス取得 (porcelain v1)

### 4.2 Git UI
- [x] Diff 表示（行単位ハイライト） (`internal/ui/diffview.go`)
- [x] Side-by-side 差分表示
- [x] Inline 差分表示
- [x] Blame 表示
- [x] History 表示
- [x] Stage / Unstage (`internal/ui/gitpanel.go`)
- [x] Commit
- [x] Stash

---

## Phase 5: AI 統合

### 5.1 Provider Abstraction Layer
- [ ] 共通インターフェース定義
- [ ] OpenAI プロバイダ
- [ ] Anthropic プロバイダ
- [ ] Copilot SDK プロバイダ
- [ ] Ollama (ローカルLLM) プロバイダ

### 5.2 AI 機能
- [ ] Inline Completion (ゴーストテキスト)
- [ ] AI Chat パネル
- [ ] コードリファクタリング提案
- [ ] コード解説 (Explain)
- [ ] テスト生成
- [ ] ドキュメント生成

### 5.3 AI コンテキスト
- [ ] 開いているファイルの自動連携
- [ ] Git diff の連携
- [ ] 選択範囲の連携
- [ ] シンボル情報の連携

### 5.4 AI UI
- [ ] Inline ghost text (予測補完表示)
- [ ] Chat panel (対話UI)
- [ ] Diff preview & Apply patch
- [ ] Command palette AI 呼び出し

---

## Phase 6: 拡張システム・互換性

### 6.1 VSCode 拡張互換 Level 1
- [ ] `package.json` パーサー
- [ ] Commands / Keybindings 変換
- [ ] Themes 自動インポート
- [ ] Snippets 自動インポート
- [ ] Grammars (TextMate) インポート

### 6.2 VSCode 拡張互換 Level 2
- [ ] LSP / DAP 接続互換
- [ ] Formatter / Linter 連携

### 6.3 Open VSX 統合
- [ ] Open VSX API クライアント
- [ ] 拡張検索・インストール
- [ ] Manifest 解析・分類 (convertible / lsp / unsupported)

### 6.4 プラグイン API
- [ ] Lua スクリプト拡張
- [ ] WASM 拡張
- [ ] C API (ネイティブ拡張)
