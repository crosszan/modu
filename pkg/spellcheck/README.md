# Spell checking

`pkg/spellcheck` wraps `github.com/client9/gospell` for English prose checks.
`NewEnglish` loads Modu's embedded `dict/en_US.aff` and `dict/en_US.dic` files;
`New` accepts alternate Hunspell-compatible readers for reuse and tests.

`Check` returns half-open rune ranges, not byte offsets. It intentionally skips
slash commands, `@` file mentions, URLs, paths, identifiers, version-like
tokens, inline backtick code, and non-ASCII text. `Suggest` returns ranked
replacement words. A checker serializes both operations because callers may
run Bubble Tea commands concurrently.
