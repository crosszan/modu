package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/providers"
	"github.com/openmodu/modu/pkg/types"
)

func TestResolveWithoutProviderReturnsNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("OLLAMA_MODEL", "")
	t.Setenv("LMSTUDIO_MODEL", "")
	t.Setenv("LMSTUDIO_BASE_URL", "")

	model, getAPIKey := Resolve()
	if model != nil || getAPIKey != nil {
		t.Fatalf("expected no implicit provider, got model=%#v getAPIKeyNil=%v", model, getAPIKey == nil)
	}
}

func TestResolveUsesMultiModelConfigBeforeEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_API_KEY", "env-key")
	t.Setenv("OPENAI_MODEL", "env-model")
	writeConfig(t, home, `active = "local-qwen"

[[models]]
name = "local-qwen"
provider = "lmstudio"
model = "qwen/qwen3.6-35b-a3b"
baseUrl = "http://127.0.0.1:1234/v1"
apiKey = "local-key"

[[models]]
name = "deepseek"
provider = "deepseek"
model = "deepseek-chat"
baseUrl = "https://api.deepseek.com/v1"
apiKey = "deepseek-key"
`)

	model, getAPIKey := Resolve()
	if model == nil {
		t.Fatal("expected configured model")
	}
	if model.ProviderID != "lmstudio" || model.ID != "qwen/qwen3.6-35b-a3b" || model.Name != "local-qwen" {
		t.Fatalf("unexpected active model: %#v", model)
	}
	key, err := getAPIKey("lmstudio")
	if err != nil || key != "local-key" {
		t.Fatalf("unexpected api key %q err=%v", key, err)
	}
}

func TestResolveUsesResponsesForOpenAIEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("LMSTUDIO_MODEL", "")
	t.Setenv("LMSTUDIO_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "gpt-5")

	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":"resp_1","model":"gpt-5","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")

	model, _ := Resolve()
	if model == nil || model.Api != types.KnownApiOpenAIResponses {
		t.Fatalf("expected OpenAI Responses model, got %#v", model)
	}
	registered, ok := providers.Get("openai")
	if !ok {
		t.Fatal("expected OpenAI provider to be registered")
	}
	if _, err := registered.Chat(context.Background(), &providers.ChatRequest{Model: model.ID}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if requestPath != "/v1/responses" {
		t.Fatalf("OpenAI environment used %q, want /v1/responses", requestPath)
	}
}

func TestResolveConfiguredResponsesProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":"resp_1","model":"gpt-5","status":"completed","output":[]}`)
	}))
	defer server.Close()

	writeConfig(t, home, `active = "gpt-5"

[providers.openai]
type = "openai-responses"
baseUrl = "`+server.URL+`/v1"
apiKey = "test-key"

[[models]]
name = "gpt-5"
provider = "openai"
model = "gpt-5"
capabilities = ["text", "image", "tools"]
`)

	model, _ := Resolve()
	if model == nil || model.Api != types.KnownApiOpenAIResponses {
		t.Fatalf("expected configured Responses model, got %#v", model)
	}
	registered, ok := providers.Get("openai")
	if !ok {
		t.Fatal("expected configured provider")
	}
	if _, err := registered.Chat(context.Background(), &providers.ChatRequest{Model: model.ID}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if requestPath != "/v1/responses" {
		t.Fatalf("configured provider used %q, want /v1/responses", requestPath)
	}
}

func TestResolveAppliesConfiguredCapabilitiesToModelInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `active = "vision-qwen"

[[models]]
name = "vision-qwen"
provider = "lmstudio"
model = "qwen/qwen3.6-35b-a3b"
baseUrl = "http://127.0.0.1:1234/v1"
capabilities = ["text", "image"]

[[models]]
name = "text-only"
provider = "lmstudio"
model = "some/text-only-model"
baseUrl = "http://127.0.0.1:1234/v1"
capabilities = ["text"]
`)

	model, _ := Resolve()
	if model == nil {
		t.Fatal("expected configured model")
	}
	if !containsString(model.Input, "image") {
		t.Fatalf("active model's declared capabilities should reach Model.Input, got %#v", model.Input)
	}

	textOnly := providers.GetModel("lmstudio", "some/text-only-model")
	if textOnly == nil {
		t.Fatal("expected the second configured model to be registered too")
	}
	if containsString(textOnly.Input, "image") {
		t.Fatalf("a model whose capabilities omit image should not report image input, got %#v", textOnly.Input)
	}
}

func TestResolveAppliesConfiguredContextWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `active = "local-qwen"

[[models]]
name = "local-qwen"
provider = "lmstudio"
model = "qwen/qwen3.6-35b-a3b"
baseUrl = "http://127.0.0.1:1234/v1"
contextWindow = 32768
`)

	model, _ := Resolve()
	if model == nil {
		t.Fatal("expected configured model")
	}
	if model.ContextWindow != 32768 {
		t.Fatalf("expected contextWindow 32768, got %d", model.ContextWindow)
	}
}

func TestResolveAppliesProviderDefaultContextWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `active = "deepseek"

[[models]]
name = "deepseek"
provider = "deepseek"
model = "deepseek-chat"
baseUrl = "https://api.deepseek.com/v1"
`)

	model, _ := Resolve()
	if model == nil {
		t.Fatal("expected configured model")
	}
	if model.ContextWindow != 1000000 {
		t.Fatalf("expected default contextWindow 1000000, got %d", model.ContextWindow)
	}
}

func TestResolveAppliesXiaomiMimoDefaultContextWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `active = "mimo-v2.5-pro"

[providers.xiaomi-mimo]
type = "openai-compatible"
baseUrl = "https://token-plan-cn.xiaomimimo.com/v1"

[[models]]
name = "mimo-v2.5-pro"
provider = "xiaomi-mimo"
model = "mimo-v2.5-pro"
`)

	model, _ := Resolve()
	if model == nil {
		t.Fatal("expected configured model")
	}
	if model.ContextWindow != 1000000 {
		t.Fatalf("expected default contextWindow 1000000, got %d", model.ContextWindow)
	}
}

func TestResolveEnvProviderDefaultContextWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-key")
	t.Setenv("DEEPSEEK_MODEL", "deepseek-v4-pro")

	model, _ := Resolve()
	if model == nil {
		t.Fatal("expected env model")
	}
	if model.ContextWindow != 1000000 {
		t.Fatalf("expected default contextWindow 1000000, got %d", model.ContextWindow)
	}
}

func TestResolveUsesV2ProviderConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DEEPSEEK_API_KEY", "env-deepseek-key")
	writeConfig(t, home, `version = 2
active = "deepseek"

[providers.deepseek]
type = "openai-compatible"
baseUrl = "https://api.deepseek.com/v1"
apiKeyEnv = "DEEPSEEK_API_KEY"

[[models]]
name = "deepseek"
description = "remote"
provider = "deepseek"
model = "deepseek-chat"
capabilities = ["tools"]
`)

	model, getAPIKey := Resolve()
	if model == nil {
		t.Fatal("expected configured model")
	}
	if model.ProviderID != "deepseek" || model.ID != "deepseek-chat" || model.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("unexpected model: %#v", model)
	}
	key, err := getAPIKey("deepseek")
	if err != nil || key != "env-deepseek-key" {
		t.Fatalf("unexpected api key %q err=%v", key, err)
	}
}

func TestResolveRejectsConfigWithUnknownActiveModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_API_KEY", "env-key")
	writeConfig(t, home, `active = "missing-model"

[[models]]
name = "local-qwen"
provider = "lmstudio"
model = "qwen"
baseUrl = "http://127.0.0.1:1234/v1"
`)

	model, getAPIKey := Resolve()
	if model != nil || getAPIKey != nil {
		t.Fatalf("expected invalid active config to block fallback, got model=%#v keyNil=%v", model, getAPIKey == nil)
	}
}

func TestSaveActiveModelUpdatesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `active = "local-qwen"

[[models]]
name = "local-qwen"
provider = "lmstudio"
model = "qwen"
baseUrl = "127.0.0.1:1234/v1"

[[models]]
name = "remote-deepseek"
provider = "deepseek"
model = "deepseek-chat"
baseUrl = "https://api.deepseek.com/v1"
`)

	if err := SaveActiveModel("deepseek", "deepseek-chat"); err != nil {
		t.Fatalf("SaveActiveModel: %v", err)
	}
	cfg, ok := LoadConfig()
	if !ok {
		t.Fatal("expected config to load")
	}
	if cfg.Active != "remote-deepseek" {
		t.Fatalf("active = %q, want remote-deepseek", cfg.Active)
	}
	if cfg.Models[0].BaseURL != "" || cfg.Models[1].BaseURL != "" {
		t.Fatalf("expected saved config to strip legacy model baseUrl: %#v", cfg.Models)
	}
	if cfg.Providers["lmstudio"].BaseURL == "" || cfg.Providers["deepseek"].BaseURL == "" {
		t.Fatalf("expected provider baseUrls after migration: %#v", cfg.Providers)
	}
}

func TestModelMatchesTarget(t *testing.T) {
	model := ModelConfig{Name: "local", Provider: "lmstudio", Model: "qwen"}
	for _, target := range []string{"local", "qwen", "lmstudio/qwen", "lmstudio:qwen"} {
		if !ModelMatchesTarget(model, target) {
			t.Fatalf("expected target %q to match", target)
		}
	}
	for _, target := range []string{"", "other", "openai/qwen"} {
		if ModelMatchesTarget(model, target) {
			t.Fatalf("expected target %q not to match", target)
		}
	}
}

func TestInitAndValidateConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := InitConfig(false)
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}
	if path != filepath.Join(home, ".modu", "config.toml") {
		t.Fatalf("unexpected path: %s", path)
	}
	result := ValidateConfig()
	if len(result.Problems) != 0 {
		t.Fatalf("expected valid example config, got %#v", result.Problems)
	}
	if result.ModelCount != 2 || result.Active != "local-qwen" {
		t.Fatalf("unexpected validation result: %#v", result)
	}
	if _, err := InitConfig(false); err == nil {
		t.Fatal("expected init without force to refuse existing config")
	}
}

func TestUpsertUseAndRemoveModelConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	created, err := UpsertModelConfig(ModelConfig{
		Name:        "local-qwen",
		Description: "local coding model",
		Provider:    "lmstudio",
		Model:       "qwen",
		BaseURL:     "127.0.0.1:1234/v1",
		APIKey:      "local-key",
	})
	if err != nil {
		t.Fatalf("UpsertModelConfig create: %v", err)
	}
	if !created {
		t.Fatal("expected model to be created")
	}

	created, err = UpsertModelConfig(ModelConfig{
		Name:        "local-qwen",
		Description: "updated description",
		Provider:    "lmstudio",
		Model:       "qwen2",
		BaseURL:     "http://127.0.0.1:1234/v1",
	})
	if err != nil {
		t.Fatalf("UpsertModelConfig update: %v", err)
	}
	if created {
		t.Fatal("expected existing model to be updated")
	}

	cfg, ok := LoadConfig()
	if !ok {
		t.Fatal("expected config to load")
	}
	if cfg.Active != "local-qwen" || len(cfg.Models) != 1 {
		t.Fatalf("unexpected config after upsert: %#v", cfg)
	}
	if cfg.Models[0].Description != "updated description" || cfg.Models[0].Model != "qwen2" {
		t.Fatalf("unexpected model after update: %#v", cfg.Models[0])
	}

	active, err := SetActiveModel("lmstudio/qwen2")
	if err != nil {
		t.Fatalf("SetActiveModel: %v", err)
	}
	if active.Name != "local-qwen" {
		t.Fatalf("unexpected active model: %#v", active)
	}

	removed, err := RemoveModelConfig("local-qwen")
	if err != nil {
		t.Fatalf("RemoveModelConfig: %v", err)
	}
	if removed.Model != "qwen2" {
		t.Fatalf("unexpected removed model: %#v", removed)
	}
	cfg, exists, err := LoadConfigFile()
	if err != nil || !exists {
		t.Fatalf("expected config file to remain, exists=%v err=%v", exists, err)
	}
	if cfg.Active != "" || len(cfg.Models) != 0 {
		t.Fatalf("unexpected config after remove: %#v", cfg)
	}
}

func TestSetScopedModelIDsPersistsConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := InitConfig(false); err != nil {
		t.Fatalf("InitConfig: %v", err)
	}

	if err := SetScopedModelIDs([]string{"deepseek"}); err != nil {
		t.Fatalf("SetScopedModelIDs: %v", err)
	}
	cfg, ok := LoadConfig()
	if !ok {
		t.Fatal("expected config")
	}
	if len(cfg.ScopedModels) != 1 || cfg.ScopedModels[0] != "deepseek" {
		t.Fatalf("unexpected scoped models: %#v", cfg.ScopedModels)
	}
	if got := ConfiguredModelIDs(); len(got) != 1 || got[0] != "deepseek-chat" {
		t.Fatalf("ConfiguredModelIDs = %#v, want deepseek-chat", got)
	}

	if err := SetScopedModelIDs(nil); err != nil {
		t.Fatalf("SetScopedModelIDs clear: %v", err)
	}
	cfg, _ = LoadConfig()
	if len(cfg.ScopedModels) != 0 {
		t.Fatalf("expected scoped models cleared, got %#v", cfg.ScopedModels)
	}
}

func TestDiscoverProviderModelsPersistsOpenAICompatibleModels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldClient := modelDiscoveryHTTPClient
	modelDiscoveryHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://example.test/v1/models" {
			t.Fatalf("unexpected URL: %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"qwen"},{"id":"gpt-4o"},{"id":"qwen"},{"id":""}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { modelDiscoveryHTTPClient = oldClient })

	if err := UpsertProviderConfig("openai", ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://example.test/v1",
		APIKey:  "test-key",
	}); err != nil {
		t.Fatalf("UpsertProviderConfig: %v", err)
	}

	discovery, err := DiscoverProviderModels(context.Background(), "openai")
	if err != nil {
		t.Fatalf("DiscoverProviderModels: %v", err)
	}
	if discovery.Found != 2 || discovery.Added != 2 || discovery.Updated != 0 {
		t.Fatalf("unexpected discovery result: %#v", discovery)
	}
	cfg, ok := LoadConfig()
	if !ok {
		t.Fatal("expected config to load")
	}
	if cfg.Active != "gpt-4o" {
		t.Fatalf("expected first discovered model active, got %q", cfg.Active)
	}
	if len(cfg.Models) != 2 || cfg.Models[0].Name != "gpt-4o" || cfg.Models[1].Name != "qwen" {
		t.Fatalf("unexpected discovered models: %#v", cfg.Models)
	}
}

func TestResponsesProviderSupportsModelDiscovery(t *testing.T) {
	oldClient := modelDiscoveryHTTPClient
	modelDiscoveryHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"gpt-5"}]}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { modelDiscoveryHTTPClient = oldClient })

	ids, err := fetchOpenAIModelIDs(context.Background(), ProviderConfig{
		Type:    "openai-responses",
		BaseURL: "https://example.test/v1",
	})
	if err != nil {
		t.Fatalf("fetchOpenAIModelIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "gpt-5" {
		t.Fatalf("unexpected model IDs: %#v", ids)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestUpsertProviderConfigPreservesExistingSecret(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := UpsertProviderConfig("openai", ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "old-key",
	}); err != nil {
		t.Fatalf("UpsertProviderConfig first: %v", err)
	}
	if err := UpsertProviderConfig("openai", ProviderConfig{
		Type:    "openai-compatible",
		BaseURL: "https://example.test/v1",
	}); err != nil {
		t.Fatalf("UpsertProviderConfig second: %v", err)
	}
	cfg, exists, err := LoadConfigFile()
	if err != nil || !exists {
		t.Fatalf("expected config, exists=%v err=%v", exists, err)
	}
	if got := cfg.Providers["openai"].APIKey; got != "old-key" {
		t.Fatalf("expected existing API key preserved, got %q", got)
	}
}

func TestValidateConfigReportsProblems(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `active = "missing"

[[models]]
name = "broken"
provider = ""
model = "qwen"
baseUrl = "127.0.0.1:1234/v1"

[[models]]
name = "broken"
provider = "deepseek"
model = ""
baseUrl = ""
`)

	result := ValidateConfig()
	for _, want := range []string{
		"models[0].provider is required",
		"models[1].model is required",
		"models[1].provider \"deepseek\" has no baseUrl",
		"providers.deepseek.baseUrl is required",
		"models[1].name duplicates \"broken\"",
		"active model does not match any configured model",
	} {
		if !containsString(result.Problems, want) {
			t.Fatalf("expected problem %q in %#v", want, result.Problems)
		}
	}
}

