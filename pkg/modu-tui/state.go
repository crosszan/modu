package modutui

import "time"

// transcriptModel owns the conversation viewport and its rendered/copyable
// representation. It does not own input, overlays, or host callbacks.
type transcriptModel struct {
	entries []Entry
	lines   []string
	gutters []int
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

	// blockRenderCache holds each finalized entry's last rendered lines,
	// keyed by entry ID (or "idx:N" for entries without one). buildTranscript
	// reuses a cached entry instead of re-running Block.Render (and, for
	// markdown, glamour's full parse) when its signature hasn't changed, so
	// redraws only pay for the entry that actually changed rather than
	// re-rendering the whole transcript every time.
	blockRenderCache map[string]blockRenderCacheEntry
}

type blockRenderCacheEntry struct {
	signature string
	lines     []RenderedLine
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
