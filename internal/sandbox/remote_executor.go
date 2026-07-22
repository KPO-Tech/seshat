package sandbox

import "sync"

// remoteExecutors is a package-level, session-keyed registry of Executors
// that bash.Tool consults (via RemoteExecutorFor) before falling back to its
// own local exec.CommandContext path. Populated by the host application
// (seshat-backend) when a session has a live remote execution channel
// available — concretely, Electron's terminal relay, so the agent's bash
// calls run inside the user's real, visible shell instead of a detached
// subprocess. Mirrors the package-level singleton convention already used by
// bash.BackgroundTaskManager for "attach to session state from tool code".
var remoteExecutors sync.Map // sessionID (string) -> Executor

// RegisterRemoteExecutor attaches executor as sessionID's remote execution
// backend. A re-registration (e.g. a reconnect) replaces any previous one.
func RegisterRemoteExecutor(sessionID string, executor Executor) {
	remoteExecutors.Store(sessionID, executor)
}

// UnregisterRemoteExecutor detaches sessionID's remote execution backend —
// subsequent bash calls for that session fall back to local execution.
func UnregisterRemoteExecutor(sessionID string) {
	remoteExecutors.Delete(sessionID)
}

// RemoteExecutorFor looks up sessionID's registered remote executor, if any.
func RemoteExecutorFor(sessionID string) (Executor, bool) {
	v, ok := remoteExecutors.Load(sessionID)
	if !ok {
		return nil, false
	}
	executor, ok := v.(Executor)
	return executor, ok
}
