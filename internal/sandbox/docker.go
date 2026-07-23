package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// dockerCreateTimeout bounds environment creation (docker run + setup
// commands), independent of RunRequest.Timeout — a first-time image pull can
// legitimately take far longer than a typical per-command bash timeout, but
// only happens once per environment.
const dockerCreateTimeout = 10 * time.Minute

// Docker labels applied to every sandbox container, used for reliable
// identification (docker ps --filter) — reaping orphans, and any future
// external tooling that wants to inspect what seshat has running — instead
// of relying on parsing the container name string.
const (
	labelSandbox       = "com.seshat.sandbox"
	labelEnvironmentID = "com.seshat.environment-id"
	labelProcessID     = "com.seshat.process-id"
	labelCreatedAt     = "com.seshat.created-at"
)

// DockerExecutor runs commands inside isolated Docker containers via the
// docker CLI (os/exec with structured argv — request text is passed as a
// single argv element to `sh -c`, never interpolated into a shell string;
// same safety pattern as NoopExecutor.Run).
//
// Each environment (RunRequest.Docker.EnvironmentID) gets one long-lived
// container, created lazily on first use and reused via `docker exec` for
// every subsequent call — this is what makes state (installed packages,
// background processes, …) persist across calls within the same
// environment, without spinning a fresh container per command.
//
// Isolation properties enforced: filesystem and process-namespace isolation
// via the container boundary, hard CPU/memory caps (--cpus/--memory), and
// optional full network isolation (--network=none). These are the two
// properties an earlier Dagger-SDK-based design could not provide — Dagger's
// public Go API (as of v0.21) exposes neither per-container resource limits
// nor a way to disable network access for a WithExec step.
//
// Scope: this backs local, single-user desktop sandboxing — not a
// multi-tenant sandbox-as-a-service platform. Multi-tenant concerns
// (Kubernetes/distributed scheduling, an ingress gateway for exposing
// sandbox-hosted services to third parties, a standalone credential vault,
// a formal third-party-runtime lifecycle protocol) are deliberately NOT
// built here — if a use case ever needs that, the right move is a
// sandbox.Executor adapter for an existing open-source platform (e.g.
// OpenSandbox) rather than reimplementing a worse copy of it inside seshat.
//
// Deliberately not attempted — --cap-drop=ALL: Docker's own default
// capability set is already reduced (not ALL), and dropping further risks
// breaking legitimate tools (e.g. CAP_SETUID/CAP_SETGID, which
// privilege-dropping install scripts rely on) for a security benefit with no
// articulated compromise scenario it would prevent that NET_RAW/NET_ADMIN
// (already dropped below) and runtime selection (below) don't already cover.
//
// Done:
//   - Strong isolation runtime selection (DockerExecutorConfig.Runtime,
//     e.g. "runsc" for gVisor) — plumbed and tested against `runc` as a
//     stand-in (this machine has no gVisor installed to verify against for
//     real: `docker info` lists only runc/nvidia-container-runtime).
//     Actually installing and registering gVisor with the Docker daemon is
//     an operator/host setup step outside what Go code here can do;
//     Healthy(ctx) refuses to proceed if the configured Runtime isn't
//     registered, rather than silently falling back to plain runc.
//   - Orphan container cleanup (labels + reaper on construction, see
//     reapOrphanContainers) — a crashed host process's containers are
//     cleaned up by the next one, not left running indefinitely.
//   - NET_RAW/NET_ADMIN dropped by default (--cap-drop) — narrows the
//     capability set Docker already reduces further, with no realistic
//     legitimate-tool breakage (package managers/builds/git all use
//     ordinary TCP, not raw sockets or network device configuration).
//   - Port tunneling (DockerExecutorConfig.PublishPorts / RunResult.Endpoints)
//     — useful even for local usage (e.g. previewing a dev server an agent
//     started inside the sandbox). Declared once at the environment level,
//     not per RunRequest, because Docker only allows publishing ports at
//     container-creation time.
//   - Git-tracked history of file changes (DockerExecutorConfig.
//     TrackFileChanges) — a product feature (undo/diff of what an agent
//     did), not a security property, but implemented: snapshots
//     RunRequest.WorkDir into a separate bare shadow repo (never the
//     project's own .git, which is explicitly excluded) after any Run call
//     that actually changed something, keyed by a hash of the absolute
//     WorkDir path so history survives across environment/process restarts
//     for the same project directory. Off by default (requires git on the
//     host; Healthy(ctx) fails loudly rather than silently skipping
//     tracking if TrackFileChanges is set but git can't be found).
//
// Already true today, not a gap: RunRequest.Env / DockerExecutorConfig.Env
// are the only environment variables that reach the container — unlike
// NoopExecutor.Run (which inherits the full host process environment via
// cmd.Environ()), DockerExecutor never implicitly forwards os.Environ()
// into the sandbox, so host-side API keys and other secrets the seshat
// process holds are not exposed to a sandboxed command unless explicitly
// passed.
type DockerExecutor struct {
	cfg DockerExecutorConfig

	mu   sync.Mutex
	envs map[string]*dockerEnv // keyed by EnvironmentID ("" = default)
}

