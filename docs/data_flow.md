# Catana データフロー設計書

## 1. メインイベントループ

```
┌────────────────────────────────────────────────────────────┐
│                    Gio Event Loop                          │
│                                                            │
│  app.Window.Event() ──→ FrameEvent ──→ Layout() ──→ GPU   │
│       ↑                     │                        │     │
│       │              ┌──────┴──────┐                 │     │
│       │              │ キー入力     │                 │     │
│       │              │ マウス入力   │                 │     │
│       │              │ ウィンドウ   │                 │     │
│       │              └──────┬──────┘                 │     │
│       │                     ↓                        │     │
│       │              State Update                    │     │
│       │                     │                        │     │
│       └── InvalidateOp ←───┘                         │     │
│                                                      │     │
│       e.Frame(ops) ──────────────────────────────────┘     │
└────────────────────────────────────────────────────────────┘
```

## 2. テキスト編集データフロー

```
キーボード入力
    │
    ▼
Key Event 処理 (EditorView)
    │
    ├── 通常文字入力 ──→ Buffer.InsertText()
    │                        │
    │                        ├── UndoStack に記録
    │                        ├── Rope.Insert()
    │                        └── 行キャッシュ更新
    │
    ├── Backspace ──→ Buffer.DeleteBackward()
    │                    │
    │                    ├── UndoStack に記録
    │                    ├── Rope.Delete()
    │                    └── 行キャッシュ更新
    │
    ├── 矢印キー ──→ Buffer.MoveCursor*()
    │                    └── カーソル位置更新
    │
    ├── Ctrl+S ──→ EditorState.SaveActiveFile()
    │                 └── Buffer.Text() → os.WriteFile()
    │
    └── Ctrl+Z ──→ Buffer.Undo()
                     │
                     ├── UndoStack → RedoStack
                     ├── Rope 巻き戻し
                     └── カーソル位置復元

    ↓
状態変更後
    │
    ├── Document.UpdateHighlight() ──→ Highlighter.Highlight()
    │                                      │
    │                                      └── 行ごとの Span[] 生成
    │
    └── op.InvalidateOp{} ──→ 次フレーム再描画
```

## 3. ファイル操作データフロー

```
ファイルツリークリック or Ctrl+O
    │
    ▼
EditorState.OpenFile(path)
    │
    ├── os.ReadFile(path) ──→ ファイル内容取得
    │
    ├── core.NewBuffer(content) ──→ Rope構築
    │                                  │
    │                                  ├── テキストを Rope ノードに分割
    │                                  └── 行キャッシュ構築
    │
    ├── Highlighter.DetectLanguage(filename) ──→ 言語判定
    │
    ├── Document 作成 ──→ documents[] に追加
    │
    └── activeDocIdx 更新 ──→ タブ/エディタ表示切替
```

### 3.1 ファイルツリーのドラッグ&ドロップ移動

```
ファイル/フォルダをドラッグ
    │
    ▼
FileTree が pointer.Press / Drag / Release を追跡
    │
    ├── 移動量が閾値未満 ──→ 通常クリックとして処理
    │
    └── 移動量が閾値以上 ──→ ドラッグ開始
                               │
                               ├── 行矩形からドロップ候補を判定
                               ├── フォルダ上ならその配下へ移動
                               ├── ファイル上なら同階層へ移動
                               └── 空き領域ならワークスペース直下へ移動

    ↓
EditorState.MovePath(src, dstDir)
    │
    ├── 自身配下への移動を拒否
    ├── 同名パス衝突を拒否
    ├── os.Rename() で実ファイル移動
    └── 開いている Document の FilePath / FileName を追従更新

    ↓
FileTree.Reload() ──→ 新しい階層を再描画
```

### 3.2 ファイルツリーの右クリック操作

