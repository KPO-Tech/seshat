package automation

const (
	ToolNameScheduleJob = "schedule_job"
	ToolNameListJobs    = "list_jobs"
	ToolNameUpdateJob   = "update_job"
	ToolNameDeleteJob   = "delete_job"
	ToolNamePauseJob    = "pause_job"
	ToolNameResumeJob   = "resume_job"
	ToolNameRunJobNow   = "run_job_now"
)

const (
	ToolDescScheduleJob = `
Create a new automation job that runs an agent on a defined schedule.

Use this tool to set up recurring or one-time automated tasks that execute
without user interaction. The job will call a seshat agent with the given
task prompt on the specified schedule.

Trigger types:
- cron: Standard 5-field cron expression (e.g. "0 9 * * 1-5" = weekdays at 9am)
- interval: Repeat every N duration (e.g. "24h", "30m", "2h30m")
- once: Run once at a specific datetime (RFC3339, e.g. "2026-07-01T09:00:00Z")

Required fields: name, trigger_type, task.
`

	ToolDescListJobs = `
List all automation jobs for the current user.

Returns all jobs including their status (active/paused/inactive), trigger
schedule, last run time, next scheduled run, and last run outcome.
Use this to inspect what automations are currently configured.
`

	ToolDescUpdateJob = `
Update an existing automation job's configuration.

Only the fields you provide will be updated; omitted fields remain unchanged.
You can modify the name, description, task prompt, trigger schedule, or
agent configuration. To change the trigger type, provide trigger_type along
with the relevant field (cron, interval, or run_at).
`

	ToolDescDeleteJob = `
Permanently delete an automation job.

This action is irreversible. The job and all its run history will be removed.
Use pause_job if you want to temporarily stop a job without losing its config.
`

	ToolDescPauseJob = `
Pause an active automation job so it stops triggering.

The job and its configuration are preserved; it simply will not run until
resumed. Use resume_job to re-enable it.
`

	ToolDescResumeJob = `
Resume a paused automation job.

Re-activates the job so it will run again on its next scheduled trigger.
NextRunAt is recalculated from the current time when the job is resumed.
`

	ToolDescRunJobNow = `
Immediately trigger a single run of an automation job, regardless of its schedule.

The job must exist and not be in an error state. This creates a one-off run
without affecting the regular schedule. Returns the run ID which can be used
to track the execution.
`
)
