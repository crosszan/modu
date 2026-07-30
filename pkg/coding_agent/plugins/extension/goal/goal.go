package goal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/plugins/extension"
	"github.com/openmodu/modu/pkg/types"
)

const (
	goalUsage                   = "Usage: /goal <objective>"
	extensionSessionStart       = "session_start"
	extensionUIReady            = "ui_ready"
	extensionShutdown           = "session_shutdown"
	extensionSubagentChildUsage = "subagent_child_usage"
	goalContinuationType        = "pi-goal-continuation"
	goalBudgetLimitType         = "pi-goal-budget-limit"
	resumeGoalChoice            = "Resume goal"
	leaveGoalPausedChoice       = "Leave paused"
)

// Extension wires persistent /goal support into a CodingSession.
type Extension struct {
	store *Store
	api   extension.ExtensionAPI

	out func(string)

	mu                      sync.Mutex
	agentGoalID             string
	agentMeasuredFrom       time.Time
	agentTurnInProgress     bool
	completedThisTurnGoalID string
	lastSessionStartReason  string
	// verifiedGoalID / unverifiedGoalID mark the goal whose completion the
	// independent verifier most recently confirmed, or could not check
	// (verifier failed to run), so the completion message can say which.
	// Consumed (cleared) when that goal's completion prints.
	verifiedGoalID   string
	unverifiedGoalID string
	// pendingChildUsage accumulates token usage reported by subagent
	// (ForkSession) children during the current agent turn. It is folded
	// into the turn's own usage at agent_end so a goal's budget reflects
	// what its subagents spent, not just the main agent. Reset when taken.
	pendingChildUsage types.AgentUsage
	// watching toggles whether the host UI should render the goal
	// indicator (e.g. statusbar line). Off by default; controlled via
	// /goal-watch [on|off]. Not persisted — every session starts hidden so
	// the statusbar stays clean until the user opts in.
	watching bool
	// verifier configures the maker-checker completion gate; zero value =
	// disabled (update_goal completes without independent verification).
	verifier verifierConfig
}

// Options configures the Extension. The zero value is usable.
type Options struct {
	Out func(string)
}

// New constructs a Goal extension. Pass it into CodingSessionOptions.Extensions.
func New(opts Options) *Extension {
	return &Extension{
		store: NewStore(),
		out:   opts.Out,
	}
}

// init registers goal as a builtin extension. Hosts that want the goal
// extension only need to anonymous-import this package; LoadEnabled then
// picks it up.
//
// The factory uses Options{} (Out=nil) — the modu_code TUI cannot safely
// receive Out writes (writing to stderr corrupts the inline-mode widget
// layout). All goal feedback already reaches the user via api.Notify.
func init() {
	extension.Register("goal", func() extension.Extension {
		return New(Options{})
	})
}

func (e *Extension) Name() string { return "goal" }

// StartAutomationGoal creates the session goal for a host-driven automation
// before the first agent turn. It intentionally does not queue the initial
// hidden continuation because the host is about to send the task prompt; the
// normal agent_end hook will drive follow-up turns until completion.
func (e *Extension) StartAutomationGoal(objective string) (Goal, error) {
	g, err := e.store.Start(objective)
	if err != nil {
		return Goal{}, err
	}
	if g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
	}
	return g, nil
}

// AutomationGoalStatus exposes the current goal status to headless hosts that
// need to wait until hidden continuations have completed or a cap stops them.
func (e *Extension) AutomationGoalStatus() (Status, bool, error) {
	g, ok, err := e.store.CurrentErr()
	if err != nil || !ok {
		return "", ok, err
	}
	return g.Status, true, nil
}

// AutomationVerifierEnabled reports whether update_goal(status=complete) is
// currently gated by the maker-checker verifier for host-driven automations.
func (e *Extension) AutomationVerifierEnabled() bool {
	return e.verifierEnabled()
}

