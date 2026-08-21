package main

import (
	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/coding_agent/tools/research"
	"github.com/openmodu/modu/pkg/types"
)

type moduCodeToolProvider struct {
	base     codingtools.DefaultProvider
	research research.Provider
}

func newModuCodeToolProvider() types.ToolManager {
	return &moduCodeToolProvider{
		base: codingtools.NewProvider(codingtools.ToolSetCoding),
	}
}

func (p *moduCodeToolProvider) Tools(ctx types.ToolContext) []types.Tool {
	return append(p.base.Tools(ctx), p.research.Tools(ctx)...)
}

func (p *moduCodeToolProvider) Rebind(tool types.Tool, ctx types.ToolContext) (types.Tool, bool) {
	if rebound, ok := p.base.Rebind(tool, ctx); ok {
		return rebound, true
	}
	return p.research.Rebind(tool, ctx)
}

func (p *moduCodeToolProvider) ShutdownTools() {
	p.base.ShutdownTools()
}
