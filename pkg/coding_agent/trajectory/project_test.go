package trajectory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSession writes JSONL lines in the on-disk session format and returns the
// path. Lines are written verbatim so the fixtures document the real shape the
// session manager produces.
func writeSession(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	return path
}

const (
	fixtureHeader = `{"type":"session","version":3,"id":"sess-1","timestamp":"2026-01-01T00:00:00Z","cwd":"/work"}`
	fixtureUser   = `{"id":"u1","parentId":null,"timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"fix the build"},"type":"message"}`
	fixtureAssist = `{"id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"check the failure first"},{"type":"text","text":"Looking at the build."},{"type":"toolCall","id":"call-1","name":"bash","arguments":{"command":"go build ./..."}}],"provider":"deepseek","model":"deepseek-v4","usage":{"input":1000,"output":50,"totalTokens":1050,"cost":{"total":0.5}}},"type":"message"}`
	fixtureResult = `{"id":"r1","parentId":"a1","timestamp":"2026-01-01T00:00:05Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"bash","content":[{"type":"text","text":"build ok"}],"isError":false},"type":"message"}`
	fixtureFinal  = `{"id":"a2","parentId":"r1","timestamp":"2026-01-01T00:00:06Z","message":{"role":"assistant","content":[{"type":"text","text":"Build passes."}],"provider":"deepseek","model":"deepseek-v4","usage":{"input":1200,"output":20,"totalTokens":1220}},"type":"message"}`
)

func fullSession(t *testing.T) Trajectory {
	t.Helper()
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, fixtureFinal)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	return result
}

func TestProjectBuildsTurnsAndSteps(t *testing.T) {
	result := fullSession(t)

	if result.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", result.SchemaVersion, SchemaVersion)
	}
	if result.Session.ID != "sess-1" || result.Session.Cwd != "/work" {
		t.Errorf("session = %+v, want id sess-1 cwd /work", result.Session)
	}
	if result.Session.Title != "fix the build" {
		t.Errorf("Title = %q, want %q", result.Session.Title, "fix the build")
	}
	if result.Session.Model != "deepseek-v4" || result.Session.Provider != "deepseek" {
		t.Errorf("model = %q/%q, want deepseek/deepseek-v4", result.Session.Provider, result.Session.Model)
	}
	if len(result.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(result.Turns))
	}

	turn := result.Turns[0]
	// Model output resumes after the tool result, which opens step 2.
	if turn.Steps != 2 {
		t.Errorf("Steps = %d, want 2", turn.Steps)
	}
	if turn.DurationMs != 6000 {
		t.Errorf("DurationMs = %d, want 6000", turn.DurationMs)
	}
	if turn.FirstResponseMs == nil || *turn.FirstResponseMs != 2000 {
		t.Errorf("FirstResponseMs = %v, want 2000", turn.FirstResponseMs)
	}
	if turn.Status != StatusComplete {
		t.Errorf("Status = %q, want %q", turn.Status, StatusComplete)
	}
	if turn.ToolCalls != 1 || turn.Failures != 0 {
		t.Errorf("toolCalls/failures = %d/%d, want 1/0", turn.ToolCalls, turn.Failures)
	}
}

func TestProjectRecordKindsAndSteps(t *testing.T) {
	result := fullSession(t)

	type want struct {
		kind string
		step int
	}
	expected := []want{
		{KindUser, 0},
		{KindReasoning, 1},
		{KindAssistant, 1},
		{KindTool, 1},
		{KindAssistant, 2},
	}
	if len(result.Records) != len(expected) {
		t.Fatalf("records = %d, want %d: %+v", len(result.Records), len(expected), result.Records)
	}
	for i, record := range result.Records {
		if record.Kind != expected[i].kind || record.Step != expected[i].step {
			t.Errorf("record %d = %s/step %d, want %s/step %d",
				i, record.Kind, record.Step, expected[i].kind, expected[i].step)
		}
		if record.Index != i+1 {
			t.Errorf("record %d Index = %d, want %d", i, record.Index, i+1)
		}
		if record.Turn != 1 {
			t.Errorf("record %d Turn = %d, want 1", i, record.Turn)
		}
	}
}

