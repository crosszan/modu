// Package dictionary embeds the dictionaries shipped with modu_code.
package dictionary

import (
	_ "embed"
	"strings"
)

var (
	//go:embed en_US.aff
	englishAff string
	//go:embed en_US.dic
	englishDic string
)

// English returns fresh readers for the bundled en_US Hunspell dictionary.
func English() (*strings.Reader, *strings.Reader) {
	return strings.NewReader(englishAff), strings.NewReader(englishDic)
}
