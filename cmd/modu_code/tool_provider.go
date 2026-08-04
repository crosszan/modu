package main

import (
	codingtools "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/types"
)

type moduCodeToolProvider struct {
	base codingtools.DefaultProvider
}

func newModuCodeToolProvider() types.ToolManager {
	return &moduCodeToolProvider{
		base: codingtools.NewProvider(codingtools.ToolSetCoding),
	}
}

func (p *moduCodeToolProvider) Tools(ctx types.ToolContext) []types.Tool {
	return append(p.base.Tools(ctx), codingtools.ResearchTools(ctx)...)
}

func (p *moduCodeToolProvider) Rebind(tool types.Tool, ctx types.ToolContext) (types.Tool, bool) {
	return p.base.Rebind(tool, ctx)
}

func (p *moduCodeToolProvider) ShutdownTools() {
	p.base.ShutdownTools()
}
