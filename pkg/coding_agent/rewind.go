package coding_agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type RewindPointInfo struct {
	Number    int
	Time      time.Time
	Label     string
	FileCount int
}

type RewindResult struct {
	Point         RewindPointInfo
	RestoredFiles []string
}

func (s *engine) beginRewindTurn(label string) {
	if s == nil || s.rewindRecorder == nil {
		return
	}
	leafID := ""
	if s.sessionManager != nil {
		leafID = s.sessionManager.LastID()
	}
	s.rewindRecorder.Begin(leafID, shortRewindLabel(label))
}

func (s *engine) finishRewindTurn() {
	if s != nil && s.rewindRecorder != nil {
		s.rewindRecorder.Commit()
	}
}

func (s *engine) GetRewindPoints() []RewindPointInfo {
	if s == nil || s.rewindRecorder == nil {
		return nil
	}
	points := s.rewindRecorder.Points()
	result := make([]RewindPointInfo, 0, len(points))
	for _, point := range points {
		result = append(result, RewindPointInfo{
			Number:    point.Number,
			Time:      point.Time,
			Label:     point.Label,
			FileCount: point.FileCount,
		})
	}
	return result
}

// Rewind restores one numbered file checkpoint and moves the active
// conversation to the leaf that preceded that turn.
func (s *engine) Rewind(number int) (RewindResult, error) {
	if s == nil || s.rewindRecorder == nil || s.sessionManager == nil {
		return RewindResult{}, fmt.Errorf("rewind is unavailable")
	}
	if s.agent != nil && s.agent.GetState().IsStreaming {
		return RewindResult{}, fmt.Errorf("cannot rewind while the agent is running")
	}
	points := s.rewindRecorder.Points()
	if number < 1 || number > len(points) {
		return RewindResult{}, fmt.Errorf("restore point %d out of range (have %d)", number, len(points))
	}
	target := points[number-1]
	if target.LeafID != "" {
		if _, ok := s.sessionManager.GetEntry(target.LeafID); !ok {
			return RewindResult{}, fmt.Errorf("conversation entry %s is no longer available", target.LeafID)
		}
	}

	point, restored, err := s.rewindRecorder.Restore(number - 1)
	if err != nil {
		return RewindResult{}, err
	}
	if point.LeafID == "" {
		s.sessionManager.ResetLeaf()
		s.agent.ReplaceMessages(nil)
		s.ctxMgr.ResetUsage()
		s.lastSavedIndex = 0
	} else {
		if err := s.sessionManager.Fork(point.LeafID); err != nil {
			return RewindResult{}, err
		}
		if _, err := s.RestoreMessages(); err != nil {
			return RewindResult{}, err
		}
	}
	s.writeRuntimeState()
	return RewindResult{
		Point: RewindPointInfo{
			Number:    target.Number,
			Time:      target.Time,
			Label:     target.Label,
			FileCount: target.FileCount,
		},
		RestoredFiles: restored,
	}, nil
}

func (s *engine) DisplayRewindPath(path string) string {
	if s == nil || s.cwd == "" {
		return path
	}
	relative, err := filepath.Rel(s.cwd, path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return relative
	}
	return path
}

func shortRewindLabel(label string) string {
	label = strings.Join(strings.Fields(label), " ")
	runes := []rune(label)
	const maxRunes = 60
	if len(runes) > maxRunes {
		label = string(runes[:maxRunes-1]) + "…"
	}
	return label
}
