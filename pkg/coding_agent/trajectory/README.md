# Trajectory

The projection turns a persisted session log into a turn-aware event ledger:
turns, approximate model steps, and per-record timing, status, and token
accounting. `/trajectory` prints it, `/trajectory html` writes a self-contained
interactive page, and the `get_trajectory` tool exposes it to the model.

## What the log does and does not record

Most of the design here follows from what a session file can actually prove.

A tool call is bracketed by two recorded events — the assistant message that
issued it and the result that came back — so its duration is measured. A model
call is written once, when it is already finished, so it has no recorded start.
Sessions written since model-call timing was added carry the request start, the
first content event, and completion, which is where real time-to-first-token,
decode time, and throughput come from. Older sessions have none of that, and
their model calls fall back to a start inferred from the previous event. Every
record says which of the two it is, so an inferred number is never read as a
measurement.

Statistics separate two pairs of numbers that are easy to conflate. Billed
input restates the whole conversation on every request, so summing it gives the
bill, not the context size. And a session resumed across weeks spans far more
wall clock than it spent working, so `durationMs` reports the span while
`activeMs` sums the turns.

Step boundaries are approximate. Nothing in the log marks them, so a new step
begins when model output resumes after one or more tool results.

## Reading the session file

Only the current root-to-leaf branch is projected; a forked session keeps its
abandoned branches in the same file, and walking the parent chain is what
excludes them. Runtime and plan snapshots are appended without moving the leaf,
so they can never sit on that branch — and they dominate a long session by
volume, so they are skipped by matching the line edge rather than decoded.

Prompt snapshots are sidecars too, but they belong in the ledger, so they are
merged back by timestamp. One taken before the session's first message belongs
to no turn and is reported in turn 0.

A subagent runs in a session of its own. An asynchronous one registers a
background task that holds its session file; a synchronous one is filed under
the tool call that requested it, since one call can fork several children.
Either way the child is an ordinary session file, so it projects through this
same code and can be opened with `/trajectory task <id>`.

## The viewer

`html.go` embeds the projection into `viewer.html`, which is self-contained: a
strict reading of "no external requests", so it works offline and can be handed
to someone else as one file.

Its behaviour lives in CSS and DOM events, which no Go test that reads the file
can reach. `viewer_browser_test.go` drives the real page in headless Chrome and
skips when no browser is installed. It exists because bugs went out that way:
an id rule outranked the `hidden` attribute so the details panel could never
close, and a missing drag threshold meant a click's pixel of drift registered
as a drag.
