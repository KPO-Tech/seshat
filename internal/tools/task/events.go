package task

import (
	"context"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func emitTaskRuntimeEvent(ctx context.Context, sessionID string, action string, task *Task) {
	emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent))
	if !ok || emitter == nil || sessionID == "" {
		return
	}
	event := types.RuntimeEvent{
		Type:      types.RuntimeEventTypeTaskChanged,
		SessionID: types.SessionID(sessionID),
		Timestamp: time.Now().UTC(),
		TaskEvent: &types.TaskRuntimeEvent{Action: action},
	}
	if task != nil {
		event.TaskEvent.TaskID = task.ID
		event.TaskEvent.Status = task.Status
		event.TaskEvent.Subject = task.Subject
	}
	emitter(event)
}
