package coding_agent

import (
	"errors"
	"sync"

	"github.com/openmodu/modu/pkg/coding_agent/trajectory"
)

var errNoTrajectorySource = errors.New("no session bound")

type trajectoryProvider interface {
	Trajectory(opts trajectory.Options) (trajectory.Trajectory, error)
}

// trajectoryProxy lets the get_trajectory tool be built before the session it
// reports on exists: tools are constructed while the session is still being
// assembled, and the session binds itself as the source once it is ready.
type trajectoryProxy struct {
	mu     sync.RWMutex
	source trajectoryProvider
}

func (p *trajectoryProxy) SetSource(source trajectoryProvider) {
	p.mu.Lock()
	p.source = source
	p.mu.Unlock()
}

func (p *trajectoryProxy) Trajectory(opts trajectory.Options) (trajectory.Trajectory, error) {
	p.mu.RLock()
	source := p.source
	p.mu.RUnlock()
	if source == nil {
		return trajectory.Trajectory{}, errNoTrajectorySource
	}
	return source.Trajectory(opts)
}
