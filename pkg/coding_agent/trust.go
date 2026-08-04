package coding_agent

import (
	"fmt"
	"strings"

	trustservice "github.com/openmodu/modu/pkg/coding_agent/services/trust"
)

type ProjectTrustStatus struct {
	Directory string
	Decision  string
	Source    string
}

func (s *engine) GetProjectTrust() ProjectTrustStatus {
	status := ProjectTrustStatus{Decision: trustservice.Undecided.String()}
	if s == nil {
		return status
	}
	status.Directory = s.cwd
	if s.trustManager == nil {
		return status
	}
	result := s.trustManager.Status(s.cwd)
	status.Decision = result.Decision.String()
	if result.Path != "" {
		status.Directory = result.Path
	}
	switch {
	case result.Session:
		status.Source = "session"
	case result.Found:
		status.Source = "persistent"
	default:
		status.Source = "default"
	}
	return status
}

func (s *engine) ConfigureProjectTrust(mode string) (ProjectTrustStatus, error) {
	if s == nil || s.trustManager == nil {
		return ProjectTrustStatus{}, fmt.Errorf("project trust is not available")
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "status":
	case "on":
		if err := s.trustManager.SetPersistent(s.cwd, trustservice.Trusted); err != nil {
			return ProjectTrustStatus{}, err
		}
	case "off":
		if err := s.trustManager.SetPersistent(s.cwd, trustservice.Untrusted); err != nil {
			return ProjectTrustStatus{}, err
		}
	case "once":
		if err := s.trustManager.SetSession(s.cwd); err != nil {
			return ProjectTrustStatus{}, err
		}
	default:
		return ProjectTrustStatus{}, fmt.Errorf("usage: /trust [status|on|off|once]")
	}
	return s.GetProjectTrust(), nil
}