// RuntimeState exposes goal state for RuntimeState JSON and host UIs.
//
// `watching` is the host-UI opt-in: callers that render a statusbar /
// footer (e.g. modu_code TUI) should suppress the goal line unless this
// flag is true. The flag is always present so consumers can rely on the
// key existing even when no goal is set.
func (e *Extension) RuntimeState() any {
	watching := e.isWatching()
	// One read for both the focused goal and the list: this runs on every
	// writeRuntimeState (agent_end, todo changes, background-task changes),
	// so reading the store twice here doubled that cost for nothing.
	all, focused, err := e.store.List()
	if err != nil {
		return map[string]any{
			"active":   false,
			"watching": watching,
			"error":    err.Error(),
		}
	}
	var g Goal
	ok := false
	for _, item := range all {
		if item.ID == focused {
			g, ok = item, true
			break
		}
	}
	if !ok {
		return map[string]any{
			"active":   false,
			"watching": watching,
		}
	}
	state := map[string]any{
		"active":           g.Status == StatusActive,
		"watching":         watching,
		"id":               g.ID,
		"threadId":         g.ThreadID,
		"objective":        g.Objective,
		"status":           g.Status,
		"tokensUsed":       g.TokensUsed,
		"inputTokens":      g.InputTokens,
		"outputTokens":     g.OutputTokens,
		"cacheReadTokens":  g.CacheReadTokens,
		"cacheWriteTokens": g.CacheWriteTokens,
		"turns":            g.Turns,
		"timeUsedSeconds":  g.TimeUsedSeconds,
		"createdAt":        g.CreatedAt,
		"updatedAt":        g.UpdatedAt,
		"indicator":        goalIndicatorText(g),
	}
	if g.TokenBudget != nil {
		state["tokenBudget"] = *g.TokenBudget
		remaining := *g.TokenBudget - g.TokensUsed
		if remaining < 0 {
			remaining = 0
		}
		state["remainingTokens"] = remaining
	}
	if g.CompletedAt != nil {
		state["completedAt"] = *g.CompletedAt
	}
	if len(all) > 1 {
		items := make([]map[string]any, 0, len(all))
		for _, item := range all {
			items = append(items, map[string]any{
				"id":         item.ID,
				"objective":  item.Objective,
				"status":     item.Status,
				"focused":    item.ID == focused,
				"tokensUsed": item.TokensUsed,
			})
		}
		state["goals"] = items
	}
	return state
}

func (e *Extension) Init(api extension.ExtensionAPI) error {
	e.api = api
	e.store.SetRefProvider(func() StoreRef {
		sessionDir := api.SessionDir()
		if sessionDir == "" {
			agentDir := api.AgentDir()
			if agentDir == "" {
				if home, err := os.UserHomeDir(); err == nil && home != "" {
					agentDir = filepath.Join(home, ".pi", "agent")
				} else {
					return StoreRef{}
				}
			}
			return StoreRef{
				BaseDir:  filepath.Join(agentDir, "extensions", "goal", "no-session", cwdStoreKey(api.Cwd())),
				ThreadID: api.SessionID(),
			}
		}
		return StoreRef{
			BaseDir:  filepath.Join(sessionDir, "extensions", "goal"),
			ThreadID: api.SessionID(),
		}
	})

	api.RegisterCommand("goal", "Set, inspect, pause, resume, or clear the persistent goal", e.cmdGoal)
	// Keep the MVP commands as aliases so existing users are not broken.
	api.RegisterCommand("goal-pause", "Pause the active goal", e.cmdPause)
	api.RegisterCommand("goal-resume", "Resume a paused goal and inject one continuation immediately", e.cmdResume)
	api.RegisterCommand("goal-cancel", "Clear the current goal", e.cmdClear)
	api.RegisterCommand("goal-status", "Print the current goal's status", e.cmdStatus)
	api.RegisterCommand("goal-list", "List all goals in this session", e.cmdList)
	api.RegisterCommand("goal-focus", "Switch the focused goal: /goal-focus <number|id>", e.cmdFocus)
	api.RegisterCommand("goal-watch", "Toggle goal indicator in the statusbar: /goal-watch [on|off]", e.cmdWatch)

	api.RegisterTool(&createGoalTool{store: e.store, onCreate: e.beginAgentGoalAccounting})
	api.RegisterTool(&updateGoalTool{
		store:          e.store,
		verify:         e.verifyCompletion,
		beforeComplete: func() { e.accountCurrentAgentTurn(types.AgentUsage{}, false) },
		onComplete:     e.markGoalCompletedThisTurn,
	})
	api.RegisterTool(&getGoalTool{store: e.store})

	api.On(string(types.EventTypeAgentStart), e.onAgentStart)
	api.On(string(types.EventTypeAgentEnd), e.onAgentEnd)
	api.On(extensionSubagentChildUsage, e.onSubagentChildUsage)
	api.On(extensionSessionStart, e.onSessionStart)
	api.On(extensionUIReady, e.onUIReady)
	api.On(extensionShutdown, e.onSessionShutdown)
	return nil
}