// dockerEnv tracks one persistent environment container.
type dockerEnv struct {
	containerID string
	// workDir is the host path bind-mounted as cfg.WorkDir. Fixed at
	// environment creation from the first Run call's RunRequest.WorkDir;
	// later calls to the same EnvironmentID reuse this container regardless
	// of what WorkDir they pass.
	workDir   string
	createdAt time.Time
	// endpoints maps DockerExecutorConfig.PublishPorts entries to their
	// resolved "host:port" strings, resolved once at creation (port
	// bindings can't change for the container's lifetime) and returned on
	// every subsequent Run call for this environment via RunResult.Endpoints.
	endpoints map[int]string
	// historyGitDir is the bare shadow git repo tracking workDir's changes,
	// set only when DockerExecutorConfig.TrackFileChanges is true and
	// workDir is non-empty. Empty means tracking is off for this environment.
	historyGitDir string
	// lastSnapshotErr records the most recent gitSnapshot failure, if any —
	// snapshotAfterRun treats a failed snapshot as best-effort (never fails
	// the Run() it's attached to), but silently dropping the error entirely
	// makes "nothing changed" indistinguishable from "the snapshot failed"
	// from the outside. Guarded by DockerExecutor.mu, same as the map this
	// dockerEnv lives in.
	lastSnapshotErr error
}

// processInstanceID tags every container this process creates
// (com.seshat.process-id label) so a later process can tell its own
// containers apart from a previous instance's leftovers when reaping
// orphans. Best-effort — even a degenerate (all-zero) value still works as
// a coordination tag, it just loses uniqueness, so failures are ignored.
var processInstanceID = newProcessInstanceID()

func newProcessInstanceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// newDockerExecutor is the internal constructor called by NewExecutor when
// Kind == EnvironmentDocker. Reaps orphaned sandbox containers left behind
// by a crashed previous process instance (best-effort, does not fail
// construction if Docker isn't reachable — Healthy(ctx) is the authoritative
// startup readiness check). Otherwise doesn't touch Docker further at
// construction time; environments are created lazily on first Run.
func newDockerExecutor(cfg DockerExecutorConfig) (*DockerExecutor, error) {
	def := DefaultDockerConfig()
	if cfg.BaseImage == "" {
		cfg.BaseImage = def.BaseImage
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = def.WorkDir
	}
	if cfg.DockerBinary == "" {
		cfg.DockerBinary = def.DockerBinary
	}
	if cfg.MemoryLimitMB == 0 {
		cfg.MemoryLimitMB = def.MemoryLimitMB
	}
	if cfg.CPULimit == 0 {
		cfg.CPULimit = def.CPULimit
	}
	if cfg.HistoryDir == "" {
		cfg.HistoryDir = def.HistoryDir
	}
	if cfg.GitBinary == "" {
		cfg.GitBinary = def.GitBinary
	}
	e := &DockerExecutor{
		cfg:  cfg,
		envs: make(map[string]*dockerEnv),
	}
	reapCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	e.reapOrphanContainers(reapCtx)
	cancel()
	return e, nil
}

