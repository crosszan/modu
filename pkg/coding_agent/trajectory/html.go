package trajectory

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed viewer.html
var viewerTemplate string

// jsonPlaceholder is where the projected trajectory is injected. json.Marshal
// escapes "<", ">" and "&" to their \u form by default, so no session content
// can close the surrounding script element.
const jsonPlaceholder = "__TRAJECTORY_JSON__"

// WriteHTML renders a trajectory as a single self-contained page: no scripts,
// styles, or fonts are fetched, so the file works offline and can be handed to
// someone else as-is.
func WriteHTML(t Trajectory, path string) error {
	payload, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("encode trajectory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	page := strings.Replace(viewerTemplate, jsonPlaceholder, string(payload), 1)
	return os.WriteFile(path, []byte(page), 0o644)
}