func (e *Extension) cmdGoal(args string) error {
	command := parseGoalCommand(args)
	switch command.Kind {
	case parsedGoalShow:
		return e.cmdStatus("")
	case parsedGoalSetStatus:
		if command.Status == StatusPaused {
			return e.cmdPause("")
		}
		return e.cmdResume("")
	case parsedGoalClear:
		return e.cmdClear("")
	case parsedGoalSetObjective:
		return e.cmdSetObjective(command.Objective)
	}
	return nil
}

func (e *Extension) cmdSetObjective(objective string) error {
	current, hasCurrent, err := e.store.CurrentErr()
	if err != nil {
		return err
	}
	// Re-issuing the focused goal's own objective resumes it in place rather
	// than creating a duplicate.
	if hasCurrent && strings.TrimSpace(current.Objective) == strings.TrimSpace(objective) {
		g, err := e.store.ReplaceObjective(objective, nil)
		if err != nil {
			return err
		}
		if g.Status == StatusActive {
			e.beginAgentGoalAccounting(g)
		}
		e.tell(formatGoalActionFeedback(g))
		return e.queueGoalContinuation(g)
	}
	// A new objective adds another goal and focuses it; the previously active
	// goal is parked (the store demotes it), so flush its accounting first.
	if hasCurrent && current.Status == StatusActive {
		e.accountCurrentAgentTurn(types.AgentUsage{}, false)
		e.stopAgentGoalAccounting(current.ID)
	}
	g, err := e.store.AddGoal(objective, nil)
	if err != nil {
		return err
	}
	if g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
	}
	e.tell(formatGoalActionFeedback(g))
	return e.queueGoalContinuation(g)
}

func (e *Extension) cmdList(string) error {
	goals, focused, err := e.store.List()
	if err != nil {
		return err
	}
	if len(goals) == 0 {
		e.tell(goalUsage + "\nNo goals are set.")
		return nil
	}
	e.tell(formatGoalList(goals, focused))
	return nil
}

func (e *Extension) cmdFocus(args string) error {
	goals, _, err := e.store.List()
	if err != nil {
		return err
	}
	id, err := resolveGoalRef(goals, args)
	if err != nil {
		return err
	}
	// Flush the outgoing focused goal's accounting before switching.
	if current, ok, _ := e.store.CurrentErr(); ok && current.Status == StatusActive && current.ID != id {
		e.accountCurrentAgentTurn(types.AgentUsage{}, false)
		e.stopAgentGoalAccounting(current.ID)
	}
	g, err := e.store.Focus(id)
	if err != nil {
		return err
	}
	if g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
	}
	e.tell(formatGoalActionFeedback(g))
	return e.queueGoalContinuation(g)
}

func (e *Extension) cmdPause(string) error {
	e.accountCurrentAgentTurn(types.AgentUsage{}, false)
	g, err := e.store.Pause()
	if err != nil {
		return err
	}
	e.stopAgentGoalAccounting(g.ID)
	e.tell(formatGoalActionFeedback(g))
	return nil
}