func TestProjectPairsToolCallWithResult(t *testing.T) {
	result := fullSession(t)

	var tool *Record
	for i := range result.Records {
		if result.Records[i].Kind == KindTool {
			tool = &result.Records[i]
		}
	}
	if tool == nil {
		t.Fatal("no tool record")
	}
	if tool.Status != StatusComplete {
		t.Errorf("Status = %q, want %q", tool.Status, StatusComplete)
	}
	if tool.DurationMs == nil || *tool.DurationMs != 3000 {
		t.Errorf("DurationMs = %v, want 3000", tool.DurationMs)
	}
	if tool.ToolName != "bash" || tool.CallID != "call-1" {
		t.Errorf("tool = %q/%q, want bash/call-1", tool.ToolName, tool.CallID)
	}

	if len(result.Stats.Tools) != 1 {
		t.Fatalf("tool stats = %d, want 1", len(result.Stats.Tools))
	}
	stat := result.Stats.Tools[0]
	if stat.Name != "bash" || stat.Calls != 1 || stat.TotalMs != 3000 || stat.MaxMs != 3000 || stat.Unfinished != 0 {
		t.Errorf("tool stat = %+v", stat)
	}
}

func TestProjectAggregatesTokens(t *testing.T) {
	result := fullSession(t)

	// Input is billed input and is summed; ContextTokens is the last reported
	// total, not a sum.
	if result.Stats.Tokens.Input != 2200 || result.Stats.Tokens.Output != 70 {
		t.Errorf("tokens = %+v, want input 2200 output 70", result.Stats.Tokens)
	}
	if result.Stats.ContextTokens != 1220 {
		t.Errorf("ContextTokens = %d, want 1220", result.Stats.ContextTokens)
	}
	if result.Stats.Tokens.Cost != 0.5 {
		t.Errorf("Cost = %v, want 0.5", result.Stats.Tokens.Cost)
	}
	if result.Stats.Steps != 2 || result.Stats.Turns != 1 || result.Stats.Reasoning != 1 {
		t.Errorf("stats = %+v", result.Stats)
	}
}

func TestProjectMarksFailedTool(t *testing.T) {
	failed := `{"id":"r1","parentId":"a1","timestamp":"2026-01-01T00:00:05Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"bash","content":[{"type":"text","text":"exit 1"}],"isError":true},"type":"message"}`
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, failed)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if result.Stats.ToolFailures != 1 {
		t.Errorf("ToolFailures = %d, want 1", result.Stats.ToolFailures)
	}
	if result.Turns[0].Failures != 1 {
		t.Errorf("turn failures = %d, want 1", result.Turns[0].Failures)
	}
	if got := result.Stats.Tools[0].Failures; got != 1 {
		t.Errorf("tool stat failures = %d, want 1", got)
	}
}

func TestProjectLeavesUnfinishedToolRunning(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if result.Turns[0].Status != StatusRunning {
		t.Errorf("turn status = %q, want %q", result.Turns[0].Status, StatusRunning)
	}
	last := result.Records[len(result.Records)-1]
	if last.Status != StatusRunning || last.DurationMs != nil {
		t.Errorf("tool record = %+v, want running with no duration", last)
	}
	if result.Stats.Tools[0].Unfinished != 1 {
		t.Errorf("Unfinished = %d, want 1", result.Stats.Tools[0].Unfinished)
	}
}

func TestProjectSkipsSidecarEntries(t *testing.T) {
	// Runtime and plan snapshots are appended without moving the leaf, so they
	// must not appear as records nor break the parent chain.
	runtime := `{"id":"s1","parentId":"u1","state":{"model":{"id":"x"}},"timestamp":"2026-01-01T00:00:01Z","type":"runtime_state"}`
	plan := `{"id":"s2","parentId":"u1","plan":{"content":"do it"},"timestamp":"2026-01-01T00:00:01Z","type":"plan_snapshot"}`
	path := writeSession(t, fixtureHeader, fixtureUser, runtime, plan, fixtureAssist, fixtureResult, fixtureFinal)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(result.Records) != 5 {
		t.Fatalf("records = %d, want 5", len(result.Records))
	}
	for _, record := range result.Records {
		if record.ID == "s1" || record.ID == "s2" {
			t.Errorf("sidecar entry %s leaked into records", record.ID)
		}
	}
}

func TestProjectFollowsCurrentBranchOnly(t *testing.T) {
	// Two children of u1: the abandoned branch must not appear.
	abandoned := `{"id":"x1","parentId":"u1","timestamp":"2026-01-01T00:00:03Z","message":{"role":"assistant","content":[{"type":"text","text":"abandoned answer"}]},"type":"message"}`
	path := writeSession(t, fixtureHeader, fixtureUser, abandoned, fixtureAssist, fixtureResult, fixtureFinal)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	for _, record := range result.Records {
		if strings.Contains(record.Summary, "abandoned") {
			t.Fatalf("abandoned branch record leaked: %+v", record)
		}
	}
	if len(result.Records) != 5 {
		t.Errorf("records = %d, want 5", len(result.Records))
	}
}

