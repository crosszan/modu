package trajectory

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The viewer's behaviour lives in CSS and DOM events, which Go tests cannot
// reach by reading the file. These run the real page in headless Chrome.
//
// They skip when no Chrome is installed rather than failing, so the package
// stays buildable and testable without a browser; the string assertions in
// render_test.go are what hold the line in that case.
//
// This exists because a bug slipped through exactly here: `#inspector` set
// `display: flex`, which outranks the UA stylesheet's `[hidden]` rule, so the
// panel opened but could never be closed. Nothing that reads the file caught it.
func findChrome(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	t.Skip("no Chrome available for headless viewer tests")
	return ""
}

var probeAttribute = regexp.MustCompile(`data-probe="([^"]*)"`)

// runViewerProbe renders a trajectory, appends a script that drives the page
// and writes its findings to a body attribute, then reads that attribute back
// out of the rendered DOM.
func runViewerProbe(t *testing.T, script string) map[string]string {
	t.Helper()
	return runViewerProbeOn(t, fullSession(t), script)
}

// richSession adds what the newer write path persists: a prompt snapshot
// before the first message, a mid-conversation prompt change, and a model call
// with its real clock.
func richSession(t *testing.T) Trajectory {
	t.Helper()
	path := writeSession(t, fixtureHeader, fixturePromptInitial, fixtureUser,
		fixtureTimed, fixturePromptChanged)
	result, err := Project(path, Options{Detail: DetailFull, MaxRecords: AllRecords})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	return result
}

func runViewerProbeOn(t *testing.T, source Trajectory, script string) map[string]string {
	t.Helper()
	chrome := findChrome(t)

	page := filepath.Join(t.TempDir(), "probe.html")
	if err := WriteHTML(source, page); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	probe := "<script>setTimeout(function(){var out=[];" + script +
		"document.body.setAttribute('data-probe', out.join('|'));},250);</script>"
	if err := os.WriteFile(page, []byte(strings.Replace(string(raw), "</body>", probe+"</body>", 1)), 0o644); err != nil {
		t.Fatalf("write probe: %v", err)
	}

	cmd := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox",
		"--window-size=1400,900", "--virtual-time-budget=4000", "--dump-dom",
		// Pin the language so the assertions do not depend on the machine's locale.
		"file://"+page+"?lang=en")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("chrome: %v", err)
	}
	match := probeAttribute.FindSubmatch(output)
	if match == nil {
		t.Fatal("the page never reported: its script probably threw before finishing")
	}
	result := make(map[string]string)
	for pair := range strings.SplitSeq(string(match[1]), "|") {
		if key, value, ok := strings.Cut(pair, "="); ok {
			result[key] = value
		}
	}
	return result
}

func expect(t *testing.T, probe map[string]string, key, want string) {
	t.Helper()
	if got, ok := probe[key]; !ok {
		t.Errorf("%s was never reported (probe: %v)", key, probe)
	} else if got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}

func TestViewerInspectorOpensAndCloses(t *testing.T) {
	probe := runViewerProbe(t, `
		var ins=document.getElementById('inspector');
		var css=function(){return getComputedStyle(ins).display;};
		out.push('initial='+css());
		document.querySelector('.record').click();
		out.push('opened='+css());
		document.getElementById('inspector-close').click();
		out.push('closed='+css());
		document.querySelector('.record').click();
		document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'}));
		out.push('escaped='+css());
	`)
	expect(t, probe, "initial", "none")
	expect(t, probe, "opened", "flex")
	// The regression: Close left the panel on screen because an id rule beat
	// the `hidden` attribute.
	expect(t, probe, "closed", "none")
	expect(t, probe, "escaped", "none")
}

func TestViewerRailCountIsUnambiguous(t *testing.T) {
	// The "All turns" row sits in the same slot where every turn row shows a
	// duration, and it counts records rather than turns — a bare number there
	// reads as neither of the things it is.
	probe := runViewerProbe(t, `
		var head=document.querySelector('.turn-item .head'), meta=head.lastElementChild;
		out.push('label='+meta.textContent);
		out.push('wrapped='+(meta.getClientRects().length>1));
		out.push('overflows='+(head.scrollWidth>head.clientWidth+1));
	`)
	// fullSession projects five records across one turn.
	expect(t, probe, "label", "5 records")
	expect(t, probe, "wrapped", "false")
	expect(t, probe, "overflows", "false")
}