func (e *Extension) cmdResume(string) error {
	g, err := e.store.Resume()
	if err != nil {
		return err
	}
	if g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
	}
	e.tell(formatGoalActionFeedback(g))
	return e.queueGoalContinuation(g)
}

func (e *Extension) cmdClear(string) error {
	e.accountCurrentAgentTurn(types.AgentUsage{}, false)
	g, ok, err := e.store.CurrentErr()
	if err != nil {
		return err
	}
	if !ok {
		e.clearAgentGoalAccounting()
		e.tell("No goal to clear\nThis thread does not currently have a goal.")
		return nil
	}
	if _, err := e.store.Cancel(); err != nil {
		return err
	}
	e.stopAgentGoalAccounting(g.ID)
	// Clearing one of several goals re-homes the focus, so say which goal is
	// now current instead of leaving the user to guess.
	if next, ok, err := e.store.CurrentErr(); err == nil && ok {
		e.tell(fmt.Sprintf("Goal cleared\nNow focused: %s %s", goalStatusIcon(next.Status), goalObjectivePreview(next.Objective, 56)))
		return nil
	}
	e.tell("Goal cleared")
	return nil
}

func (e *Extension) cmdStatus(string) error {
	g, ok, err := e.store.CurrentErr()
	if err != nil {
		return err
	}
	if !ok {
		e.tell(goalUsage + "\nNo goal is currently set.")
		return nil
	}
	e.tell(FormatGoalForUser(&g))
	if g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
	}
	return nil
}

// cmdWatch toggles whether the host UI surfaces the goal indicator.
// Bare /goal-watch flips state; explicit on / off / true / false / 1 / 0
// set it. Any other argument is rejected rather than silently treated as
// toggle, so a typo doesn't surprise the user.
func (e *Extension) cmdWatch(args string) error {
	arg := strings.ToLower(strings.TrimSpace(args))
	var next bool
	switch arg {
	case "":
		next = !e.isWatching()
	case "on", "true", "1", "show":
		next = true
	case "off", "false", "0", "hide":
		next = false
	default:
		return fmt.Errorf("goal-watch: expected on|off, got %q", args)
	}
	e.setWatching(next)
	if next {
		e.tell("Goal indicator on (statusbar will show pursuit / pause / budget / done state)")
	} else {
		e.tell("Goal indicator off")
	}
	return nil
}

func (e *Extension) isWatching() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.watching
}

func (e *Extension) setWatching(v bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.watching = v
}

func (e *Extension) onAgentStart(_ types.Event) {
	e.mu.Lock()
	e.agentTurnInProgress = true
	e.completedThisTurnGoalID = ""
	e.mu.Unlock()

	g, ok, err := e.store.CurrentErr()
	if err != nil {
		e.tell(fmt.Sprintf("read failed: %v", err))
		e.clearAgentGoalAccounting()
		return
	}
	if ok && g.Status == StatusActive {
		e.startAgentGoalTurnClock(g)
		return
	}
	e.clearAgentGoalAccounting()
}

func (e *Extension) onSessionStart(event types.Event) {
	e.mu.Lock()
	e.lastSessionStartReason = event.Reason
	e.mu.Unlock()

	g, ok, err := e.store.CurrentErr()
	if err != nil {
		e.tell(fmt.Sprintf("read failed: %v", err))
		e.clearAgentGoalAccounting()
		return
	}
	if ok && g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
		if ShouldQueueContinuationWhenIdle(&g, e.apiIsIdle(), e.apiHasPendingMessages()) {
			if err := e.queueHiddenGoalPrompt(goalContinuationType, BuildContinuationPrompt(g)); err != nil {
				e.tell(fmt.Sprintf("startup continuation inject failed: %v", err))
			}
		}
		return
	}
	e.clearAgentGoalAccounting()
}