// reapOrphanContainers removes every container labeled com.seshat.sandbox=true
// whose com.seshat.process-id label doesn't match this process's — i.e. left
// running by a previous process instance that never called Close() (a crash,
// not a graceful shutdown; --rm alone doesn't clean these up because the
// container isn't tied to the docker CLI invocation that started it).
// Best-effort: any error (docker missing, daemon down, …) is swallowed —
// Healthy(ctx) is what the caller uses to decide whether Docker sandboxing
// is usable at all.
func (e *DockerExecutor) reapOrphanContainers(ctx context.Context) {
	cmd := exec.CommandContext(ctx, e.cfg.DockerBinary, "ps", "-a",
		"--filter", "label="+labelSandbox+"=true",
		"--format", `{{.ID}} {{.Label "`+labelProcessID+`"}}`,
	)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		containerID := fields[0]
		procID := ""
		if len(fields) > 1 {
			procID = strings.TrimSpace(fields[1])
		}
		if procID == processInstanceID {
			continue
		}
		_ = e.stopContainer(containerID)
	}
}

func (e *DockerExecutor) Kind() EnvironmentKind { return EnvironmentDocker }

// Healthy runs `docker version` against the daemon. A non-nil error means
// Docker isn't installed, isn't running, or isn't reachable — the engine
// calls this once at startup so the host application can fall back
// (typically to NoopExecutor with a visible warning) instead of failing
// every subsequent bash call individually.
func (e *DockerExecutor) Healthy(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.cfg.DockerBinary, "version", "--format", "{{.Server.Version}}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker sandbox unavailable: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if e.cfg.Runtime != "" {
		available, err := e.runtimeRegistered(ctx, e.cfg.Runtime)
		if err != nil {
			return fmt.Errorf("checking docker runtime %q availability: %w", e.cfg.Runtime, err)
		}
		if !available {
			return fmt.Errorf(
				"docker runtime %q requested but not registered with the daemon — install and register it "+
					"(e.g. gVisor's runsc) or clear DockerExecutorConfig.Runtime to use the default runtime",
				e.cfg.Runtime,
			)
		}
	}
	if e.cfg.TrackFileChanges {
		if _, err := exec.LookPath(e.cfg.GitBinary); err != nil {
			return fmt.Errorf(
				"DockerExecutorConfig.TrackFileChanges is set but %q was not found on PATH: %w",
				e.cfg.GitBinary, err,
			)
		}
		if e.cfg.HistoryDir == "" {
			return fmt.Errorf("DockerExecutorConfig.TrackFileChanges is set but HistoryDir could not be determined (os.UserConfigDir failed) — set it explicitly")
		}
	}
	return nil
}

// runtimeRegistered reports whether name is a key in `docker info`'s
// Runtimes map — i.e. actually registered with the daemon, not just a
// plausible-looking string.
func (e *DockerExecutor) runtimeRegistered(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, e.cfg.DockerBinary, "info", "--format", "{{json .Runtimes}}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var runtimes map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &runtimes); err != nil {
		return false, fmt.Errorf("parse docker info runtimes: %w", err)
	}
	_, ok := runtimes[name]
	return ok, nil
}

func (e *DockerExecutor) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	envID := ""
	if req.Docker != nil {
		envID = req.Docker.EnvironmentID
	}

	env, err := e.ensureEnvironment(ctx, envID, req.WorkDir)
	if err != nil {
		return RunResult{}, err
	}

	shell := req.Shell
	if len(shell) == 0 {
		shell = []string{"sh", "-c"}
	}

	args := []string{"exec", "-i"}
	if req.WorkDir != "" {
		// Always the container-side path — RunRequest.WorkDir (a host path)
		// only determined the bind-mount source at environment creation.
		args = append(args, "-w", e.cfg.WorkDir)
	}
	for k, v := range req.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, env.containerID)
	args = append(args, shell...)
	args = append(args, req.Command)

	cmdCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(cmdCtx, e.cfg.DockerBinary, args...)
	if req.Stdin != nil {
		cmd.Stdin = req.Stdin
	}

	var stdout, stderr bytes.Buffer
	if !req.Background {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start)

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			runErr = nil // non-zero exit is not a Run error — mirrors NoopExecutor
		}
	}

	e.snapshotAfterRun(env, req.Command)

	return RunResult{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
		Duration:  dur,
		Endpoints: env.endpoints,
	}, runErr
}

