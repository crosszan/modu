package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	builtinSkillCreatorCommit = "2a40fd2e7c52207aa903bd33fc4c65716126966e"
	embeddedSkillCreatorRoot  = "builtin/skill-creator"
)

// builtinSkillCreatorFS contains the unmodified Anthropic skill-creator
// directory. It is materialized because the skill's Python scripts and HTML
// assets must be addressable by normal filesystem tools at runtime.
//
//go:embed all:builtin/skill-creator
var builtinSkillCreatorFS embed.FS

var builtinSkillCreatorExecutables = map[string]struct{}{
	"scripts/aggregate_benchmark.py": {},
	"scripts/generate_report.py":     {},
	"scripts/improve_description.py": {},
	"scripts/package_skill.py":       {},
	"scripts/quick_validate.py":      {},
	"scripts/run_eval.py":            {},
	"scripts/run_loop.py":            {},
}

func materializeBuiltinSkillCreator(agentDir string) (string, error) {
	if strings.TrimSpace(agentDir) == "" {
		return "", fmt.Errorf("agent directory is required")
	}

	parent := filepath.Join(agentDir, "builtin-skills")
	versionRoot := filepath.Join(parent, builtinSkillCreatorCommit)
	skillRoot := filepath.Join(versionRoot, "skill-creator")
	if builtinSkillMaterialized(versionRoot) {
		return skillRoot, nil
	}

	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create built-in skills directory: %w", err)
	}
	stagingRoot, err := os.MkdirTemp(parent, ".skill-creator-*")
	if err != nil {
		return "", fmt.Errorf("create skill-creator staging directory: %w", err)
	}
	defer os.RemoveAll(stagingRoot)

	stagingSkillRoot := filepath.Join(stagingRoot, "skill-creator")
	err = fs.WalkDir(builtinSkillCreatorFS, embeddedSkillCreatorRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, ok := strings.CutPrefix(path, embeddedSkillCreatorRoot)
		if !ok {
			return fmt.Errorf("embedded skill path %q is outside %q", path, embeddedSkillCreatorRoot)
		}
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(stagingSkillRoot, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := builtinSkillCreatorFS.ReadFile(path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if _, ok := builtinSkillCreatorExecutables[filepath.ToSlash(rel)]; ok {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
	if err != nil {
		return "", fmt.Errorf("materialize skill-creator: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stagingRoot, ".complete"), []byte(builtinSkillCreatorCommit+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("mark skill-creator materialization complete: %w", err)
	}
	if err := os.Rename(stagingRoot, versionRoot); err != nil {
		if builtinSkillMaterialized(versionRoot) {
			return skillRoot, nil
		}
		return "", fmt.Errorf("publish skill-creator materialization: %w", err)
	}
	return skillRoot, nil
}

func builtinSkillMaterialized(versionRoot string) bool {
	data, err := os.ReadFile(filepath.Join(versionRoot, ".complete"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == builtinSkillCreatorCommit
}