func (e *Extension) onUIReady(_ types.Event) {
	g, ok, err := e.store.CurrentErr()
	if err != nil {
		e.tell(fmt.Sprintf("read failed: %v", err))
		return
	}
	if !ok || g.Status != StatusPaused {
		return
	}
	if e.sessionStartReason() != "resume" || !e.apiIsIdle() || e.apiHasPendingMessages() {
		return
	}
	if e.api.Select(fmt.Sprintf("Resume paused goal?\nGoal: %s", g.Objective), []string{resumeGoalChoice, leaveGoalPausedChoice}) != resumeGoalChoice {
		return
	}
	if err := e.cmdResume(""); err != nil {
		e.tell(fmt.Sprintf("resume failed: %v", err))
	}
}

func (e *Extension) onSessionShutdown(_ types.Event) {
	if e.hasAgentGoalAccounting() {
		e.accountCurrentAgentTurn(types.AgentUsage{}, false)
	}
	e.clearAgentGoalAccounting()
}

// onSubagentChildUsage folds a subagent child's token usage into the
// current turn's pending accounting. The host emits this event (carrying
// the child's transcript) when a ForkSession child finishes; without it the
// goal budget would ignore everything subagents spent.
func (e *Extension) onSubagentChildUsage(event types.Event) {
	usage := collectUsage(event.Messages)
	if usage == (types.AgentUsage{}) {
		return
	}
	e.mu.Lock()
	e.pendingChildUsage = addUsage(e.pendingChildUsage, usage)
	e.mu.Unlock()
}

// takePendingChildUsage returns and clears the accumulated subagent usage.
func (e *Extension) takePendingChildUsage() types.AgentUsage {
	e.mu.Lock()
	defer e.mu.Unlock()
	u := e.pendingChildUsage
	e.pendingChildUsage = types.AgentUsage{}
	return u
}

func (e *Extension) onAgentEnd(event types.Event) {
	includeComplete := e.completedGoalThisTurn() != ""
	usage := addUsage(collectUsage(event.Messages), e.takePendingChildUsage())
	g, ok := e.accountCurrentAgentTurn(usage, includeComplete)

	e.mu.Lock()
	e.agentTurnInProgress = false
	e.completedThisTurnGoalID = ""
	e.mu.Unlock()

	if !ok {
		e.clearAgentGoalAccounting()
		return
	}
	if includeComplete && g.Status == StatusComplete {
		e.clearAgentGoalAccounting()
		e.tell(e.completionFeedback(g))
		return
	}
	if g.Status == StatusActive {
		e.beginAgentGoalAccounting(g)
		// agent_end fires inside the agent loop just before IsStreaming
		// flips off, so we must not gate on IsIdle here; the pure
		// after-agent-end policy only cares about pending user messages.
		if ShouldQueueContinuationAfterAgentEnd(&g, e.apiHasPendingMessages()) {
			if err := e.queueHiddenGoalPrompt(goalContinuationType, BuildContinuationPrompt(g)); err != nil {
				e.tell(fmt.Sprintf("continuation inject failed: %v (loop will halt)", err))
			}
		}
		return
	}
	e.clearAgentGoalAccounting()
	if g.Status == StatusBudgetLimited && !e.apiHasPendingMessages() {
		if err := e.queueHiddenGoalPrompt(goalBudgetLimitType, BuildBudgetLimitedPrompt(g)); err != nil {
			e.tell(fmt.Sprintf("budget-limit prompt inject failed: %v", err))
		}
	}
}

// queueGoalContinuation is the slash-command entry point (used by /goal,
// /goal-resume, /goal <new objective>). It uses the WhenIdle policy because
// these handlers can fire while the agent has a pending message queue or is
// otherwise busy and we must not double-prompt.
func (e *Extension) queueGoalContinuation(g Goal) error {
	if !ShouldQueueContinuationWhenIdle(&g, e.apiIsIdle(), e.apiHasPendingMessages()) {
		return nil
	}
	return e.queueHiddenGoalPrompt(goalContinuationType, BuildContinuationPrompt(g))
}

func (e *Extension) queueHiddenGoalPrompt(customType, content string) error {
	if e.api == nil {
		return nil
	}
	return e.api.SendMessageWithOptions(content, extension.MessageOptions{
		CustomType: customType,
		Display:    false,
		DeliverAs:  "followUp",
	})
}