func TestProjectDetailLevels(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, fixtureFinal)

	summary, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if summary.DetailLevel != DetailSummary {
		t.Errorf("DetailLevel = %q, want %q", summary.DetailLevel, DetailSummary)
	}
	for _, record := range summary.Records {
		if record.Input != "" || record.Output != "" {
			t.Errorf("summary detail leaked payload: %+v", record)
		}
	}

	full, err := Project(path, Options{Detail: DetailFull})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var tool Record
	for _, record := range full.Records {
		if record.Kind == KindTool {
			tool = record
		}
	}
	if !strings.Contains(tool.Input, "go build") {
		t.Errorf("Input = %q, want the tool arguments", tool.Input)
	}
	if tool.Output != "build ok" {
		t.Errorf("Output = %q, want %q", tool.Output, "build ok")
	}
}

func TestProjectMaxRecordsKeepsTailWithStableIndexes(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, fixtureFinal)
	// MaxRecords clamps up to the floor of 50, so use a session larger than the
	// floor only where it matters; here we assert the clamp itself.
	result, err := Project(path, Options{MaxRecords: 1})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(result.Records) != 5 {
		t.Errorf("records = %d, want 5 (MaxRecords clamps up to %d)", len(result.Records), minMaxRecords)
	}
	if result.Stats.Records != 5 {
		t.Errorf("Stats.Records = %d, want 5", result.Stats.Records)
	}
}

func TestProjectReportsMalformedLine(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, `{"id":"broken"`, fixtureAssist)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Line != 3 {
		t.Fatalf("warnings = %+v, want one warning on line 3", result.Warnings)
	}
	if len(result.Records) == 0 {
		t.Error("a malformed line should not discard the rest of the session")
	}
}

func TestProjectRecordsCompaction(t *testing.T) {
	compaction := `{"id":"c1","parentId":"a1","newCount":6,"originalCount":107,"summary":"...","timestamp":"2026-01-01T00:00:07Z","type":"compaction"}`
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, compaction)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	last := result.Records[len(result.Records)-1]
	if last.Kind != KindCompaction {
		t.Fatalf("last record kind = %q, want %q", last.Kind, KindCompaction)
	}
	if last.Summary != "context compacted: 107 → 6 messages" {
		t.Errorf("Summary = %q", last.Summary)
	}
	if result.Stats.Compactions != 1 {
		t.Errorf("Compactions = %d, want 1", result.Stats.Compactions)
	}
}

func TestProjectMarksSubagentTools(t *testing.T) {
	spawn := `{"id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-9","name":"subagent","arguments":{"prompt":"go look"}}]},"type":"message"}`
	path := writeSession(t, fixtureHeader, fixtureUser, spawn)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	last := result.Records[len(result.Records)-1]
	if last.Kind != KindSubagent {
		t.Errorf("kind = %q, want %q", last.Kind, KindSubagent)
	}
}

func TestProjectStartsSyntheticTurnForBranchedSession(t *testing.T) {
	// A branched session can begin mid-conversation with model output.
	orphan := `{"id":"a1","parentId":null,"timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"continuing"}]},"type":"message"}`
	path := writeSession(t, fixtureHeader, orphan)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(result.Turns) != 1 || result.Turns[0].Prompt != "(continued)" {
		t.Fatalf("turns = %+v, want one synthetic turn", result.Turns)
	}
	if result.Session.Title != "(no prompt)" {
		t.Errorf("Title = %q, want %q", result.Session.Title, "(no prompt)")
	}
}

func TestShortenDoesNotSplitRunes(t *testing.T) {
	got := shorten("轨迹投影中文摘要", 4)
	if got != "轨迹投影…" {
		t.Errorf("shorten = %q, want %q", got, "轨迹投影…")
	}
	if strings.ContainsRune(got, '�') {
		t.Errorf("shorten produced a replacement rune: %q", got)
	}
}

func TestProjectMissingFile(t *testing.T) {
	if _, err := Project(filepath.Join(t.TempDir(), "nope.jsonl"), Options{}); err == nil {
		t.Fatal("expected an error for a missing session file")
	}
}

