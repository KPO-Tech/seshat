package sandbox

// EnvironmentKind describes where a tool executes.
// The policy layer should know this, even if the actual backend runtime
// (local process, docker, remote host) is implemented elsewhere.
type EnvironmentKind string

const (
	EnvironmentLocal    EnvironmentKind = "local"
	EnvironmentWorktree EnvironmentKind = "worktree"
	EnvironmentDocker   EnvironmentKind = "docker"
	EnvironmentRemote   EnvironmentKind = "remote"
	EnvironmentUnknown  EnvironmentKind = "unknown"
)

// AccessKind describes the resource access being requested.
type AccessKind string

const (
	AccessRead     AccessKind = "read"
	AccessWrite    AccessKind = "write"
	AccessCreate   AccessKind = "create"
	AccessDelete   AccessKind = "delete"
	AccessSearch   AccessKind = "search"
	AccessExecute  AccessKind = "execute"
	AccessNetwork  AccessKind = "network"
	AccessEscalate AccessKind = "escalate"
)

// Decision is the normalized outcome of a sandbox/policy check.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

// DecisionResult is the normalized output of a policy decision.
type DecisionResult struct {
	Decision Decision
	Reason   string
}

// PathDecision includes the resolved path for filesystem checks.
type PathDecision struct {
	DecisionResult
	ResolvedPath string
}
