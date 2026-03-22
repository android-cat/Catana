package terminal

import (
	"sync"
)

// Manager は複数のターミナルセッションを管理する
type Manager struct {
	mu         sync.RWMutex
	terminals  []*Terminal
	activeIdx  int
	defaultCwd string
}

// NewManager は新しいターミナルマネージャーを作成する
func NewManager(cwd string) *Manager {
	return &Manager{
		terminals:  make([]*Terminal, 0),
		activeIdx:  -1,
		defaultCwd: cwd,
	}
}

// NewTerminal は新しいターミナルを作成して追加する
func (m *Manager) NewTerminal(rows, cols int) (*Terminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t := New(rows, cols)
	if err := t.Start("", m.defaultCwd, nil); err != nil {
		return nil, err
	}

	m.terminals = append(m.terminals, t)
	m.activeIdx = len(m.terminals) - 1
	return t, nil
}

// ActiveTerminal はアクティブなターミナルを返す
func (m *Manager) ActiveTerminal() *Terminal {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeIdx < 0 || m.activeIdx >= len(m.terminals) {
		return nil
	}
	return m.terminals[m.activeIdx]
}

// SetActive はアクティブなターミナルインデックスを設定する
func (m *Manager) SetActive(idx int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if idx >= 0 && idx < len(m.terminals) {
		m.activeIdx = idx
	}
}

// Count はターミナル数を返す
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.terminals)
}

// ActiveIndex はアクティブなインデックスを返す
func (m *Manager) ActiveIndex() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeIdx
}

// CloseTerminal は指定インデックスのターミナルを閉じる
func (m *Manager) CloseTerminal(idx int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if idx < 0 || idx >= len(m.terminals) {
		return
	}

	m.terminals[idx].Close()
	m.terminals = append(m.terminals[:idx], m.terminals[idx+1:]...)

	if m.activeIdx >= len(m.terminals) {
		m.activeIdx = len(m.terminals) - 1
	}
}

// CloseAll は全ターミナルを閉じる
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, t := range m.terminals {
		t.Close()
	}
	m.terminals = m.terminals[:0]
	m.activeIdx = -1
}