// fixtureTimed is an assistant reply from a session written after model-call
// timing was persisted: the request began at :00.500, the first token arrived
// at :01.500, and decoding finished at :03.500.
const fixtureTimed = `{"id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:04Z","message":{"role":"assistant","content":[{"type":"text","text":"Answer."}],"provider":"deepseek","model":"deepseek-v4","usage":{"input":1000,"output":40,"totalTokens":1040},"timing":{"requestStartMs":1767225600500,"firstTokenMs":1767225601500,"completedMs":1767225603500}},"type":"message"}`

func TestProjectPrefersRecordedModelTiming(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureTimed)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	record := result.Records[len(result.Records)-1]
	if record.Timing != TimingMeasured {
		t.Errorf("Timing = %q, want %q", record.Timing, TimingMeasured)
	}
	// The span runs request start to completion, not to when the entry landed.
	if record.DurationMs == nil || *record.DurationMs != 3000 {
		t.Errorf("DurationMs = %v, want 3000", record.DurationMs)
	}
	if record.FirstTokenMs == nil || *record.FirstTokenMs != 1000 {
		t.Errorf("FirstTokenMs = %v, want 1000", record.FirstTokenMs)
	}
	if record.DecodeMs == nil || *record.DecodeMs != 2000 {
		t.Errorf("DecodeMs = %v, want 2000", record.DecodeMs)
	}
	// 40 output tokens decoded over 2s.
	if record.Throughput == nil || *record.Throughput != 20 {
		t.Errorf("Throughput = %v, want 20", record.Throughput)
	}
	turn := result.Turns[0]
	if turn.FirstTokenMs == nil || *turn.FirstTokenMs != 1500 {
		t.Errorf("turn FirstTokenMs = %v, want 1500", turn.FirstTokenMs)
	}
}

func TestProjectFallsBackToDerivedTimingOnOlderSessions(t *testing.T) {
	// A session written before timing was persisted must still project, with
	// its inferred start labelled as such.
	result := fullSession(t)
	var model *Record
	for i := range result.Records {
		if result.Records[i].Kind == KindReasoning {
			model = &result.Records[i]
		}
	}
	if model == nil {
		t.Fatal("no model record")
	}
	if model.Timing != TimingDerived {
		t.Errorf("Timing = %q, want %q", model.Timing, TimingDerived)
	}
	if model.FirstTokenMs != nil || model.DecodeMs != nil || model.Throughput != nil {
		t.Errorf("derived timing must not invent a token split: %+v", model)
	}
	if result.Turns[0].FirstTokenMs != nil {
		t.Errorf("turn FirstTokenMs = %v, want nil without recorded timing", result.Turns[0].FirstTokenMs)
	}
}

const (
	fixturePromptInitial = `{"id":"p1","parentId":null,"timestamp":"2026-01-01T00:00:00Z","prompt":{"system":"You are modu.","tools":[{"name":"bash","description":"Run a command","schema":"{}"}],"change":"initial"},"type":"prompt_snapshot"}`
	fixturePromptChanged = `{"id":"p2","parentId":"a1","timestamp":"2026-01-01T00:00:03Z","prompt":{"system":"You are modu, in plan mode.","tools":[{"name":"bash","description":"Run a command","schema":"{}"}],"change":"system"},"type":"prompt_snapshot"}`
)

func TestProjectMergesPromptSnapshotsIntoTheLedger(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixturePromptInitial, fixtureUser,
		fixtureAssist, fixturePromptChanged, fixtureResult, fixtureFinal)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}

	var system []Record
	for _, record := range result.Records {
		if record.Kind == KindSystem {
			system = append(system, record)
		}
	}
	if len(system) != 2 {
		t.Fatalf("system records = %d, want 2: %+v", len(system), result.Records)
	}
	if result.Stats.PromptChanges != 2 {
		t.Errorf("PromptChanges = %d, want 2", result.Stats.PromptChanges)
	}

	first, second := system[0], system[1]
	if first.Prompt == nil || first.Prompt.Change != PromptChangeInitial {
		t.Fatalf("first snapshot = %+v, want the initial capture", first.Prompt)
	}
	if first.Prompt.PreviousSystem != "" {
		t.Error("the initial capture has nothing to diff against")
	}
	if second.Prompt == nil || second.Prompt.Change != PromptChangeSystem {
		t.Fatalf("second snapshot = %+v, want a system change", second.Prompt)
	}
	// The previous prompt travels with the change so it can be diffed.
	if second.Prompt.PreviousSystem != "You are modu." {
		t.Errorf("PreviousSystem = %q", second.Prompt.PreviousSystem)
	}
	if len(second.Prompt.Tools) != 1 || second.Prompt.Tools[0].Name != "bash" {
		t.Errorf("tools = %+v", second.Prompt.Tools)
	}

	// Merged by timestamp: the change landed between the model reply and the
	// tool result, and the sidecar must not have broken the branch walk.
	if first.Index != 1 {
		t.Errorf("initial snapshot index = %d, want it first", first.Index)
	}
	var kinds []string
	for _, record := range result.Records {
		kinds = append(kinds, record.Kind)
	}
	want := []string{KindSystem, KindUser, KindReasoning, KindAssistant, KindTool, KindSystem, KindAssistant}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("record order = %v, want %v", kinds, want)
	}
}