// ensureEnvironment returns the existing container for envID, or creates one
// (docker run, then SetupCommands in order) if this is the first call.
func (e *DockerExecutor) ensureEnvironment(ctx context.Context, envID, workDir string) (*dockerEnv, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if env, ok := e.envs[envID]; ok {
		return env, nil
	}

	createCtx, cancel := context.WithTimeout(ctx, dockerCreateTimeout)
	defer cancel()

	containerName, err := dockerContainerName(envID)
	if err != nil {
		return nil, err
	}

	envLabel := envID
	if envLabel == "" {
		envLabel = "default"
	}
	args := []string{
		"run", "-d", "--rm", "--name", containerName, "--security-opt", "no-new-privileges",
		// Docker's own default capability set is already reduced (not ALL),
		// but still includes NET_RAW/NET_ADMIN — raw sockets and network
		// device configuration, neither needed by typical shell/dev workloads
		// (package managers, builds, git all run over ordinary TCP). Dropping
		// them narrows the surface without the breakage risk of --cap-drop=ALL
		// (which would also remove capabilities legitimate tools do rely on,
		// e.g. CAP_SETUID/CAP_SETGID for privilege-dropping install scripts).
		"--cap-drop", "NET_RAW", "--cap-drop", "NET_ADMIN",
		"--label", labelSandbox + "=true",
		"--label", labelEnvironmentID + "=" + envLabel,
		"--label", labelProcessID + "=" + processInstanceID,
		"--label", labelCreatedAt + "=" + time.Now().UTC().Format(time.RFC3339),
	}
	if workDir != "" {
		args = append(args, "-v", workDir+":"+e.cfg.WorkDir)
	}
	args = append(args, "-w", e.cfg.WorkDir)
	if !e.cfg.NetworkAccess {
		args = append(args, "--network=none")
	}
	if e.cfg.MemoryLimitMB > 0 {
		args = append(args, "--memory", strconv.Itoa(e.cfg.MemoryLimitMB)+"m")
	}
	if e.cfg.CPULimit > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(e.cfg.CPULimit, 'f', -1, 64))
	}
	if e.cfg.Runtime != "" {
		// Healthy(ctx) already verified this runtime is registered — an
		// unregistered value here would fail `docker run` outright rather
		// than silently falling back to the default runtime, which is the
		// intended behavior (see DockerExecutorConfig.Runtime).
		args = append(args, "--runtime", e.cfg.Runtime)
	}
	for _, port := range e.cfg.PublishPorts {
		// No host-side port given — "-p <containerPort>" alone publishes to
		// a random free host port, resolved afterward via `docker port`.
		args = append(args, "-p", strconv.Itoa(port))
	}
	for k, v := range e.cfg.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, e.cfg.BaseImage, "sleep", "infinity")

	cmd := exec.CommandContext(createCtx, e.cfg.DockerBinary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("create sandbox environment: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	containerID := strings.TrimSpace(stdout.String())

	for _, setupCmd := range e.cfg.SetupCommands {
		setupArgs := []string{"exec", containerID, "sh", "-c", setupCmd}
		setup := exec.CommandContext(createCtx, e.cfg.DockerBinary, setupArgs...)
		var setupErr bytes.Buffer
		setup.Stderr = &setupErr
		if err := setup.Run(); err != nil {
			_ = e.stopContainer(containerID)
			return nil, fmt.Errorf("sandbox setup command %q failed: %w: %s", setupCmd, err, strings.TrimSpace(setupErr.String()))
		}
	}

	var endpoints map[int]string
	if len(e.cfg.PublishPorts) > 0 {
		endpoints, err = e.resolvePublishedPorts(createCtx, containerID)
		if err != nil {
			_ = e.stopContainer(containerID)
			return nil, fmt.Errorf("resolve published ports: %w", err)
		}
	}

	var historyGitDir string
	if e.cfg.TrackFileChanges && workDir != "" {
		historyGitDir, err = e.initHistoryTracking(createCtx, workDir)
		if err != nil {
			_ = e.stopContainer(containerID)
			return nil, fmt.Errorf("init file-change tracking: %w", err)
		}
	}

	env := &dockerEnv{
		containerID:   containerID,
		workDir:       workDir,
		createdAt:     time.Now(),
		endpoints:     endpoints,
		historyGitDir: historyGitDir,
	}
	e.envs[envID] = env
	return env, nil
}

// historyGitDirFor derives a stable shadow-repo path for workDir, keyed by
// a hash of its absolute path — so re-running against the same project
// directory continues the same history instead of starting fresh every
// time a new environment/container is created for it.
func (e *DockerExecutor) historyGitDirFor(workDir string) string {
	abs, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		abs = workDir
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(e.cfg.HistoryDir, hex.EncodeToString(sum[:16])+".git")
}

// initHistoryTracking ensures a bare shadow git repository exists for
// workDir (creating one and taking an initial snapshot commit the first
// time this exact directory is tracked) and returns its path. Deliberately
// separate from any .git the project directory already has — this never
// touches the project's own repository, and workDir's own .git (if any) is
// added to the shadow repo's exclude list so it's never snapshotted.
func (e *DockerExecutor) initHistoryTracking(ctx context.Context, workDir string) (string, error) {
	gitDir := e.historyGitDirFor(workDir)
	if _, err := os.Stat(gitDir); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.MkdirAll(e.cfg.HistoryDir, 0o700); err != nil {
			return "", fmt.Errorf("create history dir: %w", err)
		}
		initCmd := exec.CommandContext(ctx, e.cfg.GitBinary, "init", "--bare", "-q", gitDir)
		var stderr bytes.Buffer
		initCmd.Stderr = &stderr
		if err := initCmd.Run(); err != nil {
			return "", fmt.Errorf("git init --bare: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		excludePath := filepath.Join(gitDir, "info", "exclude")
		if err := os.MkdirAll(filepath.Dir(excludePath), 0o700); err == nil {
			_ = os.WriteFile(excludePath, []byte(".git\n"), 0o600)
		}
	}
	if err := e.gitSnapshot(ctx, gitDir, workDir, "initial snapshot"); err != nil {
		return "", err
	}
	return gitDir, nil
}

// gitSnapshot commits any changes in workDir since the last snapshot into
// gitDir. A no-op if nothing changed (never creates empty commits).
func (e *DockerExecutor) gitSnapshot(ctx context.Context, gitDir, workDir, message string) error {
	gitArgs := func(args ...string) []string {
		return append([]string{
			"--git-dir", gitDir, "--work-tree", workDir,
			"-c", "user.name=seshat-sandbox", "-c", "user.email=sandbox@seshat.local",
		}, args...)
	}

	statusCmd := exec.CommandContext(ctx, e.cfg.GitBinary, gitArgs("status", "--porcelain")...)
	var statusOut, statusErr bytes.Buffer
	statusCmd.Stdout = &statusOut
	statusCmd.Stderr = &statusErr
	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("git status: %w: %s", err, strings.TrimSpace(statusErr.String()))
	}
	if strings.TrimSpace(statusOut.String()) == "" {
		return nil
	}

	addCmd := exec.CommandContext(ctx, e.cfg.GitBinary, gitArgs("add", "-A")...)
	var addErr bytes.Buffer
	addCmd.Stderr = &addErr
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add: %w: %s", err, strings.TrimSpace(addErr.String()))
	}

	commitCmd := exec.CommandContext(ctx, e.cfg.GitBinary, gitArgs("commit", "-q", "-m", message)...)
	var commitErr bytes.Buffer
	commitCmd.Stderr = &commitErr
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit: %w: %s", err, strings.TrimSpace(commitErr.String()))
	}
	return nil
}

