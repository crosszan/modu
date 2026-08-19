package market_sentiment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/modu/pkg/types"
)

func TestAgentExplainerUsesPkgAgentAndReturnsAssistantText(t *testing.T) {
	var prompt string
	streamFn := func(_ context.Context, model *types.Model, llmCtx *types.LLMContext, _ *types.SimpleStreamOptions) (types.EventStream, error) {
		for _, message := range llmCtx.Messages {
			if user, ok := message.(types.UserMessage); ok {
				prompt = user.Content.(string)
			}
		}
		stream := types.NewEventStream()
		go func() {
			message := &types.AssistantMessage{
				Role: types.RoleAssistant, ProviderID: model.ProviderID, Model: model.ID,
				Content:    []types.ContentBlock{&types.TextContent{Type: "text", Text: "市场处于中性。"}},
				StopReason: "stop", Timestamp: time.Now().UnixMilli(),
			}
			stream.Push(types.StreamEvent{Type: types.EventDone, Reason: "stop", Message: message})
			stream.Resolve(message, nil)
			stream.Close()
		}()
		return stream, nil
	}
	explainer := NewAgentExplainer(&types.Model{ID: "mock", ProviderID: "mock"}, streamFn)
	got, err := explainer.Explain(context.Background(), Snapshot{TradeDate: "2026-08-14", Score: 50, State: "中性"})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if got != "市场处于中性。" {
		t.Fatalf("analysis = %q", got)
	}
	if !strings.Contains(prompt, "2026-08-14") || !strings.Contains(prompt, `"score":50`) {
		t.Fatalf("prompt = %q", prompt)
	}
}
