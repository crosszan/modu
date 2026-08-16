package trajectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatDuration(t *testing.T) {
	cases := map[int64]string{
		0:          "—",
		-1:         "—",
		250:        "250ms",
		1500:       "1.5s",
		61_000:     "1m01s",
		3_725_000:  "1h02m",
		97_200_000: "1d03h",
	}
	for input, want := range cases {
		if got := formatDuration(input); got != want {
			t.Errorf("formatDuration(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatCount(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 1000: "1,000", 5_074_516: "5,074,516", -1234: "-1,234"}
	for input, want := range cases {
		if got := formatCount(input); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestOverviewReportsActiveAndSpanSeparately(t *testing.T) {
	result := fullSession(t)
	text := strings.Join(Overview(result), "\n")
	for _, want := range []string{"session: sess-1", "deepseek/deepseek-v4", "turns: 1", "1 calls", "active:", "span"} {
		if !strings.Contains(text, want) {
			t.Errorf("overview missing %q:\n%s", want, text)
		}
	}
}

func TestTurnLinesAnnouncesTruncation(t *testing.T) {
	total := maxRenderedTurns + 3
	lines := make([]string, 0, total+1)
	lines = append(lines, fixtureHeader)
	parent := "null"
	for i := range total {
		id := fmt.Sprintf("u%d", i)
		lines = append(lines, fmt.Sprintf(
			`{"id":%q,"parentId":%s,"timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"turn"},"type":"message"}`,
			id, parent))
		parent = fmt.Sprintf("%q", id)
	}
	path := writeSession(t, lines...)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	rendered := TurnLines(result)
	if len(rendered) != maxRenderedTurns+1 {
		t.Fatalf("rendered %d lines, want %d", len(rendered), maxRenderedTurns+1)
	}
	// A cap that hides turns must say so rather than showing a silent prefix.
	if !strings.Contains(rendered[0], "of 23 turns") {
		t.Errorf("truncation not announced: %q", rendered[0])
	}
}

func TestTurnDetailRendersStepsAndPayloads(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, fixtureFinal)
	result, err := Project(path, Options{Detail: DetailFull})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	text := strings.Join(TurnDetail(result, 1), "\n")
	for _, want := range []string{"turn 1", "prompt: fix the build", "step 1", "step 2", "bash", "output: build ok"} {
		if !strings.Contains(text, want) {
			t.Errorf("turn detail missing %q:\n%s", want, text)
		}
	}
	if TurnDetail(result, 7) != nil {
		t.Error("TurnDetail should return nil for a turn that does not exist")
	}
}

func TestTurnDetailOmitsPayloadsAtSummaryDetail(t *testing.T) {
	result := fullSession(t)
	text := strings.Join(TurnDetail(result, 1), "\n")
	if strings.Contains(text, "output:") || strings.Contains(text, "input:") {
		t.Errorf("summary detail leaked payloads:\n%s", text)
	}
}

func TestToolLinesReportFailuresAndUnfinished(t *testing.T) {
	path := writeSession(t, fixtureHeader, fixtureUser, fixtureAssist)
	result, err := Project(path, Options{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	text := strings.Join(ToolLines(result), "\n")
	if !strings.Contains(text, "bash") || !strings.Contains(text, "1 unfinished") {
		t.Errorf("tool lines = %q", text)
	}
}

func TestWriteHTMLEmbedsTrajectoryAndStandsAlone(t *testing.T) {
	result := fullSession(t)
	path := filepath.Join(t.TempDir(), "nested", "trajectory.html")
	if err := WriteHTML(result, path); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	page := string(raw)
	if strings.Contains(page, jsonPlaceholder) {
		t.Error("placeholder was not replaced")
	}
	// A self-contained page: nothing is fetched, so it works offline.
	for _, forbidden := range []string{"src=\"http", "href=\"http", "@import"} {
		if strings.Contains(page, forbidden) {
			t.Errorf("page references an external resource: %q", forbidden)
		}
	}

	start := strings.Index(page, `id="trajectory-data">`)
	if start < 0 {
		t.Fatal("no embedded data element")
	}
	start += len(`id="trajectory-data">`)
	end := strings.Index(page[start:], "</script>")
	if end < 0 {
		t.Fatal("embedded data element is unterminated")
	}
	var decoded Trajectory
	if err := json.Unmarshal([]byte(page[start:start+end]), &decoded); err != nil {
		t.Fatalf("embedded JSON does not parse: %v", err)
	}
	if decoded.Session.ID != "sess-1" || len(decoded.Records) != len(result.Records) {
		t.Errorf("embedded trajectory = %+v", decoded.Session)
	}
}

func TestViewerEnforcesTheHiddenAttribute(t *testing.T) {
	// The inspector panel is opened and closed by toggling `hidden`, but the
	// UA stylesheet's `[hidden] { display: none }` loses to any author rule
	// that sets `display` on the same element — `#inspector { display: flex }`
	// once left the panel impossible to close. The page must restate the rule
	// at a specificity nothing here can outrank.
	if !strings.Contains(viewerTemplate, "[hidden] { display: none !important; }") {
		t.Error("viewer.html must force the hidden attribute to win over author display rules")
	}
	for _, id := range []string{"inspector", "warnings"} {
		if !strings.Contains(viewerTemplate, `id="`+id+`" hidden`) &&
			!strings.Contains(viewerTemplate, `id="`+id+`"  hidden`) {
			t.Errorf("#%s should start hidden", id)
		}
	}
}

func TestWriteHTMLEscapesScriptClosingTag(t *testing.T) {
	// Session text can contain anything, including a closing script tag. If it
	// were embedded raw it would end the data element and break the page.
	hostile := `{"id":"u1","parentId":null,"timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"</script><script>alert(1)</script>"},"type":"message"}`
	path := writeSession(t, fixtureHeader, hostile)
	result, err := Project(path, Options{Detail: DetailFull})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	out := filepath.Join(t.TempDir(), "trajectory.html")
	if err := WriteHTML(result, out); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "<script>alert(1)</script>") {
		t.Error("session text was embedded without escaping")
	}
	if !strings.Contains(string(raw), `\u003c/script\u003e`) {
		t.Error("expected the closing tag to be unicode-escaped in the embedded JSON")
	}
}