func TestSaveConfigPreservesSettingsTable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `version = 2

[settings]
disableWorkflows = true

[settings.permissions]
defaultMode = "auto"

[providers.deepseek]
type = "openai-compatible"
baseUrl = "https://api.deepseek.com/v1"

[[models]]
name = "deepseek"
provider = "deepseek"
model = "deepseek-chat"
`)

	if err := SaveActiveModel("deepseek", "deepseek-chat"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".modu", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`[settings]`, `disableWorkflows = true`, `[settings.permissions]`, `defaultMode = "auto"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("config save lost settings %q:\n%s", want, text)
		}
	}
}

func TestLoadConfigFileAtUsesExplicitPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, `version = 2

[providers.default]
baseUrl = "https://default.example/v1"

[[models]]
name = "default"
provider = "default"
model = "default-model"
`)

	customPath := filepath.Join(t.TempDir(), "onecatch", "config.toml")
	if err := os.MkdirAll(filepath.Dir(customPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customPath, []byte(`version = 2
active = "managed"

[providers.managed]
baseUrl = "https://managed.example/v1"
apiKeyEnv = "ONECATCH_MODU_API_KEY"

[[models]]
name = "managed"
provider = "managed"
model = "managed-model"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, exists, err := LoadConfigFileAt(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || cfg.Active != "managed" || len(cfg.Models) != 1 || cfg.Models[0].Model != "managed-model" {
		t.Fatalf("LoadConfigFileAt() = (%+v, %v), want explicit managed config", cfg, exists)
	}
}

func TestResolveConfigFileUsesExplicitPath(t *testing.T) {
	t.Setenv("ONECATCH_MODU_API_KEY", "managed-secret")
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`version = 2
active = "managed"

[providers.managed]
baseUrl = "https://managed.example/v1"
apiKeyEnv = "ONECATCH_MODU_API_KEY"

[[models]]
name = "managed"
provider = "managed"
model = "managed-model"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	model, getAPIKey, err := ResolveConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || model.ProviderID != "managed" || model.ID != "managed-model" {
		t.Fatalf("ResolveConfigFile() model = %+v", model)
	}
	key, err := getAPIKey("managed")
	if err != nil || key != "managed-secret" {
		t.Fatalf("GetAPIKey(managed) = %q, %v", key, err)
	}
}

func TestSaveConfigFileAtUsesExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "onecatch", "config.toml")
	cfg := Config{
		Active: "managed",
		Providers: map[string]ProviderConfig{
			"managed": {BaseURL: "https://managed.example/v1", APIKeyEnv: "ONECATCH_MODU_API_KEY"},
		},
		Models: []ModelConfig{{Name: "managed", Provider: "managed", Model: "managed-model"}},
	}
	if err := SaveConfigFileAt(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := LoadConfigFileAt(path)
	if err != nil || !exists || loaded.Active != "managed" {
		t.Fatalf("saved config = (%+v, %v, %v)", loaded, exists, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}
}

func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".modu")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
