package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentconfig "github.com/openmodu/modu/pkg/coding_agent/foundation/config"
	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/types"
)

func TestProviderReturnsNetworkTools(t *testing.T) {
	got := Provider{}.Tools(types.ToolContext{})
	if len(got) != 2 || got[0].Name() != "web_fetch" || got[1].Name() != "web_search" {
		t.Fatalf("research tools = %v", toolNames(got))
	}
}

func TestProviderUsesWebSearchConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "config-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["query"] != "modu exa" || payload["type"] != "fast" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Configured Exa","url":"https://example.com","highlights":["from config"]}]}`))
	}))
	defer server.Close()

	tools := Provider{}.Tools(types.ToolContext{Values: map[string]any{
		codingtools.ValueWebSearch: agentconfig.WebSearchConfig{
			Provider: "exa", Endpoint: server.URL, APIKey: "config-key", SearchType: "fast",
		},
	}})
	search := findTool(tools, "web_search")
	result, err := search.Execute(context.Background(), "search-1", map[string]any{"query": "modu exa"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Provider: exa") || !strings.Contains(text, "Configured Exa") || !strings.Contains(text, "from config") {
		t.Fatalf("unexpected search result:\n%s", text)
	}
}

func TestProviderUsesWebFetchConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fetch-key" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Configured Fetch","metadata":{"title":"Configured Fetch","sourceURL":"https://example.com/source"}}}`))
	}))
	defer server.Close()

	tools := Provider{}.Tools(types.ToolContext{Values: map[string]any{
		codingtools.ValueWebFetch: agentconfig.WebFetchConfig{
			Provider: "firecrawl", Endpoint: server.URL, APIKey: "fetch-key",
		},
	}})
	result, err := findTool(tools, "web_fetch").Execute(context.Background(), "fetch-1", map[string]any{"url": "https://example.com/source"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := resultText(result); !strings.Contains(text, "Configured Fetch") {
		t.Fatalf("unexpected fetch result:\n%s", text)
	}
}

func findTool(tools []types.Tool, name string) types.Tool {
	for _, tool := range tools {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}

func toolNames(tools []types.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

func resultText(result types.ToolResult) string {
	var text strings.Builder
	for _, block := range result.Content {
		if part, ok := block.(*types.TextContent); ok {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}
