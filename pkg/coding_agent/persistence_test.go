package coding_agent

import (
	"encoding/json"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

func TestUnmarshalAssistantMessagePreservesProviderMetadata(t *testing.T) {
	original := types.AssistantMessage{
		Role:    types.RoleAssistant,
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: "done"}},
		ProviderMetadata: map[string]json.RawMessage{
			"openai.responses.output": json.RawMessage(`[{"type":"reasoning","encrypted_content":"opaque"}]`),
		},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := unmarshalAssistantMessage(raw)
	if err != nil {
		t.Fatalf("unmarshalAssistantMessage: %v", err)
	}
	got := restored.ProviderMetadata["openai.responses.output"]
	if string(got) != string(original.ProviderMetadata["openai.responses.output"]) {
		t.Fatalf("provider metadata = %s", got)
	}
}