func TestProjectDrainsTrailingPromptSnapshots(t *testing.T) {
	// A prompt change after the last message must still appear.
	trailing := `{"id":"p9","parentId":"a2","timestamp":"2026-01-01T00:09:00Z","prompt":{"system":"Later.","change":"system"},"type":"prompt_snapshot"}`
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, fixtureFinal, trailing)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	last := result.Records[len(result.Records)-1]
	if last.Kind != KindSystem {
		t.Errorf("last record = %q, want a trailing system record", last.Kind)
	}
}

func TestProjectKeepsPreSessionSnapshotOutsideEveryTurn(t *testing.T) {
	// The first snapshot is written before the session's opening message, so
	// it belongs to no turn — inventing an empty one for it would report a
	// turn that never happened.
	path := writeSession(t, fixtureHeader, fixturePromptInitial, fixtureUser, fixtureAssist)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if result.Stats.Turns != 1 {
		t.Errorf("Turns = %d, want 1", result.Stats.Turns)
	}
	if result.Records[0].Kind != KindSystem || result.Records[0].Turn != 0 {
		t.Errorf("leading record = %s/turn %d, want system/turn 0",
			result.Records[0].Kind, result.Records[0].Turn)
	}
	if result.Turns[0].Prompt != "fix the build" {
		t.Errorf("first turn = %q, want the real prompt", result.Turns[0].Prompt)
	}
}

func TestProjectCapturesSubagentTaskID(t *testing.T) {
	spawn := `{"id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-9","name":"subagent","arguments":{"prompt":"go look"}}]},"type":"message"}`
	// The subagent tool reports the background task its child runs under; that
	// id is the parent session's only link to the child's own session.
	result := `{"id":"r1","parentId":"a1","timestamp":"2026-01-01T00:00:09Z","message":{"role":"toolResult","toolCallId":"call-9","toolName":"subagent","content":[{"type":"text","text":"started"}],"details":{"task_id":"task-7","subagent":"explorer","status":"running"},"isError":false},"type":"message"}`
	path := writeSession(t, fixtureHeader, fixtureUser, spawn, result)
	projected, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	record := projected.Records[len(projected.Records)-1]
	if record.Kind != KindSubagent {
		t.Fatalf("kind = %q, want %q", record.Kind, KindSubagent)
	}
	if record.Subagent == nil {
		t.Fatal("no subagent run captured")
	}
	if record.Subagent.TaskID != "task-7" || record.Subagent.Agent != "explorer" {
		t.Errorf("run = %+v, want task-7/explorer", record.Subagent)
	}
	// The projection alone cannot reach the child session, so it must not
	// claim the run's statistics are available.
	if record.Subagent.Available {
		t.Error("a bare projection cannot know the child session is readable")
	}
	if projected.Stats.Subagents != 1 {
		t.Errorf("Subagents = %d, want 1", projected.Stats.Subagents)
	}
}

func TestDescribeSubagentStatesUnavailableReason(t *testing.T) {
	text := describeSubagent(&SubagentRun{
		Agent: "explorer", TaskID: "task-7", Reason: "this run recorded no session file"})
	// The task id is the handle for `/trajectory task <id>`, so it must show
	// even when the run itself could not be read.
	for _, want := range []string{"explorer", "task-7", "no session file"} {
		if !strings.Contains(text, want) {
			t.Errorf("describeSubagent missing %q: %q", want, text)
		}
	}
	available := describeSubagent(&SubagentRun{
		Agent: "explorer", Available: true, Turns: 2, Steps: 5, ToolCalls: 7,
		ActiveMs: 4000, Tokens: Usage{Input: 1000, Output: 200}, Failures: 1,
	})
	for _, want := range []string{"explorer", "2 turns", "7 tools", "4.0s", "1,000 in", "1 failed"} {
		if !strings.Contains(available, want) {
			t.Errorf("describeSubagent missing %q: %q", want, available)
		}
	}
}
