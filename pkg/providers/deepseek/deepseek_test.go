package deepseek

import (
	"testing"

	"github.com/openmodu/modu/pkg/providers"
)

func TestNewReturnsDeepSeekProvider(t *testing.T) {
	p := New("explicit-key")
	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.ID() != providers.ProviderNameDeepSeek {
		t.Errorf("ID() = %q, want %q", p.ID(), providers.ProviderNameDeepSeek)
	}
}

func TestNewFallsBackToEnvVarWhenAPIKeyEmpty(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "from-env")
	p := New("")
	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.ID() != providers.ProviderNameDeepSeek {
		t.Errorf("ID() = %q, want %q", p.ID(), providers.ProviderNameDeepSeek)
	}
}
