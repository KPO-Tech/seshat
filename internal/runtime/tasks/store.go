package tasks

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (m *Manager) tasksStatePath() string {
	return filepath.Join(m.config.OutputDir, tasksStateFileName)
}

// persistTasksLocked writes the reconnectable task snapshot used across manager
// restarts. Only serializable lifecycle state is persisted; live process handles
// and cancel functions are always stripped.
func (m *Manager) persistTasksLocked() {
	persistable := make([]*Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		cloned := cloneTask(task)
		cloned.cancel = nil
		cloned.cmd = nil
		persistable = append(persistable, cloned)
	}
	data, err := json.MarshalIndent(persistable, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(m.tasksStatePath(), data, 0644)
}

// restorePersistedTasks reloads the serialized task snapshot and refreshes the
// runtime view from persisted pid/output state when possible.
func (m *Manager) restorePersistedTasks() {
	data, err := os.ReadFile(m.tasksStatePath())
	if err != nil {
		return
	}
	var persisted []*Task
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	for _, task := range persisted {
		if task == nil {
			continue
		}
		task.restored = true
		m.refreshRestoredTask(task)
		m.tasks[task.ID] = task
	}
}
