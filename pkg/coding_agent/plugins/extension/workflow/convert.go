package workflow

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

var workflowMetaNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type agentOptions struct {
	Label           string
	Phase           string
	Model           string
	Cwd             string
	Isolation       string
	Tools           []string
	DisallowedTools []string
	PermissionMode  string
	MaxTurns        int
	Thinking        string
	Skills          []string
	MemoryScope     string
	Schema          map[string]any
}

// objectField returns the map for a JS object argument exported via goja's
// Value.Export(). Nil/undefined exports as nil; anything that is not an object
// is rejected so the caller can surface a clear error.
func objectField(raw any) (map[string]any, bool) {
	m, ok := raw.(map[string]any)
	return m, ok
}

// firstKey reads the first present key (camelCase preferred, snake_case
// accepted) so workflow authors can use either JS-idiomatic or snake_case
// option names without surprises.
func firstKey(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func decodeMeta(raw any) (metaInfo, error) {
	m, ok := objectField(raw)
	if !ok {
		return metaInfo{}, fmt.Errorf("meta() requires an object argument")
	}
	rawDescription := stringField(m, "description")
	meta := metaInfo{
		Name:        strings.TrimSpace(stringField(m, "name")),
		Description: strings.TrimSpace(rawDescription),
		WhenToUse:   strings.TrimSpace(stringFieldAny(m, "whenToUse", "when_to_use")),
	}
	if !workflowMetaNamePattern.MatchString(meta.Name) {
		return meta, fmt.Errorf("meta.name must be a kebab-case string")
	}
	if meta.Description == "" {
		return meta, fmt.Errorf("meta.description must be a non-empty string")
	}
	if !workflowMetaTextIsOneLine(rawDescription) {
		return meta, fmt.Errorf("meta.description must be a one-line string without control characters")
	}
	if rawPhases := firstKey(m, "phases"); rawPhases != nil {
		list, ok := rawPhases.([]any)
		if !ok {
			return meta, fmt.Errorf("meta.phases must be an array")
		}
		decoded, err := decodePhases(list)
		if err != nil {
			return meta, err
		}
		meta.Phases = decoded
	}
	if rawExampleArgs := firstKey(m, "exampleArgs", "example_args"); rawExampleArgs != nil {
		exampleArgs, err := decodeExampleArgs(rawExampleArgs)
		if err != nil {
			return meta, err
		}
		meta.ExampleArgs = &exampleArgs
	}
	return meta, nil
}

func decodePhases(list []any) ([]phaseInfo, error) {
	var out []phaseInfo
	seen := map[string]struct{}{}
	for index, item := range list {
		obj, ok := objectField(item)
		if !ok {
			return nil, fmt.Errorf("meta.phases[%d] must be an object", index)
		}
		rawTitle := stringField(obj, "title")
		p := phaseInfo{
			Title:  strings.TrimSpace(rawTitle),
			Detail: strings.TrimSpace(stringField(obj, "detail")),
			Model:  strings.TrimSpace(stringField(obj, "model")),
		}
		if p.Title == "" {
			return nil, fmt.Errorf("meta.phases[%d].title must be a non-empty string", index)
		}
		if !workflowMetaTextIsOneLine(rawTitle) {
			return nil, fmt.Errorf("meta.phases[%d].title must be a one-line string without control characters", index)
		}
		if _, exists := seen[p.Title]; exists {
			return nil, fmt.Errorf("meta.phases titles must be unique")
		}
		seen[p.Title] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

func decodeExampleArgs(raw any) (map[string]any, error) {
	exampleArgs, ok := objectField(raw)
	if !ok {
		return nil, fmt.Errorf("meta.exampleArgs must be a JSON-safe object")
	}
	data, err := json.Marshal(exampleArgs)
	if err != nil {
		return nil, fmt.Errorf("meta.exampleArgs must be a JSON-safe object: %w", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("meta.exampleArgs must be a JSON-safe object: %w", err)
	}
	return cloned, nil
}

func workflowMetaTextIsOneLine(value string) bool {
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func decodeAgentOptions(raw any) (agentOptions, error) {
	if raw == nil {
		return agentOptions{}, nil
	}
	m, ok := objectField(raw)
	if !ok {
		return agentOptions{}, fmt.Errorf("agent options must be an object")
	}
	maxTurns, err := positiveIntField(m, "maxTurns", "max_turns")
	if err != nil {
		return agentOptions{}, err
	}
	opts := agentOptions{
		Label:          strings.TrimSpace(stringField(m, "label")),
		Phase:          strings.TrimSpace(stringField(m, "phase")),
		Model:          strings.TrimSpace(stringField(m, "model")),
		Cwd:            strings.TrimSpace(stringField(m, "cwd")),
		Isolation:      strings.TrimSpace(stringField(m, "isolation")),
		PermissionMode: strings.TrimSpace(stringFieldAny(m, "permissionMode", "permission_mode")),
		MaxTurns:       maxTurns,
		Thinking:       strings.TrimSpace(stringField(m, "thinking")),
		MemoryScope:    strings.TrimSpace(stringFieldAny(m, "memoryScope", "memory_scope")),
	}
	if opts.Isolation != "" && opts.Isolation != "worktree" {
		return opts, fmt.Errorf("isolation must be \"worktree\" when set")
	}
	if !validMemoryScope(opts.MemoryScope) {
		return opts, fmt.Errorf("memoryScope must be one of none, user, global, project, local, both, or all")
	}
	if opts.Tools, err = stringListField(m, "tools"); err != nil {
		return opts, err
	}
	if opts.DisallowedTools, err = stringListFieldAny(m, "disallowedTools", "disallowed_tools"); err != nil {
		return opts, err
	}
	if opts.Skills, err = stringListField(m, "skills"); err != nil {
		return opts, err
	}
	if opts.Schema, err = schemaField(m, "schema"); err != nil {
		return opts, err
	}
	return opts, nil
}

func validMemoryScope(scope string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "none", "user", "global", "project", "local", "both", "all":
		return true
	default:
		return false
	}
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func stringFieldAny(m map[string]any, keys ...string) string {
	if s, ok := firstKey(m, keys...).(string); ok {
		return s
	}
	return ""
}

func positiveIntField(m map[string]any, keys ...string) (int, error) {
	raw := firstKey(m, keys...)
	if raw == nil {
		return 0, nil
	}
	f, ok := numberValue(raw)
	if !ok || f != math.Trunc(f) || f <= 0 || f > float64(math.MaxInt) {
		return 0, fmt.Errorf("%s must be a positive integer", keys[0])
	}
	return int(f), nil
}

// numberValue normalizes the numeric kinds goja's Export produces (int64 for
// integral JS numbers, float64 otherwise).
func numberValue(raw any) (float64, bool) {
	switch v := raw.(type) {
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func stringListField(m map[string]any, key string) ([]string, error) {
	return stringListFieldAny(m, key)
}

func stringListFieldAny(m map[string]any, keys ...string) ([]string, error) {
	raw := firstKey(m, keys...)
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", keys[0])
	}
	out := make([]string, 0, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", keys[0], i)
		}
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, nil
}

func schemaField(m map[string]any, key string) (map[string]any, error) {
	raw := firstKey(m, key)
	if raw == nil {
		return nil, nil
	}
	schema, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", key)
	}
	if err := validateSchemaDefinition(schema, "schema"); err != nil {
		return nil, err
	}
	return schema, nil
}