func TestViewerReservesRoomForTheInspector(t *testing.T) {
	// The panel is position:fixed over the timeline, so without a reservation
	// the last records can never be scrolled clear of it.
	probe := runViewerProbe(t, `
		var reserved=function(){return getComputedStyle(document.documentElement).getPropertyValue('--inspector-height').trim();};
		out.push('closed='+(reserved()||'0px'));
		document.querySelector('.record').click();
		var height=document.getElementById('inspector').offsetHeight;
		out.push('matchesPanel='+(reserved()===height+'px'));
		out.push('timelinePadding='+(parseInt(getComputedStyle(document.querySelector('section.timeline')).paddingBottom,10)>=height));
		document.getElementById('inspector-close').click();
		out.push('closedAgain='+(reserved()||'0px'));
	`)
	expect(t, probe, "closed", "0px")
	expect(t, probe, "matchesPanel", "true")
	expect(t, probe, "timelinePadding", "true")
	expect(t, probe, "closedAgain", "0px")
}

func TestViewerDividerResizesTheInspector(t *testing.T) {
	probe := runViewerProbe(t, `
		var ins=document.getElementById('inspector'), rz=document.getElementById('inspector-resize');
		var drag=function(id,y){
			rz.dispatchEvent(new PointerEvent('pointerdown',{pointerId:id,clientY:ins.getBoundingClientRect().top,bubbles:true}));
			rz.dispatchEvent(new PointerEvent('pointermove',{pointerId:id,clientY:y,bubbles:true}));
			var held=document.body.classList.contains('resizing');
			rz.dispatchEvent(new PointerEvent('pointerup',{pointerId:id,clientY:y,bubbles:true}));
			return held;
		};
		document.querySelector('.record').click();
		var held=drag(1,200);
		out.push('heldWhileDragging='+held);
		out.push('released='+!document.body.classList.contains('resizing'));
		out.push('height='+ins.offsetHeight+'/'+(window.innerHeight-200));
		out.push('reservedAfterDrag='+(getComputedStyle(document.documentElement).getPropertyValue('--inspector-height').trim()===ins.offsetHeight+'px'));
		drag(2,-800);
		out.push('clampedTop='+(ins.offsetHeight<=window.innerHeight-120));
		drag(3,window.innerHeight+800);
		out.push('clampedBottom='+(ins.offsetHeight>=92));
	`)
	expect(t, probe, "heldWhileDragging", "true")
	expect(t, probe, "released", "true")
	expect(t, probe, "reservedAfterDrag", "true")
	// Dragging to y keeps the panel's top edge at y.
	if got := probe["height"]; got != "" {
		if parts := strings.Split(got, "/"); len(parts) == 2 && parts[0] != parts[1] {
			t.Errorf("panel height = %s, want %s", parts[0], parts[1])
		}
	}
	// Overshooting must not swallow the toolbar or collapse the panel.
	expect(t, probe, "clampedTop", "true")
	expect(t, probe, "clampedBottom", "true")
}

// dragTrack is the shared gesture helper the timeline probes reuse.
const dragTrack = `
	var track=document.getElementById('track');
	var box=track.getBoundingClientRect();
	var rows=function(){return document.querySelectorAll('.record').length;};
	var drag=function(button,fromX,toX){
		track.dispatchEvent(new PointerEvent('pointerdown',{pointerId:9,button:button,clientX:box.left+fromX,clientY:box.top+20,bubbles:true}));
		track.dispatchEvent(new PointerEvent('pointermove',{pointerId:9,button:button,clientX:box.left+toX,clientY:box.top+20,bubbles:true}));
		track.dispatchEvent(new PointerEvent('pointerup',{pointerId:9,button:button,clientX:box.left+toX,clientY:box.top+20,bubbles:true}));
	};
`

func TestViewerTimelineProjectsEveryRecord(t *testing.T) {
	probe := runViewerProbe(t, dragTrack+`
		out.push('spans='+track.querySelectorAll('.span').length);
		out.push('turnMarks='+track.querySelectorAll('.turn-mark').length);
		var axis=function(){return document.getElementById('axis-end').textContent;};
		var mode=function(m){document.querySelector('.mode[data-mode="'+m+'"]').click();};
		out.push('compressedAxis='+axis());
		mode('sequence'); out.push('sequenceAxis='+axis());
		mode('actual');   out.push('actualAxis='+axis());
	`)
	// Every record reaches the track, and each turn gets a boundary mark.
	expect(t, probe, "spans", "5")
	expect(t, probe, "turnMarks", "1")
	// Sequence numbers records; the timed projections read as clock or elapsed.
	expect(t, probe, "sequenceAxis", "#6")
	if probe["compressedAxis"] == probe["sequenceAxis"] {
		t.Errorf("compressed projection did not change the domain: %v", probe)
	}
	if probe["actualAxis"] == "" {
		t.Error("actual projection produced no axis label")
	}
}

