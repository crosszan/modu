package tools

import (
	backendtask "github.com/openmodu/modu/pkg/coding_agent/tools/backend_task"
	"github.com/openmodu/modu/pkg/coding_agent/tools/bash"
	"github.com/openmodu/modu/pkg/coding_agent/tools/common"
	contextremaining "github.com/openmodu/modu/pkg/coding_agent/tools/context_remaining"
	"github.com/openmodu/modu/pkg/coding_agent/tools/edit"
	"github.com/openmodu/modu/pkg/coding_agent/tools/find"
	"github.com/openmodu/modu/pkg/coding_agent/tools/grep"
	"github.com/openmodu/modu/pkg/coding_agent/tools/ls"
	memorytool "github.com/openmodu/modu/pkg/coding_agent/tools/memory"
	"github.com/openmodu/modu/pkg/coding_agent/tools/planning"
	"github.com/openmodu/modu/pkg/coding_agent/tools/read"
	toolresult "github.com/openmodu/modu/pkg/coding_agent/tools/toolresult"
	trajectorytool "github.com/openmodu/modu/pkg/coding_agent/tools/trajectory"
	worktreetool "github.com/openmodu/modu/pkg/coding_agent/tools/worktree"
	"github.com/openmodu/modu/pkg/coding_agent/tools/write"
	"github.com/openmodu/modu/pkg/types"
)

type ToolSet string

const (
	ToolSetCoding   ToolSet = "coding"
	ToolSetReadOnly ToolSet = "read-only"
	ToolSetAll      ToolSet = "all"

	FeatureMemory       = "memory"
	FeatureTodo         = "todo"
	FeatureTaskOutput   = "task_output"
	FeaturePlanMode     = "plan_mode"
	FeatureWorktreeMode = "worktree_mode"

	ValueMemoryStore = "memory_store"
	ValueTodoStore   = "todo_store"
	ValueTaskStore   = "task_store"
	ValuePlanMode    = "plan_mode"
	ValueWorktree    = "worktree"
	ValueContext     = "context_remaining"
	ValueTrajectory  = "trajectory"
	ValueArtifacts   = "artifacts"
	ValueWebSearch   = "web_search"
	ValueWebFetch    = "web_fetch"
	ValueRewind      = "rewind_snapshots"
)

type DefaultProvider struct {
	Set       ToolSet
	readState *common.FileReadState
	bashJobs  *bash.JobStore
}

func NewProvider(set ToolSet) DefaultProvider {
	if set == "" {
		set = ToolSetCoding
	}
	return DefaultProvider{Set: set, readState: common.NewFileReadState(), bashJobs: bash.NewJobStore()}
}

func (p DefaultProvider) Tools(ctx types.ToolContext) []types.Tool {
	readState := p.state()
	artifacts, _ := ctx.Value(ValueArtifacts).(*common.ArtifactStore)
	snapshots, _ := ctx.Value(ValueRewind).(common.SnapshotRecorder)
	out := append([]types.Tool{}, ctx.BaseTools...)
	if ctx.BaseTools == nil {
		out = p.baseTools(ctx.Cwd, readState, artifacts, snapshots)
	}
	if artifacts != nil && !containsTool(out, "read_tool_result") {
		out = append(out, toolresult.NewTool(artifacts))
	}
	out = append(out, ctx.ExtraTools...)
	if ctx.FeatureEnabled(FeatureMemory) {
		out = append(out, memorytool.NewMemoryTool(valueAs[memorytool.MemoryStore](ctx, ValueMemoryStore)))
	}
	if ctx.FeatureEnabled(FeatureTodo) {
		out = append(out, planning.NewTodoWriteTool(valueAs[planning.TodoStore](ctx, ValueTodoStore)))
	}
	if ctx.FeatureEnabled(FeatureTaskOutput) {
		out = append(out, backendtask.NewTaskOutputTool(valueAs[backendtask.BackgroundTaskStore](ctx, ValueTaskStore)))
	}
	if ctx.FeatureEnabled(FeaturePlanMode) {
		planMode := valueAs[planning.PlanModeManager](ctx, ValuePlanMode)
		out = append(out, planning.NewEnterPlanModeTool(planMode), planning.NewExitPlanModeTool(planMode))
	}
	if ctx.FeatureEnabled(FeatureWorktreeMode) {
		worktree := valueAs[worktreetool.WorktreeManager](ctx, ValueWorktree)
		out = append(out, worktreetool.NewEnterWorktreeTool(worktree), worktreetool.NewExitWorktreeTool(worktree))
	}
	out = append(out, contextremaining.New(valueAs[contextremaining.Provider](ctx, ValueContext)))
	out = append(out, trajectorytool.New(valueAs[trajectorytool.Provider](ctx, ValueTrajectory)))
	return out
}

