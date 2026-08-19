package market_sentiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openmodu/modu/pkg/agent"
	"github.com/openmodu/modu/pkg/types"
)

const marketAnalysisSystemPrompt = `你是 A 股市场情绪分析助手。数值分数已由确定性的 Go 代码计算，你不能修改分数。
只根据输入 JSON 解释当前市场；必须区分 ok、proxy、missing，不能把代理指标描述成原始全 A 指标。
用简洁中文输出三段：结论、主要驱动、风险与数据缺口。不要给出个股买卖建议。`

type AgentExplainer struct {
	model    *types.Model
	streamFn types.StreamFn
}

func NewAgentExplainer(model *types.Model, streamFn types.StreamFn) *AgentExplainer {
	return &AgentExplainer{model: model, streamFn: streamFn}
}

func (e *AgentExplainer) Explain(ctx context.Context, snapshot Snapshot) (string, error) {
	if e == nil || e.model == nil {
		return "", errors.New("market sentiment agent model is not configured")
	}
	view := struct {
		TradeDate  string            `json:"trade_date"`
		Score      float64           `json:"score"`
		State      string            `json:"state"`
		Change     float64           `json:"change"`
		Components []Component       `json:"components"`
		Quotes     []Quote           `json:"quotes"`
		Industries []Industry        `json:"top_industries"`
		HotStocks  []HotStock        `json:"top_hot_stocks"`
		Errors     map[string]string `json:"source_errors,omitempty"`
	}{
		TradeDate: snapshot.TradeDate, Score: snapshot.Score, State: snapshot.State,
		Change: snapshot.Change, Components: snapshot.Components, Quotes: snapshot.Quotes,
		Industries: firstIndustries(snapshot.Industries, 8), HotStocks: firstHotStocks(snapshot.HotStocks, 8),
		Errors: snapshot.Errors,
	}
	payload, err := json.Marshal(view)
	if err != nil {
		return "", fmt.Errorf("marshal market sentiment agent input: %w", err)
	}
	maxTokens := 900
	a := agent.NewAgent(types.Config{
		InitialState: &types.State{SystemPrompt: marketAnalysisSystemPrompt, Model: e.model},
		StreamFn:     e.streamFn,
		MaxTokens:    &maxTokens,
	})
	if err := a.Prompt(ctx, string(payload)); err != nil {
		return "", fmt.Errorf("market sentiment agent: %w", err)
	}
	text := lastAssistantText(a.GetState().Messages)
	if text == "" {
		return "", errors.New("market sentiment agent returned empty text")
	}
	return text, nil
}

type RuleExplainer struct{}

func (RuleExplainer) Explain(_ context.Context, snapshot Snapshot) (string, error) {
	if len(snapshot.Components) == 0 {
		return "暂无可解释的市场情绪数据。", nil
	}
	strongest, weakest := snapshot.Components[0], snapshot.Components[0]
	missing := 0
	for _, component := range snapshot.Components {
		if component.Score > strongest.Score {
			strongest = component
		}
		if component.Score < weakest.Score {
			weakest = component
		}
		if component.Status == StatusMissing {
			missing++
		}
	}
	return fmt.Sprintf(
		"%s 市场情绪指数为 %.1f，处于%s。主要正向驱动是%s（%.1f），主要拖累是%s（%.1f）。当前有 %d 个分项因数据缺失使用 50 分中性值；proxy 分项是公开数据代理口径，不等同于全 A 历史截面。",
		snapshot.TradeDate, snapshot.Score, snapshot.State, strongest.Name, strongest.Score, weakest.Name, weakest.Score, missing,
	), nil
}

func lastAssistantText(messages []types.AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		var content []types.ContentBlock
		switch message := messages[i].(type) {
		case types.AssistantMessage:
			content = message.Content
		case *types.AssistantMessage:
			if message != nil {
				content = message.Content
			}
		}
		parts := make([]string, 0, len(content))
		for _, block := range content {
			if text, ok := block.(*types.TextContent); ok && text != nil {
				parts = append(parts, text.Text)
			}
		}
		if len(parts) > 0 {
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
	}
	return ""
}

func firstIndustries(values []Industry, limit int) []Industry {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func firstHotStocks(values []HotStock, limit int) []HotStock {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