func TestViewerTimelineSelectionFocusesTheLedger(t *testing.T) {
	probe := runViewerProbe(t, dragTrack+`
		var all=rows();
		out.push('all='+all);
		drag(0, 2, box.width*0.2);
		out.push('selectionShown='+!document.getElementById('selection').hidden);
		out.push('narrowed='+(rows()<all && rows()>0));
		drag(2, 50, 50);
		out.push('cleared='+(rows()===all));
	`)
	expect(t, probe, "all", "5")
	expect(t, probe, "selectionShown", "true")
	expect(t, probe, "narrowed", "true")
	// A right-click with no movement restores the full ledger.
	expect(t, probe, "cleared", "true")
}

func TestViewerTimelineZoomAndPan(t *testing.T) {
	probe := runViewerProbe(t, dragTrack+`
		var axis=function(){return document.getElementById('axis-start').textContent;};
		var full=axis(), all=rows();
		track.dispatchEvent(new WheelEvent('wheel',{deltaY:-200,clientX:box.left+box.width/2,clientY:box.top+20,bubbles:true,cancelable:true}));
		var zoomed=axis();
		out.push('zoomed='+(zoomed!==full));
		drag(2, box.width*0.6, box.width*0.3);
		out.push('panned='+(axis()!==zoomed));
		out.push('panKeptLedger='+(rows()===all));
		document.getElementById('reset-view').click();
		out.push('reset='+(axis()===full));
	`)
	expect(t, probe, "zoomed", "true")
	expect(t, probe, "panned", "true")
	// Panning must not be mistaken for a selection.
	expect(t, probe, "panKeptLedger", "true")
	expect(t, probe, "reset", "true")
}

func TestViewerShowsSystemPromptWhenSupplied(t *testing.T) {
	source := fullSession(t)
	source.Session.Prompt = &Prompt{
		System: "You are a coding agent for modu.",
		Bytes:  32,
		Tools:  []Tool{{Name: "bash", Description: "Run a command", Schema: `{"type":"object"}`}},
	}
	probe := runViewerProbeOn(t, source, `
		var button=document.getElementById('system-button');
		out.push('offered='+!button.hidden);
		button.click();
		var tab=function(id){
			var tabs=document.querySelectorAll('#inspector-body .tab');
			for(var i=0;i<tabs.length;i++){ if(tabs[i].dataset.tab===id){tabs[i].click();return true;} }
			return false;
		};
		out.push('opened='+(getComputedStyle(document.getElementById('inspector')).display==='flex'));
		tab('prompt');
		out.push('showsPrompt='+(document.getElementById('inspector-body').textContent.indexOf('coding agent for modu')>=0));
		tab('catalog');
		var catalog=document.getElementById('inspector-body').textContent;
		out.push('showsTool='+(catalog.indexOf('Run a command')>=0));
		out.push('showsSchema='+(catalog.indexOf('"type":"object"')>=0));
	`)
	expect(t, probe, "offered", "true")
	expect(t, probe, "showsPrompt", "true")
	expect(t, probe, "showsTool", "true")
	expect(t, probe, "showsSchema", "true")
	expect(t, probe, "opened", "true")
}

func TestViewerHidesSystemPromptWhenAbsent(t *testing.T) {
	// A trajectory projected from a session file alone carries no prompt, and
	// the page must not offer a control that would open an empty panel.
	probe := runViewerProbe(t, `
		out.push('hidden='+document.getElementById('system-button').hidden);
	`)
	expect(t, probe, "hidden", "true")
}

func TestViewerLabelsDerivedTiming(t *testing.T) {
	// A model call's start is inferred, not recorded. The panel must say so
	// rather than presenting it as a measurement.
	probe := runViewerProbe(t, `
		// The derived span is charged to the first record the model call produced,
		// so one call is one span rather than one per content block.
		var timingTab=function(){
			var tabs=document.querySelectorAll('#inspector-body .tab');
			for(var i=0;i<tabs.length;i++){ if(tabs[i].dataset.tab==='timing'){tabs[i].click();return;} }
		};
		document.querySelector('.record.k-reasoning').click();
		timingTab();
		out.push('body='+(document.getElementById('inspector-body').textContent.indexOf('inferred from the previous event')>=0));
		document.getElementById('inspector-close').click();
		document.querySelectorAll('.record.k-assistant')[0].click();
		timingTab();
		out.push('instant='+(document.getElementById('inspector-body').textContent.indexOf('inferred')<0));
	`)
	expect(t, probe, "body", "true")
	expect(t, probe, "instant", "true")
}

