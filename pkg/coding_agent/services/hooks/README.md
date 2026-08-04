# Configuration hooks

Trusted projects can run shell commands at `PreToolUse`, `PostToolUse`, and
`UserPromptSubmit`. Hook commands receive one JSON object on stdin and may emit
one JSON object on stdout.

`decision: "block"` or exit code `2` blocks a pre-tool call or submitted
prompt. `updatedInput` replaces tool arguments, `updatedPrompt` replaces a user
prompt, and `additionalContext` is appended to the model-visible result.
Unexpected hook failures are fail-open. Each hook is capped at 60 seconds and
1 MiB per output stream.