```
ファイル/フォルダを右クリック
    │
    ▼
FileTree がコンテキストメニューを表示
    │
    ├── 名前を変更
    │      │
    │      ├── インライン入力を表示
    │      ├── EditorState.RenamePath(oldPath, newName)
    │      └── 展開状態と開いている Document を追従更新
    │
    └── 削除
           │
           ├── EditorState.DeletePath(path)
           ├── 対象配下の Document を閉じる
           └── FileTree.Reload() で再描画
```

### 3.3 ドラッグ中の自動スクロール

```
ドラッグ中のカーソルがツリー上下端へ接近
    │
    ▼
FileTree.autoScrollDrag()
    │
    ├── widget.List.Position.Offset を増減
    ├── ドロップ候補を再計算
    └── invalidate により連続スクロール
```

## 4. UIレンダリングデータフロー

```
Layout(gtx)
    │
    ▼
┌── layout.Flex{Vertical} ────────────────────────────┐
│                                                      │
│  ┌── layout.Flex{Horizontal} ──────────────────────┐│
│  │                                                  ││
│  │  ActivityBar  │  Sidebar  │  MainEditor          ││
│  │  (48dp幅)     │  (288dp)  │  (残り全て)           ││
│  │               │           │                      ││
│  │               │  ┌───────────────────────┐       ││
│  │               │  │ TabBar + Breadcrumb   │       ││
│  │               │  ├───────────────────────┤       ││
│  │               │  │                       │       ││
│  │               │  │  layout.Stack{} ──┐   │       ││
│  │               │  │  │ EditorView     │   │       ││
│  │               │  │  │ (Expanded)     │   │       ││
│  │               │  │  │ +Completion    │   │       ││
│  │               │  │  │  Popup(浮動)   │   │       ││
│  │               │  │  │                │   │       ││
│  │               │  │  │ OmniBar       │   │       ││
│  │               │  │  │ (Stacked/S)   │   │       ││
│  │               │  │  └───────────────┘   │       ││
│  │               │  ├───────────────────────┤       ││
│  │               │  │ TerminalView (250dp)  │       ││
│  │               │  │ (Cmd+Jで表示/非表示)   │       ││
│  │               │  └───────────────────────┘       ││
│  │               │                                  ││
│  └──────────────────────────────────────────────────┘│
│                                                      │
│  StatusBar (24dp高)                                  │
│                                                      │
└──────────────────────────────────────────────────────┘
```

## 5. オムニバー データフロー

```
ユーザー操作
    │
    ├── Ctrl+I ──→ OmniMode = AI,   showChat = true
    ├── Ctrl+K ──→ OmniMode = CMD,  showChat = false
    ├── Ctrl+J ──→ OmniMode = TERM, showChat = false
    └── Escape ──→ showChat = false
    
    ↓
OmniBar.Layout()
    │
    ├── AI モード
    │   ├── チャット履歴表示（showChat == true の場合）
    │   ├── コンテキストピル表示（アクティブファイル名）
    │   └── 入力 → AIプロバイダへ送信 (Phase 5)
    │
    ├── CMD モード
    │   └── 入力 → コマンドパレット検索・実行
    │
    └── TERM モード
        └── 入力 → PTY 実行 (Phase 3)
```

## 6. LSP通信データフロー (Phase 3)

```
エディタ操作
    │
    ├── ファイル開く ──→ LSPManager.NotifyDidOpen()
    │                        └── LSPClient.DidOpen() ──→ textDocument/didOpen
    │
    ├── テキスト編集 ──→ EditorState.NotifyDidChange()
    │                        └── LSPClient.DidChange() ──→ textDocument/didChange
    │
    ├── ファイル保存 ──→ LSPManager.NotifyDidSave()
    │                        └── LSPClient.DidSave() ──→ textDocument/didSave
    │
    └── 文字入力 ──→ CompletionPopup.RequestCompletion()
                         │
                         └── LSPClient.Completion() ──→ textDocument/completion
                              │                                │
                              └──── LSP Response ←─────────────┘
                                       │
                                       └── CompletionPopup.Show(items)
                                              │
                                              └── UIに補完候補表示

Language Server → Diagnostics 通知 (textDocument/publishDiagnostics)
    │
    └── LSPClient.handleNotification()
            └── LSPManager.diagnostics に蓄積
                    │
                    └── EditorView.updateDiagnostics()
                            └── 行ごとの診断アンダーライン描画
                                 (赤=Error, 黄=Warning, 青=Info)
```

