# Rewind recorder

The recorder snapshots files changed by the built-in `write` and `edit` tools
and groups those snapshots by agent turn. `/rewind N` restores the selected
turn and all later tracked changes, then moves the conversation leaf to its
pre-turn position.

Bash, MCP, network, and other external side effects are intentionally outside
the rollback boundary. Files larger than 16 MiB are not retained. Rewind also
refuses to overwrite a file whose current state no longer matches the latest
tracked tool mutation.