// apiIsIdle and apiHasPendingMessages thin-wrap the API so the nil-api
// case (unit tests using NewStore directly) gracefully degrades to "idle
// and no pending work" - the same default a freshly started, never-driven
// agent would report.
func (e *Extension) apiIsIdle() bool {
	if e.api == nil {
		return true
	}
	return e.api.IsIdle()
}

func (e *Extension) apiHasPendingMessages() bool {
	if e.api == nil {
		return false
	}
	return e.api.HasPendingMessages()
}

// markGoalVerified records that the independent verifier confirmed this
// goal's completion, so the completion message can say so.
func (e *Extension) markGoalVerified(id string) {
	e.mu.Lock()
	e.verifiedGoalID = id
	e.unverifiedGoalID = ""
	e.mu.Unlock()
}

// markGoalUnverified records that the verifier was enabled but could not run,
// so the completion is going through without an independent check.
func (e *Extension) markGoalUnverified(id string) {
	e.mu.Lock()
	e.unverifiedGoalID = id
	e.verifiedGoalID = ""
	e.mu.Unlock()
}

// takeGoalVerificationNote reports (and clears) how id's completion was
// checked: "verified", "unverified", or "" (no verifier involved).
func (e *Extension) takeGoalVerificationNote(id string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case e.verifiedGoalID != "" && e.verifiedGoalID == id:
		e.verifiedGoalID = ""
		return "verified"
	case e.unverifiedGoalID != "" && e.unverifiedGoalID == id:
		e.unverifiedGoalID = ""
		return "unverified"
	default:
		return ""
	}
}

// completionFeedback is the user-facing completion message, noting whether an
// independent verifier confirmed the completion or could not run.
func (e *Extension) completionFeedback(g Goal) string {
	msg := formatGoalActionFeedback(g)
	switch e.takeGoalVerificationNote(g.ID) {
	case "verified":
		msg += "\n✓ Passed an independent completion check"
	case "unverified":
		msg += "\n⚠ Completed without verification — the verifier could not run"
	}
	return msg
}

// beginAgentGoalAccounting marks g as the goal future accounting flushes
// apply to. It is called both mid-turn (create_goal, and re-entrantly by
// onAgentStart/onAgentEnd for the goal already being tracked) and outside any
// turn (slash commands like /goal-resume, /goal-focus, /goal-status,
// session_start). Only the mid-turn case should start the wall clock: a
// command that activates a goal while the agent is idle must leave
// agentMeasuredFrom stopped, or the gap between that command and whenever the
// next turn actually starts — a user reading output, switching goals ahead of
// time — gets billed as work once the next accounting flush reads
// time.Since(agentMeasuredFrom). The next real onAgentStart starts the clock
// via startAgentGoalTurnClock; this only needs to do it when a turn is
// already running around the call (agentTurnInProgress), e.g. a create_goal
// tool call partway through one.
func (e *Extension) beginAgentGoalAccounting(g Goal) {
	if g.Status != StatusActive {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.agentGoalID == g.ID {
		return
	}
	e.agentGoalID = g.ID
	if e.agentTurnInProgress {
		e.agentMeasuredFrom = time.Now()
	} else {
		e.agentMeasuredFrom = time.Time{}
	}
}

// startAgentGoalTurnClock (re)starts the elapsed-time measurement at the start
// of an agent turn. Unlike beginAgentGoalAccounting it always restarts the
// clock, even when the goal is unchanged: only time spent inside a turn counts
// toward the goal, so an idle gap between turns (the user reading output,
// stepping away, or an interrupted run) must not be billed to it.
func (e *Extension) startAgentGoalTurnClock(g Goal) {
	if g.Status != StatusActive {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agentGoalID = g.ID
	e.agentMeasuredFrom = time.Now()
}

func (e *Extension) markGoalCompletedThisTurn(g Goal) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.agentTurnInProgress {
		return
	}
	e.completedThisTurnGoalID = g.ID
	e.agentGoalID = g.ID
	e.agentMeasuredFrom = time.Now()
}

func (e *Extension) stopAgentGoalAccounting(goalID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.agentGoalID == goalID {
		e.agentGoalID = ""
		e.agentMeasuredFrom = time.Time{}
	}
	if e.completedThisTurnGoalID == goalID {
		e.completedThisTurnGoalID = ""
	}
}

func (e *Extension) clearAgentGoalAccounting() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agentGoalID = ""
	e.agentMeasuredFrom = time.Time{}
	e.completedThisTurnGoalID = ""
}

