# Project trust

Project trust stores per-directory decisions in `<agentDir>/trust.json`.
The nearest configured ancestor applies to a working directory. Persistent
`trusted` and `untrusted` decisions survive restarts; session trust exists only
for the current process.

The approval service may use trust to auto-allow `write`, `edit`, `kill_bash`,
and non-dangerous `bash` calls. Explicit deny rules and the dangerous Bash
classifier still take precedence.
