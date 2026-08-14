package main

import (
	"fmt"
	"strings"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
)

// normalizeResumeArgs rewrites a valueless `--resume` into `--resume=`, so the
// flag package records it as set-but-empty instead of swallowing the next
// argument or failing with "flag needs an argument". Bare --resume means "the
// latest session saved for this directory"; `--resume <id>` and
// `--resume=<id>` keep their existing meaning.
func normalizeResumeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if (arg == "-resume" || arg == "--resume") && !resumeValueFollows(args, i) {
			arg = "--resume="
		}
		out = append(out, arg)
	}
	return out
}

// resumeValueFollows reports whether the argument after --resume at index i is
// its value. Another flag, or nothing at all, means the user asked for the
// latest session instead of naming one.
func resumeValueFollows(args []string, i int) bool {
	if i+1 >= len(args) {
		return false
	}
	next := args[i+1]
	return next == "-" || !strings.HasPrefix(next, "-")
}

// resolveStartupResumeID turns the parsed --resume value into a session id. A
// set-but-empty flag resumes the newest session recorded for cwd, so plain
// `modu_code --resume` picks up where this directory left off.
func resolveStartupResumeID(agentDir, cwd, flagValue string, flagSet bool) (string, error) {
	id := strings.TrimSpace(flagValue)
	if id != "" || !flagSet {
		return id, nil
	}
	latest, ok, err := coding_agent.LatestSessionIDForCwd(agentDir, cwd)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no saved session found for %s", cwd)
	}
	return latest, nil
}