func TestViewerFoldsTurnsAndSteps(t *testing.T) {
	probe := runViewerProbe(t, `
		var rows=function(){return document.querySelectorAll('.record').length;};
		var all=rows();
		out.push('all='+all);
		document.getElementById('fold-turns').click();
		out.push('turnsCollapsed='+(rows()===0));
		document.getElementById('fold-turns').click();
		out.push('turnsRestored='+(rows()===all));
		document.getElementById('fold-steps').click();
		out.push('stepsCollapsed='+(rows()<all&&rows()>0));
		document.getElementById('fold-steps').click();
		document.querySelector('.turn-head').click();
		out.push('singleTurnCollapsed='+(rows()<all));
	`)
	expect(t, probe, "turnsCollapsed", "true")
	expect(t, probe, "turnsRestored", "true")
	// Collapsing calls hides step contents but keeps the turn's own rows.
	expect(t, probe, "stepsCollapsed", "true")
	expect(t, probe, "singleTurnCollapsed", "true")
}

func TestViewerSearchesRecordPayloads(t *testing.T) {
	// The summary is one line; what you are looking for is usually in the tool
	// input or output, so the index must cover them.
	source := fullSession(t)
	full, err := Project(writeSession(t, fixtureHeader, fixtureUser, fixtureAssist, fixtureResult, fixtureFinal),
		Options{Detail: DetailFull})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	source = full
	probe := runViewerProbeOn(t, source, `
		var rows=function(){return document.querySelectorAll('.record').length;};
		var box=document.getElementById('search');
		var all=rows();
		box.value='build ok'; box.dispatchEvent(new Event('input',{bubbles:true}));
		out.push('payloadHits='+rows());
		out.push('narrowed='+(rows()>0&&rows()<all));
		box.value='nothing-matches-this'; box.dispatchEvent(new Event('input',{bubbles:true}));
		out.push('missShows='+(document.querySelector('.empty')!==null));
	`)
	// "build ok" only ever appears in the tool result body.
	expect(t, probe, "narrowed", "true")
	expect(t, probe, "missShows", "true")
}

func TestViewerTabsDetailsByRecordKind(t *testing.T) {
	probe := runViewerProbeOn(t, richSession(t), `
		var tabsOf=function(){
			var names=[];
			Array.prototype.forEach.call(document.querySelectorAll('#inspector-body .tab'),
				function(b){names.push(b.dataset.tab);});
			return names.join(',');
		};
		document.querySelector('.record.k-assistant').click();
		out.push('assistant='+tabsOf());
		document.getElementById('inspector-close').click();
		var systems=document.querySelectorAll('.record.k-system');
		systems[systems.length-1].click();
		out.push('system='+tabsOf());
	`)
	// A model reply offers its payloads, timing and usage.
	if got := probe["assistant"]; got == "" || !strings.Contains(got, "timing") || !strings.Contains(got, "usage") {
		t.Errorf("assistant tabs = %q", got)
	}
	// A prompt change offers the prompt, the catalog, and a diff.
	for _, want := range []string{"prompt", "catalog", "diff"} {
		if !strings.Contains(probe["system"], want) {
			t.Errorf("system tabs = %q, want a %q tab", probe["system"], want)
		}
	}
}

func TestViewerDiffsPromptChanges(t *testing.T) {
	probe := runViewerProbeOn(t, richSession(t), `
		var systems=document.querySelectorAll('.record.k-system');
		systems[systems.length-1].click();
		var tabs=document.querySelectorAll('#inspector-body .tab');
		for(var i=0;i<tabs.length;i++){ if(tabs[i].dataset.tab==='diff'){tabs[i].click();break;} }
		var body=document.getElementById('inspector-body');
		out.push('removed='+body.querySelectorAll('.diff .del').length);
		out.push('added='+body.querySelectorAll('.diff .add').length);
		document.getElementById('inspector-close').click();
		systems[0].click();
		var first=document.querySelectorAll('#inspector-body .tab');
		var names=[]; Array.prototype.forEach.call(first,function(b){names.push(b.dataset.tab);});
		out.push('initialHasNoDiff='+(names.indexOf('diff')<0));
	`)
	// The prompt changed from "You are modu." to "You are modu, in plan mode."
	expect(t, probe, "removed", "1")
	expect(t, probe, "added", "1")
	// The first snapshot has nothing to compare against.
	expect(t, probe, "initialHasNoDiff", "true")
}