func (e *Extension) completedGoalThisTurn() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.completedThisTurnGoalID
}

func (e *Extension) hasAgentGoalAccounting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.agentGoalID != ""
}

func (e *Extension) sessionStartReason() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastSessionStartReason
}

func (e *Extension) accountCurrentAgentTurn(usage types.AgentUsage, includeComplete bool) (Goal, bool) {
	e.mu.Lock()
	goalID := e.agentGoalID
	measuredFrom := e.agentMeasuredFrom
	e.mu.Unlock()
	if goalID == "" {
		g, ok, err := e.store.CurrentErr()
		if err != nil {
			e.tell(fmt.Sprintf("usage accounting failed: %v", err))
			return Goal{}, false
		}
		return g, ok
	}
	elapsed := int64(0)
	if !measuredFrom.IsZero() {
		elapsed = int64(time.Since(measuredFrom).Round(time.Second).Seconds())
	}
	g, ok, err := e.store.AccountUsage(usage, elapsed, includeComplete, goalID)
	if err != nil {
		e.tell(fmt.Sprintf("usage accounting failed: %v", err))
		return Goal{}, false
	}
	if ok && g.ID == goalID {
		// Stop the clock rather than restarting it: the turn's time is now
		// banked, and whatever happens before the next turn starts (idle
		// time, the user reading output) is not work on this goal. The next
		// agent_start restarts it via startAgentGoalTurnClock; a mid-turn
		// completion restarts it via markGoalCompletedThisTurn.
		e.mu.Lock()
		e.agentMeasuredFrom = time.Time{}
		e.mu.Unlock()
	} else {
		e.clearAgentGoalAccounting()
	}
	return g, ok
}

// addUsage returns the field-wise sum of two usage records, including the
// cost breakdown. Used to merge subagent child usage into a turn's usage.
func addUsage(a, b types.AgentUsage) types.AgentUsage {
	a.Input += b.Input
	a.Output += b.Output
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	a.TotalTokens += b.TotalTokens
	a.Cost.Input += b.Cost.Input
	a.Cost.Output += b.Cost.Output
	a.Cost.CacheRead += b.Cost.CacheRead
	a.Cost.CacheWrite += b.Cost.CacheWrite
	a.Cost.Total += b.Cost.Total
	return a
}

func collectUsage(messages []types.AgentMessage) types.AgentUsage {
	var out types.AgentUsage
	for _, msg := range messages {
		switch m := msg.(type) {
		case types.AssistantMessage:
			out.Input += m.Usage.Input
			out.Output += m.Usage.Output
			out.CacheRead += m.Usage.CacheRead
			out.CacheWrite += m.Usage.CacheWrite
			out.TotalTokens += m.Usage.TotalTokens
			out.Cost.Input += m.Usage.Cost.Input
			out.Cost.Output += m.Usage.Cost.Output
			out.Cost.CacheRead += m.Usage.Cost.CacheRead
			out.Cost.CacheWrite += m.Usage.Cost.CacheWrite
			out.Cost.Total += m.Usage.Cost.Total
		case *types.AssistantMessage:
			if m != nil {
				out.Input += m.Usage.Input
				out.Output += m.Usage.Output
				out.CacheRead += m.Usage.CacheRead
				out.CacheWrite += m.Usage.CacheWrite
				out.TotalTokens += m.Usage.TotalTokens
				out.Cost.Input += m.Usage.Cost.Input
				out.Cost.Output += m.Usage.Cost.Output
				out.Cost.CacheRead += m.Usage.Cost.CacheRead
				out.Cost.CacheWrite += m.Usage.Cost.CacheWrite
				out.Cost.Total += m.Usage.Cost.Total
			}
		}
	}
	return out
}

