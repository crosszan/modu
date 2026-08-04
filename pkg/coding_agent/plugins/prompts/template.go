package prompts

import (
	"strconv"
	"strings"

	"github.com/kballard/go-shellquote"
)

// SplitArgs tokenizes a template invocation using shell quoting rules.
func SplitArgs(input string) ([]string, error) {
	args, err := shellquote.Split(input)
	if err != nil {
		return nil, err
	}
	if args == nil {
		return []string{}, nil
	}
	return args, nil
}

// ExpandTemplate expands positional, all-argument, default, and slice
// placeholders. With no placeholder, arguments are appended after a blank line.
func ExpandTemplate(template string, args []string) string {
	if !hasTemplatePlaceholder(template) {
		if len(args) == 0 {
			return template
		}
		return template + "\n\n" + strings.Join(args, " ")
	}

	var b strings.Builder
	for i := 0; i < len(template); {
		if template[i] != '$' || i+1 >= len(template) {
			b.WriteByte(template[i])
			i++
			continue
		}

		switch next := template[i+1]; {
		case next == '{':
			end := strings.IndexByte(template[i+2:], '}')
			if end < 0 {
				b.WriteByte(template[i])
				i++
				continue
			}
			b.WriteString(expandBraced(template[i+2:i+2+end], args))
			i += end + 3
		case next == '@':
			b.WriteString(strings.Join(args, " "))
			i += 2
		case isDigit(next):
			end := i + 1
			for end < len(template) && isDigit(template[end]) {
				end++
			}
			index, _ := strconv.Atoi(template[i+1 : end])
			b.WriteString(positionalArg(args, index))
			i = end
		case strings.HasPrefix(template[i+1:], "ARGUMENTS"):
			b.WriteString(strings.Join(args, " "))
			i += len("$ARGUMENTS")
		default:
			b.WriteByte(template[i])
			i++
		}
	}
	return b.String()
}

func hasTemplatePlaceholder(template string) bool {
	for i := 0; i+1 < len(template); i++ {
		if template[i] != '$' {
			continue
		}
		next := template[i+1]
		if next == '{' || next == '@' || isDigit(next) || strings.HasPrefix(template[i+1:], "ARGUMENTS") {
			return true
		}
	}
	return false
}

func expandBraced(value string, args []string) string {
	if index := strings.Index(value, ":-"); index >= 0 {
		name, fallback := value[:index], value[index+2:]
		if name == "@" || name == "ARGUMENTS" || isPositiveInteger(name) {
			if expanded := expandPlain(name, args); expanded != "" {
				return expanded
			}
			return fallback
		}
	}
	if index := strings.IndexByte(value, ':'); index >= 0 {
		name := value[:index]
		if name != "@" && name != "ARGUMENTS" {
			return ""
		}
		return expandSlice(value[index+1:], args)
	}
	return expandPlain(value, args)
}

func isPositiveInteger(value string) bool {
	if value == "" {
		return false
	}
	for i := range value {
		if !isDigit(value[i]) {
			return false
		}
	}
	index, err := strconv.Atoi(value)
	return err == nil && index > 0
}

func expandPlain(name string, args []string) string {
	if name == "@" || name == "ARGUMENTS" {
		return strings.Join(args, " ")
	}
	index, err := strconv.Atoi(name)
	if err != nil {
		return ""
	}
	return positionalArg(args, index)
}

func positionalArg(args []string, index int) string {
	if index < 1 || index > len(args) {
		return ""
	}
	return args[index-1]
}

func expandSlice(value string, args []string) string {
	parts := strings.SplitN(value, ":", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil || start < 1 || start > len(args) {
		return ""
	}
	end := len(args)
	if len(parts) == 2 {
		length, err := strconv.Atoi(parts[1])
		if err != nil || length < 0 {
			return ""
		}
		end = min(start-1+length, len(args))
	}
	return strings.Join(args[start-1:end], " ")
}

func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
