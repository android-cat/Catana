package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.General.FontSize != 14 {
		t.Errorf("FontSize = %d, want 14", cfg.General.FontSize)
	}
	if cfg.General.TabSize != 4 {
		t.Errorf("TabSize = %d, want 4", cfg.General.TabSize)
	}
	if cfg.General.DarkMode != true {
		t.Error("DarkMode should be true")
	}
	if cfg.General.WordWrap != false {
		t.Error("WordWrap should be false")
	}
	if cfg.General.Minimap != true {
		t.Error("Minimap should be true")
	}
	if cfg.General.LineNumbers != true {
		t.Error("LineNumbers should be true")
	}
	if cfg.General.AutoSave != false {
		t.Error("AutoSave should be false")
	}
	if cfg.General.Shell == "" {
		t.Error("Shell is empty")
	}
}

func TestDefaultConfig_AIProviders(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AI.ActiveProvider != "ollama" {
		t.Errorf("ActiveProvider = %q, want ollama", cfg.AI.ActiveProvider)
	}
	expected := []string{"openai", "anthropic", "copilot", "ollama", "gemini"}
	for _, name := range expected {
		if _, ok := cfg.AI.Providers[name]; !ok {
			t.Errorf("provider %q not defined", name)
		}
	}
	if cfg.AI.Providers["openai"].Model == "" {
		t.Error("OpenAI model is empty")
	}
	if cfg.AI.Providers["ollama"].Endpoint != "http://localhost:11434" {
		t.Errorf("Ollama Endpoint = %q", cfg.AI.Providers["ollama"].Endpoint)
	}
}

func TestGetAIProvider(t *testing.T) {
	cfg := DefaultConfig()
	p := cfg.GetAIProvider("openai")
	if p.Model == "" {
		t.Error("GetAIProvider(openai) model is empty")
	}
	p = cfg.GetAIProvider("nonexistent")
	if p.Model != "" || p.APIKey != "" || p.Endpoint != "" {
		t.Errorf("nonexistent provider returned values: %+v", p)
	}
}

func TestSetAIProvider(t *testing.T) {
	cfg := DefaultConfig()
	np := AIProviderConfig{APIKey: "test-key", Model: "test-model", Endpoint: "http://test.example.com"}
	cfg.SetAIProvider("test", np)
	got := cfg.GetAIProvider("test")
	if got.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", got.APIKey)
	}
	if got.Model != "test-model" {
		t.Errorf("Model = %q, want test-model", got.Model)
	}
	if got.Endpoint != "http://test.example.com" {
		t.Errorf("Endpoint = %q", got.Endpoint)
	}
}

func TestSetAIProvider_Overwrite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SetAIProvider("openai", AIProviderConfig{APIKey: "new-key", Model: "gpt-5"})
	got := cfg.GetAIProvider("openai")
	if got.APIKey != "new-key" {
		t.Errorf("APIKey = %q, want new-key", got.APIKey)
	}
	if got.Model != "gpt-5" {
		t.Errorf("Model = %q, want gpt-5", got.Model)
	}
}

func TestSetAIProvider_NilMap(t *testing.T) {
	cfg := &Config{}
	cfg.SetAIProvider("test", AIProviderConfig{Model: "test"})
	got := cfg.GetAIProvider("test")
	if got.Model != "test" {
		t.Errorf("Model = %q, want test", got.Model)
	}
}