// goalStatusIcon returns a single-width status glyph for the detail view and
// any other beautified surface.
func goalStatusIcon(status Status) string {
	switch status {
	case StatusActive:
		return "●"
	case StatusPaused:
		return "⏸"
	case StatusBudgetLimited:
		return "⚠"
	case StatusComplete:
		return "✓"
	default:
		return "•"
	}
}

func goalStatusLabel(status Status) string {
	switch status {
	case StatusActive:
		return "active"
	case StatusPaused:
		return "paused"
	case StatusBudgetLimited:
		return "limited by budget"
	case StatusComplete:
		return "complete"
	default:
		return string(status)
	}
}

func goalIndicatorText(g Goal) string {
	icon := goalStatusIcon(g.Status)
	switch g.Status {
	case StatusActive:
		if g.TokenBudget != nil {
			return fmt.Sprintf("%s goal %s/%s", icon, formatTokensCompact(g.TokensUsed), formatTokensCompact(*g.TokenBudget))
		}
		return icon + " goal " + formatElapsed(g.TimeUsedSeconds)
	case StatusPaused:
		return icon + " goal paused"
	case StatusBudgetLimited:
		if g.TokenBudget != nil {
			return fmt.Sprintf("%s goal limited %s/%s", icon, formatTokensCompact(g.TokensUsed), formatTokensCompact(*g.TokenBudget))
		}
		return icon + " goal limited"
	case StatusComplete:
		if g.TokenBudget != nil {
			return fmt.Sprintf("%s goal done %s", icon, formatTokensCompact(g.TokensUsed))
		}
		return icon + " goal done " + formatElapsed(g.TimeUsedSeconds)
	default:
		return string(g.Status)
	}
}

// formatGoalActionFeedback is the concise, single-line echo for a goal action
// (set / resume / pause / complete). The host already prefixes "goal: " and
// the full multi-line card lives behind /goal-status, so setting a goal reads
// as one clean status line instead of a card with meaningless 0 tokens / 0s.
func formatGoalActionFeedback(g Goal) string {
	obj := strings.TrimSpace(strings.Join(strings.Fields(g.Objective), " "))
	if r := []rune(obj); len(r) > 72 {
		obj = string(r[:71]) + "…"
	}
	icon := goalStatusIcon(g.Status)
	switch g.Status {
	case StatusPaused:
		return icon + " paused"
	case StatusComplete:
		// Claude-Code-style compact close: the agent already summarized the
		// work, so the goal line is just the outcome plus elapsed/tokens.
		return strings.TrimRight(fmt.Sprintf("%s Goal achieved %s", icon, goalCompletionStats(g)), " ")
	case StatusBudgetLimited:
		return fmt.Sprintf("%s budget-limited · %s", icon, obj)
	default: // active
		return fmt.Sprintf("%s %s", icon, obj)
	}
}

// goalCompletionStats renders "(37s · 26K tokens)" from the goal's tallies, or
// "" when nothing was recorded.
func goalCompletionStats(g Goal) string {
	var parts []string
	if g.TimeUsedSeconds > 0 {
		parts = append(parts, formatElapsed(g.TimeUsedSeconds))
	}
	if g.Turns > 0 {
		unit := " turns"
		if g.Turns == 1 {
			unit = " turn"
		}
		parts = append(parts, fmt.Sprintf("%d%s", g.Turns, unit))
	}
	if g.TokensUsed > 0 {
		parts = append(parts, formatTokensCompact(g.TokensUsed)+" tokens")
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, " · ") + ")"
}

func (e *Extension) tell(msg string) {
	if e.out != nil {
		e.out(msg)
	}
	if e.api != nil {
		e.api.Notify(e.Name(), msg)
	}
}
