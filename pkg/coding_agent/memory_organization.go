package coding_agent

import (
	"context"
	"fmt"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/services/memory"
)

const memoryOrganizationTimeout = 2 * time.Minute

// OrganizeMemory forces one bounded, non-destructive memory-summary pass.
func (s *engine) OrganizeMemory(ctx context.Context) (memory.OrganizationResult, error) {
	if s == nil || s.memoryStore == nil || !memoryFeatureEnabled(s.config) {
		return memory.OrganizationResult{}, fmt.Errorf("memory is disabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, memoryOrganizationTimeout)
	defer cancel()
	result, err := s.memoryStore.Organize(ctx, s.memoryOrganizationOptions(true))
	s.writeRuntimeState()
	return result, err
}

// GetMemoryOrganizationStatus returns the persisted process state.
func (s *engine) GetMemoryOrganizationStatus() memory.OrganizationState {
	if s == nil || s.memoryStore == nil {
		return memory.OrganizationState{Status: "unavailable"}
	}
	if !memoryFeatureEnabled(s.config) {
		return memory.OrganizationState{Status: "disabled"}
	}
	return s.memoryStore.OrganizationStatus()
}

// MemoryContextStats reports the size and source of the memory block most
// recently injected into a prompt.
func (s *engine) MemoryContextStats() memory.ContextStats {
	if s == nil || s.memoryStore == nil {
		return memory.ContextStats{}
	}
	return s.memoryStore.ContextStats()
}

func (s *engine) maybeAutoOrganizeMemory() {
	if s == nil || s.memoryStore == nil || s.config == nil ||
		!memoryFeatureEnabled(s.config) || !s.config.MemoryAutoOrganize() {
		return
	}
	opts := s.memoryOrganizationOptions(false)
	if should, _ := s.memoryStore.ShouldOrganize(opts); !should {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), memoryOrganizationTimeout)
		defer cancel()
		_, _ = s.memoryStore.Organize(ctx, opts)
		s.writeRuntimeState()
	}()
}

func (s *engine) memoryOrganizationOptions(force bool) memory.OrganizeOptions {
	return memory.OrganizeOptions{
		Model:          s.model,
		StreamFn:       s.streamFn,
		GetAPIKey:      s.getAPIKey,
		ThresholdBytes: s.config.MemoryOrganizeThresholdBytes(),
		RecentDays:     s.config.MemoryRecentDailyDays(),
		MinInterval:    time.Duration(s.config.MemoryOrganizeIntervalHours()) * time.Hour,
		Force:          force,
	}
}
