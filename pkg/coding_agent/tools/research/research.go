// Package research provides the optional network research tool set.
//
// It is deliberately separate from the core tools package so SDK hosts that
// only need coding tools do not link browser and HTML extraction dependencies.
package research

import (
	agentconfig "github.com/openmodu/modu/pkg/coding_agent/foundation/config"
	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/coding_agent/tools/common"
	webtools "github.com/openmodu/modu/pkg/coding_agent/tools/web"
	"github.com/openmodu/modu/pkg/types"
)

// Provider constructs and rebinds the optional web_search and web_fetch tools.
type Provider struct{}

// Tools returns the configured network research tools.
func (Provider) Tools(ctx types.ToolContext) []types.Tool {
	return []types.Tool{
		newFetchTool(ctx),
		newSearchTool(ctx),
	}
}

// Rebind rebuilds a research tool with the destination session context.
func (Provider) Rebind(tool types.Tool, ctx types.ToolContext) (types.Tool, bool) {
	switch tool.Name() {
	case "web_fetch":
		return newFetchTool(ctx), true
	case "web_search":
		return newSearchTool(ctx), true
	default:
		return nil, false
	}
}

func newFetchTool(ctx types.ToolContext) types.Tool {
	cfg, _ := ctx.Value(codingtools.ValueWebFetch).(agentconfig.WebFetchConfig)
	artifacts, _ := ctx.Value(codingtools.ValueArtifacts).(*common.ArtifactStore)
	return webtools.NewFetchToolWithConfig(artifacts, webtools.FetchConfig{
		Provider:  cfg.Provider,
		Endpoint:  cfg.Endpoint,
		APIKey:    cfg.APIKey,
		APIKeyEnv: cfg.APIKeyEnv,
	})
}

func newSearchTool(ctx types.ToolContext) types.Tool {
	cfg, _ := ctx.Value(codingtools.ValueWebSearch).(agentconfig.WebSearchConfig)
	return webtools.NewSearchToolWithConfig(webtools.SearchConfig{
		Provider:   cfg.Provider,
		Endpoint:   cfg.Endpoint,
		APIKey:     cfg.APIKey,
		APIKeyEnv:  cfg.APIKeyEnv,
		SearchType: cfg.SearchType,
	})
}