func (p DefaultProvider) Rebind(tool types.Tool, ctx types.ToolContext) (types.Tool, bool) {
	readState := p.state()
	artifacts, _ := ctx.Value(ValueArtifacts).(*common.ArtifactStore)
	snapshots, _ := ctx.Value(ValueRewind).(common.SnapshotRecorder)
	switch tool.Name() {
	case "read":
		return read.NewTrackedTool(ctx.Cwd, readState), true
	case "write":
		return write.NewTrackedToolWithSnapshots(ctx.Cwd, readState, snapshots), true
	case "edit":
		return edit.NewTrackedToolWithSnapshots(ctx.Cwd, readState, snapshots), true
	case "bash":
		return bash.NewToolWithStore(ctx.Cwd, artifacts, p.jobs()), true
	case "bash_output":
		return bash.NewBashOutputTool(p.jobs()), true
	case "kill_bash":
		return bash.NewKillBashTool(p.jobs()), true
	case "grep":
		return grep.NewToolWithArtifacts(ctx.Cwd, artifacts), true
	case "find":
		return find.NewToolWithArtifacts(ctx.Cwd, artifacts), true
	case "ls":
		return ls.NewToolWithArtifacts(ctx.Cwd, artifacts), true
	case "read_tool_result":
		return toolresult.NewTool(artifacts), true
	case "memo":
		return memorytool.NewMemoryTool(valueAs[memorytool.MemoryStore](ctx, ValueMemoryStore)), true
	default:
		return nil, false
	}
}

func (p DefaultProvider) state() *common.FileReadState {
	if p.readState != nil {
		return p.readState
	}
	return common.NewFileReadState()
}

func (p DefaultProvider) jobs() *bash.JobStore {
	if p.bashJobs != nil {
		return p.bashJobs
	}
	return bash.NewJobStore()
}

func (p DefaultProvider) ShutdownTools() {
	if p.bashJobs != nil {
		p.bashJobs.KillAll()
	}
}

func valueAs[T any](ctx types.ToolContext, name string) T {
	v, _ := ctx.Value(name).(T)
	return v
}

func containsTool(tools []types.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name() == name {
			return true
		}
	}
	return false
}

func (p DefaultProvider) baseTools(cwd string, readState *common.FileReadState, artifacts *common.ArtifactStore, snapshots common.SnapshotRecorder) []types.Tool {
	if readState == nil {
		readState = p.state()
	}
	switch p.Set {
	case ToolSetReadOnly:
		return []types.Tool{
			read.NewTrackedTool(cwd, readState),
			grep.NewToolWithArtifacts(cwd, artifacts),
			find.NewToolWithArtifacts(cwd, artifacts),
			ls.NewToolWithArtifacts(cwd, artifacts),
		}
	case ToolSetAll:
		out := []types.Tool{
			read.NewTrackedTool(cwd, readState),
			write.NewTrackedToolWithSnapshots(cwd, readState, snapshots),
			edit.NewTrackedToolWithSnapshots(cwd, readState, snapshots),
		}
		out = append(out, bash.NewTools(cwd, artifacts, p.jobs())...)
		return append(out,
			grep.NewToolWithArtifacts(cwd, artifacts),
			find.NewToolWithArtifacts(cwd, artifacts),
			ls.NewToolWithArtifacts(cwd, artifacts),
		)
	default:
		out := []types.Tool{
			read.NewTrackedTool(cwd, readState),
		}
		out = append(out, bash.NewTools(cwd, artifacts, p.jobs())...)
		return append(out,
			edit.NewTrackedToolWithSnapshots(cwd, readState, snapshots),
			write.NewTrackedToolWithSnapshots(cwd, readState, snapshots),
			grep.NewToolWithArtifacts(cwd, artifacts),
			find.NewToolWithArtifacts(cwd, artifacts),
			ls.NewToolWithArtifacts(cwd, artifacts),
		)
	}
}

// CodingTools returns the core coding tools: read, bash, edit, write.
func CodingTools(cwd string) []types.Tool {
	return NewProvider(ToolSetCoding).baseTools(cwd, nil, nil, nil)
}

// ReadOnlyTools returns read-only tools: read, grep, find, ls.
func ReadOnlyTools(cwd string) []types.Tool {
	return NewProvider(ToolSetReadOnly).baseTools(cwd, nil, nil, nil)
}

// AllTools returns all available built-in coding tools.
func AllTools(cwd string) []types.Tool {
	return NewProvider(ToolSetAll).baseTools(cwd, nil, nil, nil)
}