// snapshotAfterRun is called after every Run on an environment that has
// TrackFileChanges enabled. Best-effort: errors are dropped rather than
// failing the command's own result — a snapshot failure (e.g. a transient
// git error) shouldn't make an otherwise-successful command look failed.
// Uses its own timeout, independent of the command's, so a slow snapshot on
// a large workDir doesn't fight over the command's own deadline.
func (e *DockerExecutor) snapshotAfterRun(env *dockerEnv, command string) {
	if env.historyGitDir == "" {
		return
	}
	message := command
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		message = message[:idx] + "…"
	}
	if len(message) > 200 {
		message = message[:200] + "…"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := e.gitSnapshot(ctx, env.historyGitDir, env.workDir, "cmd: "+message)
	e.mu.Lock()
	env.lastSnapshotErr = err
	e.mu.Unlock()
}

// resolvePublishedPorts reads back the host-side ports Docker assigned to
// containerID's published container ports (docker run -p <port> lets Docker
// pick a free host port — this is how the caller finds out which one).
func (e *DockerExecutor) resolvePublishedPorts(ctx context.Context, containerID string) (map[int]string, error) {
	cmd := exec.CommandContext(ctx, e.cfg.DockerBinary, "port", containerID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	endpoints := make(map[int]string, len(e.cfg.PublishPorts))
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Each line looks like "3000/tcp -> 0.0.0.0:54321" — a port with
		// both IPv4 and IPv6 bindings produces two lines; the first one
		// seen per container port wins.
		portProto, hostAddr, ok := strings.Cut(line, " -> ")
		if !ok {
			continue
		}
		proto, ok := strings.CutSuffix(portProto, "/tcp")
		if !ok {
			proto, ok = strings.CutSuffix(portProto, "/udp")
			if !ok {
				continue
			}
		}
		containerPort, err := strconv.Atoi(proto)
		if err != nil {
			continue
		}
		if _, exists := endpoints[containerPort]; exists {
			continue
		}
		hostPort := strings.TrimSpace(hostAddr)
		if idx := strings.LastIndexByte(hostPort, ':'); idx >= 0 {
			hostPort = "localhost:" + hostPort[idx+1:]
		}
		endpoints[containerPort] = hostPort
	}
	return endpoints, nil
}

