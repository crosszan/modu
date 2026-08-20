// Package spellcheck checks ordinary English prose with a Hunspell dictionary.
package spellcheck

import (
	"io"
	"strings"
	"sync"
	"unicode"

	"github.com/client9/gospell"
	dictionary "github.com/openmodu/modu/dict"
)

// Issue identifies one misspelled word by half-open rune offsets in the input.
type Issue struct {
	Start int
	End   int
	Word  string
}

// Checker serializes access to gospell so Bubble Tea commands may safely run
// checks and suggestion lookups concurrently.
type Checker struct {
	mu    sync.Mutex
	spell *gospell.GoSpell
}

func New(aff, dic io.Reader) (*Checker, error) {
	spell, err := gospell.NewGoSpellReader(aff, dic)
	if err != nil {
		return nil, err
	}
	return &Checker{spell: spell}, nil
}

// NewEnglish loads modu_code's bundled en_US dictionary.
func NewEnglish() (*Checker, error) {
	aff, dic := dictionary.English()
	return New(aff, dic)
}

// Check returns misspellings in ordinary prose. Shell-style commands, file
// mentions, URLs, paths, identifiers, version-like tokens, and inline
// backtick code are deliberately ignored.
func (c *Checker) Check(text string) []Issue {
	if c == nil || c.spell == nil {
		return nil
	}
	runes := []rune(text)
	type span struct{ start, end int }
	var spans []span
	inCode := false
	start := -1
	flush := func(end int) {
		if start >= 0 && !inCode {
			spans = append(spans, span{start: start, end: end})
		}
		start = -1
	}
	for i, r := range runes {
		if r == '`' {
			flush(i)
			inCode = !inCode
			continue
		}
		if inCode || unicode.IsSpace(r) || isInputPlaceholder(r) {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(len(runes))

	issues := make([]Issue, 0)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, token := range spans {
		raw := string(runes[token.start:token.end])
		if shouldSkipRaw(raw) {
			continue
		}
		start, end := trimNonLetters(runes, token.start, token.end)
		if start >= end {
			continue
		}
		word := string(runes[start:end])
		if shouldSkip(word) || c.spell.Spell(word) {
			continue
		}
		issues = append(issues, Issue{Start: start, End: end, Word: word})
	}
	return issues
}

func (c *Checker) Suggest(word string, limit int) ([]string, error) {
	if c == nil || c.spell == nil || limit <= 0 {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	suggestions, err := c.spell.Suggest(word, limit)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.Word != "" {
			out = append(out, suggestion.Word)
		}
	}
	return out, nil
}

func trimNonLetters(runes []rune, start, end int) (int, int) {
	for start < end && !unicode.IsLetter(runes[start]) {
		start++
	}
	for end > start && !unicode.IsLetter(runes[end-1]) {
		end--
	}
	return start, end
}

func shouldSkip(word string) bool {
	runes := []rune(word)
	if len(runes) < 2 {
		return true
	}
	upperAfterFirst := false
	for i, r := range runes {
		if r > unicode.MaxASCII {
			return true
		}
		if unicode.IsDigit(r) {
			return true
		}
		if !unicode.IsLetter(r) && r != '\'' && r != '’' && r != '-' {
			return true
		}
		if i > 0 && unicode.IsUpper(r) {
			upperAfterFirst = true
		}
	}
	return upperAfterFirst
}

func shouldSkipRaw(token string) bool {
	trimmed := strings.TrimLeftFunc(token, unicode.IsPunct)
	return strings.HasPrefix(token, "/") || strings.HasPrefix(token, "@") ||
		strings.HasPrefix(trimmed, "www.") || strings.Contains(token, "://") ||
		strings.ContainsAny(token, "/\\_") || strings.Contains(strings.Trim(token, ",;:!?()[]{}\"'"), ".")
}

func isInputPlaceholder(r rune) bool {
	return r >= 0xE000 && r < 0x10000
}
