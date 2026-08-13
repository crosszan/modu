package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	"github.com/openmodu/modu/pkg/coding_agent/plugins/extension"
	"github.com/openmodu/modu/pkg/coding_agent/services/session"
	"github.com/openmodu/modu/pkg/coding_agent/sessionipc"
	"github.com/openmodu/modu/pkg/provider"
	"github.com/openmodu/modu/pkg/types"
)

const appServerStartupTimeout = 5 * time.Second

var ensureInteractiveAppServer = ensureSessionAppServer
var interactiveSessionIPCEnabled = true

func runAppServerCommand(args []string, stdout, stderr io.Writer) error {
	action := "start"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("modu_code app-server "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	agentDir := flags.String("agent-dir", coding_agent.DefaultAgentDir(), "Modu agent directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runtimeDir := sessionipc.DefaultRuntimeDir(*agentDir)

	switch action {
	case "serve":
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return serveSessionAppServer(ctx, *agentDir, runtimeDir)
	case "start":
		if _, err := ensureSessionAppServer(*agentDir); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "app-server running: %s\n", filepath.Join(runtimeDir, "ipc.sock"))
		return nil
	case "status":
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if err := sessionipc.Probe(ctx, runtimeDir); err != nil {
			fmt.Fprintln(stdout, "app-server stopped")
			return nil
		}
		pid, _ := readAppServerPID(runtimeDir)
		if pid > 0 {
			fmt.Fprintf(stdout, "app-server running: pid=%d socket=%s\n", pid, filepath.Join(runtimeDir, "ipc.sock"))
		} else {
			fmt.Fprintf(stdout, "app-server running: socket=%s\n", filepath.Join(runtimeDir, "ipc.sock"))
		}
		return nil
	case "stop":
		return stopSessionAppServer(runtimeDir, stdout)
	default:
		return fmt.Errorf("unknown app-server command %q (want start, status, stop, or serve)", action)
	}
}

func ensureSessionAppServer(agentDir string) (string, error) {
	runtimeDir := sessionipc.DefaultRuntimeDir(agentDir)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err := sessionipc.Probe(ctx, runtimeDir)
	cancel()
	if err == nil {
		return runtimeDir, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve modu_code executable: %w", err)
	}
	command := exec.Command(executable, "app-server", "serve", "--agent-dir", agentDir)
	configureDetachedProcess(command)
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("start Modu app-server: %w", err)
	}
	_ = command.Process.Release()

	deadline := time.Now().Add(appServerStartupTimeout)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		err = sessionipc.Probe(probeCtx, runtimeDir)
		probeCancel()
		if err == nil {
			return runtimeDir, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return "", fmt.Errorf("Modu app-server did not become ready at %s: %w", filepath.Join(runtimeDir, "ipc.sock"), err)
}

func serveSessionAppServer(ctx context.Context, agentDir, runtimeDir string) error {
	backend := coding_agent.NewSessionIPCBackend(agentDir, func(info session.SessionInfo) (*coding_agent.CodingSession, error) {
		return newAppServerSession(agentDir, info)
	})
	server, err := sessionipc.StartServer(sessionipc.ServerOptions{RuntimeDir: runtimeDir, Backend: backend})
	if err != nil {
		return err
	}
	defer func() {
		_ = server.Close()
		backend.Close()
	}()
	if err := writeAppServerPID(runtimeDir, os.Getpid()); err != nil {
		return err
	}
	defer removeOwnedAppServerPID(runtimeDir, os.Getpid())
	<-ctx.Done()
	return nil
}

func newAppServerSession(agentDir string, info session.SessionInfo) (*coding_agent.CodingSession, error) {
	model, getAPIKey := provider.Resolve()
	if model == nil {
		return nil, errors.New("no model provider is configured")
	}
	extensions, err := extension.LoadEnabled(extension.LoadOptions{ConfigPath: filepath.Join(agentDir, "extensions.yaml")})
	if err != nil {
		return nil, err
	}
	target, err := coding_agent.NewCodingSession(coding_agent.CodingSessionOptions{
		Cwd:               info.Cwd,
		AgentDir:          agentDir,
		Model:             model,
		ThinkingLevel:     provider.ResolveThinkingLevel(),
		GetAPIKey:         getAPIKey,
		ScopedModels:      provider.ConfiguredModelIDs(),
		ModelConfigPath:   provider.ConfigPath(),
		ResumeSessionID:   info.ID,
		Extensions:        extensions,
		ToolProvider:      newModuCodeToolProvider(),
		DeferStartupEvent: true,
	})
	if err != nil {
		return nil, err
	}
	if autoApprove, ok := target.GetAutoApprove(); !ok || !autoApprove {
		target.SetToolApprovalCallback(func(string, string, map[string]any) (types.ToolApprovalDecision, error) {
			return types.ToolApprovalDeny, errors.New("background app-server session requires a previously saved --no-approve setting")
		})
	}
	target.EmitStartupEvent()
	return target, nil
}

func stopSessionAppServer(runtimeDir string, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	err := sessionipc.Probe(ctx, runtimeDir)
	cancel()
	if err != nil {
		fmt.Fprintln(stdout, "app-server already stopped")
		return nil
	}
	pid, err := readAppServerPID(runtimeDir)
	if err != nil {
		return fmt.Errorf("read app-server pid: %w", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find app-server process %d: %w", pid, err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop app-server process %d: %w", pid, err)
	}
	deadline := time.Now().Add(appServerStartupTimeout)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err = sessionipc.Probe(probeCtx, runtimeDir)
		probeCancel()
		if err != nil {
			fmt.Fprintln(stdout, "app-server stopped")
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("app-server process %d did not stop", pid)
}

func appServerPIDPath(runtimeDir string) string { return filepath.Join(runtimeDir, "app-server.pid") }

func writeAppServerPID(runtimeDir string, pid int) error {
	path := appServerPIDPath(runtimeDir)
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600); err != nil {
		return fmt.Errorf("write app-server pid: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func readAppServerPID(runtimeDir string) (int, error) {
	data, err := os.ReadFile(appServerPIDPath(runtimeDir))
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, errors.New("invalid app-server pid file")
	}
	return pid, nil
}

func removeOwnedAppServerPID(runtimeDir string, pid int) {
	current, err := readAppServerPID(runtimeDir)
	if err == nil && current == pid {
		_ = os.Remove(appServerPIDPath(runtimeDir))
	}
}