// Close stops and removes every environment container this executor has
// created (--rm at creation makes stop trigger automatic removal). Safe to
// call multiple times. Host applications should call this at shutdown —
// DockerExecutor is not wired into any automatic engine-wide shutdown hook
// yet, so the caller that owns the Executor is responsible for this today.
func (e *DockerExecutor) Close() error {
	e.mu.Lock()
	envs := e.envs
	e.envs = make(map[string]*dockerEnv)
	e.mu.Unlock()

	var errs []string
	for id, env := range envs {
		if err := e.stopContainer(env.containerID); err != nil {
			errs = append(errs, fmt.Sprintf("environment %q (container %s): %v", id, env.containerID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to clean up %d sandbox environment(s): %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func (e *DockerExecutor) stopContainer(containerID string) error {
	cmd := exec.Command(e.cfg.DockerBinary, "stop", "--time", "5", containerID)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// dockerContainerName builds a container name that is always unique
// (random suffix) so a stale container left behind by a previous process
// crash never collides with a fresh one, with envID kept as a human-readable
// hint for `docker ps`.
func dockerContainerName(envID string) (string, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate sandbox container name: %w", err)
	}
	hint := sanitizeDockerNameHint(envID)
	if hint == "" {
		hint = "default"
	}
	return fmt.Sprintf("seshat-sbx-%s-%s", hint, hex.EncodeToString(suffix)), nil
}

// sanitizeDockerNameHint strips envID down to characters valid in a Docker
// container name, truncated to a reasonable length. Purely a debugging aid
// (`docker ps` readability) — never used to look up a container by name.
func sanitizeDockerNameHint(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.', r == '-':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}
