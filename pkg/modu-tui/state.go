package modutui

import (
	"time"

	"github.com/charmbracelet/glamour"
)

// transcriptModel owns the conversation viewport and its rendered/copyable
// representation. It does not own input, overlays, or host callbacks.
type transcriptModel struct {
	entries []Entry
	lines   []string
	gutters []int
	// copyBlocks parallels lines. Repeated non-empty values identify one
	// contiguous rendered block with a semantic whole-block representation.
	copyBlocks []string

	headers map[int]int
	yOffset int
	follow  bool
	unseen  int

	selecting        bool
	selStart, selEnd cell
	dragCol          int
	// pendingToggle is the entry index of a collapsible block whose header
	// line was pressed, or -1. The press only becomes a collapse/expand if
	// the pointer never moved; a press that turns into a drag is a text
	// selection instead. Resolved on mouse release.
	pendingToggle int
	autoScroll       int
	autoScrolling    bool
	autoScrollTicks  int

	infoCardLines       []string
	blockFactories      []EntryBlockFactory
	blockGap            int
	toolArtifactCache   map[string]toolArtifactCacheEntry
	toolArtifactLoading map[string]bool
	loadToolArtifact    func(string) (string, error)

	// blockRenderCache holds the last rendered lines for each finalized entry
	// (or contiguous batched-tool-call group), keyed by entry ID (or "idx:N"
	// / "group:..." for entries without one). buildTranscript reuses a cached
	// render instead of re-running Block.Render (and, for markdown, glamour's
	// full parse) when the source entries and content width are unchanged
	// (reflect.DeepEqual against the snapshot taken at cache time — cheaper
	// than re-serializing the entries to a string on every check, and, being
	// a structural comparison rather than hand-picked fields, automatically
	// stays correct as Node kinds gain new fields), so redraws only pay for
	// the entry that actually changed rather than re-rendering the whole
	// transcript every time.
	blockRenderCache map[string]blockRenderCacheEntry

	// markdownRenderers caches glamour.TermRenderer by content width so
	// buildTranscript doesn't reconstruct one (style config + chroma lexer
	// setup) on every redraw — terminal width rarely changes within a
	// session.
	markdownRenderers map[int]*glamour.TermRenderer

	// streamRenderPending/streamRenderTicking implement streamRenderThrottle:
	// a live-streaming entry update sets streamRenderPending instead of
	// rebuilding immediately, and a streamRenderTickMsg loop (armed by
	// ensureStreamRenderRunning, guarded from double-arming by
	// streamRenderTicking) flushes it on a schedule.
	streamRenderPending bool
	streamRenderTicking bool
}

type blockRenderCacheEntry struct {
	entries []Entry
	width   int
	lines   []RenderedLine
}

// composerModel owns editable input and command completion state.
type composerModel struct {
	input        InputBlock
	inputHistory []string
	historyIdx   int
	historyHold  string
	imeTail      string
	imeActive    bool

	arrowKeysScroll       bool
	slashCommands         []SlashCommand
	slashCommandsProvider func() []SlashCommand
	slashMatches          []SlashCommand
	slashIndex            int

	// atFilesProvider resolves an in-progress "@query" token to candidate
	// paths (see Services.ListFiles). atMatches/atIndex mirror
	// slashMatches/slashIndex, but the results already come pre-filtered
	// from the host rather than being filtered client-side from a static
	// list, since a project's file count doesn't fit the "load everything
	// once" pattern slash commands use.
	atFilesProvider func(query string) []string
	atMatches       []string
	atIndex         int
}

// overlayModel owns the single focused surface above the normal composer.
// All transitions are centralized here so only one overlay can be active.
type overlayModel struct {
	panel         *Panel
	panelLines    []string
	panelRowLines []int
	panelOffset   int
	panelSelected int
	approval      *pendingApproval
	humanPrompt   *pendingHumanPrompt
	humanText     *pendingHumanText
}

// chromeModel owns fixed status/footer/todo state and simulated streaming.
type chromeModel struct {
	streaming   bool
	streamRunes []rune
	streamIdx   int
	streamReply string
	busy        bool

	// spinnerFrame indexes into spinnerFrames for the animated status glyph
	// shown while streaming or busy. spinnerRunning guards against arming a
	// second concurrent tick loop when both states are already active.
	spinnerFrame   int
	spinnerRunning bool

	todos        []TodoItem
	todosCurrent bool

	status            string
	statusExpiresAt   time.Time
	statusExpiresText string
	statusHint        string
	footer            string
}