func TestViewerShowsBetweenTurnsSection(t *testing.T) {
	// The snapshot taken before the session's first message belongs to no turn.
	probe := runViewerProbeOn(t, richSession(t), `
		var between=document.querySelector('.turn-head.between');
		out.push('present='+(between!==null));
		out.push('labelled='+(between!==null&&between.textContent.length>0));
	`)
	expect(t, probe, "present", "true")
	expect(t, probe, "labelled", "true")
}

func TestViewerReportsRecordedModelTiming(t *testing.T) {
	probe := runViewerProbeOn(t, richSession(t), `
		document.querySelector('.record.k-assistant').click();
		var tabs=document.querySelectorAll('#inspector-body .tab');
		for(var i=0;i<tabs.length;i++){ if(tabs[i].dataset.tab==='timing'){tabs[i].click();break;} }
		var body=document.getElementById('inspector-body').textContent;
		out.push('hasThroughput='+(body.indexOf('tok/s')>=0));
		out.push('notDerived='+(body.indexOf('inferred')<0&&body.indexOf('推导')<0));
	`)
	// A measured call reports decode throughput and must not be labelled inferred.
	expect(t, probe, "hasThroughput", "true")
	expect(t, probe, "notDerived", "true")
}

func TestViewerSwitchesLanguage(t *testing.T) {
	probe := runViewerProbe(t, `
		var label=function(){return document.getElementById('reset-view').textContent;};
		var before=label();
		document.getElementById('lang-button').click();
		var after=label();
		out.push('changed='+(after!==before));
		out.push('bothKnown='+((before==='Reset'&&after==='重置')||(before==='重置'&&after==='Reset')));
		document.getElementById('lang-button').click();
		out.push('restored='+(label()===before));
	`)
	expect(t, probe, "changed", "true")
	expect(t, probe, "bothKnown", "true")
	expect(t, probe, "restored", "true")
}

func TestViewerReportsSubagentRuns(t *testing.T) {
	source := fullSession(t)
	source.Records = append(source.Records, Record{
		Index: 90, Turn: 1, Step: 2, Kind: KindSubagent, Event: "tool_call",
		ToolName: "subagent", Summary: "subagent explore", Status: StatusComplete,
		Subagent: &SubagentRun{
			RunID: "task-7", Agent: "explorer", Available: true,
			Turns: 3, Steps: 8, ToolCalls: 11, Failures: 1, ActiveMs: 42000,
			Tokens: Usage{Input: 9000, Output: 400},
			Tools:  []ToolStat{{Name: "read", Calls: 9, TotalMs: 300}},
		},
	}, Record{
		Index: 91, Turn: 1, Step: 2, Kind: KindSubagent, Event: "tool_call",
		ToolName: "subagent", Summary: "subagent inline", Status: StatusComplete,
		Subagent: &SubagentRun{RunID: "task-8", Reason: "this run recorded no session file"},
	})
	probe := runViewerProbeOn(t, source, `
		var open=function(text){
			var rows=document.querySelectorAll('.record.k-subagent');
			for (var i=0;i<rows.length;i++){
				if (rows[i].textContent.indexOf(text)>=0){ rows[i].click(); return; }
			}
		};
		var tab=function(id){
			var tabs=document.querySelectorAll('#inspector-body .tab');
			for(var i=0;i<tabs.length;i++){ if(tabs[i].dataset.tab===id){tabs[i].click();return true;} }
			return false;
		};
		open('explore');
		out.push('offersTab='+tab('subagent'));
		var body=document.getElementById('inspector-body').textContent;
		out.push('childTurns='+(body.indexOf('3')>=0));
		out.push('childTokens='+(body.indexOf('9,000')>=0));
		out.push('childTools='+(body.indexOf('read')>=0));
		document.getElementById('inspector-close').click();
		open('inline');
		tab('subagent');
		var second=document.getElementById('inspector-body').textContent;
		out.push('explainsAbsence='+(second.indexOf('no session file')>=0));
		out.push('noFakeZeros='+(second.indexOf('9,000')<0));
	`)
	expect(t, probe, "offersTab", "true")
	expect(t, probe, "childTurns", "true")
	expect(t, probe, "childTokens", "true")
	expect(t, probe, "childTools", "true")
	// An unresolved run must say why instead of rendering a run of zeros.
	expect(t, probe, "explainsAbsence", "true")
	expect(t, probe, "noFakeZeros", "true")
}