## 7. DAPデバッグデータフロー (Phase 3)

```
デバッグ開始
    │
    ├── DAPClient.Start(adapter) ──→ アダプタプロセス起動
    │       └── initialize ──→ configurationDone
    │
    ├── DAPClient.Launch(program) ──→ launch request
    │       └── プロセス開始
    │
    ├── ブレークポイント設定 ──→ DAPClient.SetBreakpoints()
    │                                └── setBreakpoints request → response
    │
    └── ステップ実行リクエスト
            ├── Continue / StepOver / StepIn
            └── Response + StoppedEvent
                    │
                    ├── Threads() ──→ スレッド一覧取得
                    ├── StackTrace() ──→ コールスタック取得
                    ├── Scopes() ──→ スコープ取得
                    └── Variables() ──→ 変数値取得 → UI表示
```

## 8. ターミナルデータフロー (Phase 3)

```
Cmd+J ──→ TerminalView表示トグル
    │
    ├── 初回表示 ──→ TerminalManager.NewTerminal(rows, cols, cwd)
    │                     │
    │                     ├── Terminal.New(rows, cols)
    │                     └── Terminal.Start(shell, cwd, env)
    │                            │
    │                            ├── openPTY() ──→ /dev/ptmx
    │                            ├── exec.Command(shell) ──→ プロセス起動
    │                            └── readLoop() goroutine 開始
    │
    └── キー入力 ──→ Terminal.Write(data)
                         │
                         └── PTY ──→ シェルプロセス
                                        │
                                        └── 出力 ──→ PTY ──→ readLoop
                                                              │
                                                              └── processOutput()
                                                                   │
                                                                   ├── 通常文字 → putChar()
                                                                   ├── ESC [ → CSI解析
                                                                   │    ├── カーソル移動 (A/B/C/D)
                                                                   │    ├── 画面クリア (J/K)
                                                                   │    └── SGR色属性 (m)
                                                                   └── onUpdate() → UI再描画

TerminalView.Layout()
    │
    ├── タブバー: ターミナルタブ表示 (+/✕ ボタン)
    └── ターミナル本体: セルグリッド描画
         │
         ├── Terminal.GetScreen() → セルスナップショット
         ├── 各セルの文字 + FG/BG色 + Bold → GPU描画
         └── カーソル位置にブロック描画
```

## 9. 将来フェーズのデータフロー（概要）

### Git統合 (Phase 4)
```
Git操作 (GitPanel / StatusBar / DiffView)
    │
    ▼
EditorState.RefreshGitStatus() / GitStage() / GitCommit() 等
    │
    ▼
git.Repository メソッド
    │
    ├── runGit() ──→ exec.Command("git", args...) ──→ CLI Output
    │
    ▼
パーサー (parseStatusOutput / parseDiffOutput / parseLogOutput 等)
    │
    ▼
EditorState フィールド更新 (GitBranch / GitStatus / GitDiffCache 等)
    │
    ▼
UI再描画
    ├── GitPanel: ステージ/アンステージ一覧、コミット、stash
    ├── DiffView: インライン or Side-by-side diff表示
    └── StatusBar: ブランチ名動的表示
```

### AI統合 (Phase 5)
```
AI入力 → Provider Abstraction → HTTP API → ストリーミングレスポンス → UI更新
                                    │
                                    ├── OpenAI
                                    ├── Anthropic
                                    ├── Copilot SDK
                                    └── Ollama (Local)
```
