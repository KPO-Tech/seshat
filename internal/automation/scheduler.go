package automation

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ─── Schedule interface ───────────────────────────────────────────────────────

// Schedule computes the next trigger time after a given reference point.
type Schedule interface {
	// Next returns the next time the schedule fires after from.
	// Returns the zero Time if the schedule never fires again.
	Next(from time.Time) time.Time
	// String returns a human-readable description of the schedule.
	String() string
}

// ─── IntervalSchedule ─────────────────────────────────────────────────────────

// IntervalSchedule fires every fixed Interval starting from first use.
type IntervalSchedule struct {
	Interval time.Duration
}

func Every(d time.Duration) *IntervalSchedule { return &IntervalSchedule{Interval: d} }

func (s *IntervalSchedule) Next(from time.Time) time.Time { return from.Add(s.Interval) }
func (s *IntervalSchedule) String() string                { return fmt.Sprintf("every %s", s.Interval) }

// ─── OnceSchedule ─────────────────────────────────────────────────────────────

// OnceSchedule fires exactly once at At.
type OnceSchedule struct {
	At time.Time
}

func Once(at time.Time) *OnceSchedule { return &OnceSchedule{At: at} }

func (s *OnceSchedule) Next(from time.Time) time.Time {
	if s.At.After(from) {
		return s.At
	}
	return time.Time{} // already fired
}

func (s *OnceSchedule) String() string { return fmt.Sprintf("once at %s", s.At.Format(time.RFC3339)) }

// ─── CronSchedule ─────────────────────────────────────────────────────────────

// CronSchedule fires according to a standard 5-field cron expression:
//
//	┌───────────── minute  (0–59)
//	│ ┌─────────── hour    (0–23)
//	│ │ ┌───────── dom     (1–31)
//	│ │ │ ┌─────── month   (1–12)
//	│ │ │ │ ┌───── weekday (0–6, 0=Sun)
//	│ │ │ │ │
//	* * * * *
//
// Supports: *, N, N-M, */N, N,M,…, and combinations thereof. When both dom
// and dow are restricted (neither is *), POSIX cron semantics fire on
// EITHER matching (not both) - e.g. "0 9 15 * 1" fires at 9am on the 15th
// of every month AND every Monday, not only when the 15th is a Monday.
// Parsing/computation is delegated to robfig/cron (MIT), which already
// implements this correctly - a hand-rolled bitmask parser used to live
// here and got the dom/dow OR-semantics wrong (AND instead of OR). Idea to
// use a battle-tested cron library instead of a hand-rolled parser came
// from studying neul-labs/m9m (MIT), which does the same.
type CronSchedule struct {
	expr     string
	schedule cron.Schedule
}

// Cron parses a 5-field cron expression. Returns an error on invalid syntax.
func Cron(expr string) (*CronSchedule, error) {
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, fmt.Errorf("cron: %w", err)
	}
	return &CronSchedule{expr: expr, schedule: sched}, nil
}

// MustCron parses expr and panics on error.
func MustCron(expr string) *CronSchedule {
	c, err := Cron(expr)
	if err != nil {
		panic(err)
	}
	return c
}

func (c *CronSchedule) Next(from time.Time) time.Time { return c.schedule.Next(from) }

func (c *CronSchedule) String() string { return fmt.Sprintf("cron(%s)", c.expr) }

// ─── Scheduler ────────────────────────────────────────────────────────────────

// scheduledJob pairs a Workflow with its Schedule and execution Options.
type scheduledJob struct {
	workflow Workflow
	schedule Schedule
	opts     Options
	nextRun  time.Time
}

// Scheduler runs registered workflows according to their schedules.
// It uses a single goroutine and a timer-based loop; it does not spawn a
// goroutine per job. Concurrent job execution is not supported by design —
// use multiple Schedulers if you need parallel pipelines.
type Scheduler struct {
	mu       sync.Mutex
	jobs     []*scheduledJob
	executor *Executor
}

// NewScheduler creates a Scheduler backed by executor.
func NewScheduler(executor *Executor) *Scheduler {
	return &Scheduler{executor: executor}
}

// Add registers a workflow with its schedule and options.
// Returns the Scheduler to allow method chaining.
func (s *Scheduler) Add(w Workflow, schedule Schedule, opts Options) *Scheduler {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, &scheduledJob{
		workflow: w,
		schedule: schedule,
		opts:     opts,
		nextRun:  schedule.Next(time.Now()),
	})
	return s
}

// Run blocks and runs scheduled jobs until ctx is cancelled.
// It ticks at most once per second to avoid busy-wait.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			s.tick(ctx, now)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	s.mu.Lock()
	due := make([]*scheduledJob, 0)
	for _, job := range s.jobs {
		if !job.nextRun.IsZero() && !now.Before(job.nextRun) {
			due = append(due, job)
		}
	}
	s.mu.Unlock()

	for _, job := range due {
		// Run synchronously — one job at a time.
		s.executor.Run(ctx, job.workflow, job.opts) //nolint:errcheck // the error of one run does not stop the scheduler loop

		s.mu.Lock()
		job.nextRun = job.schedule.Next(now)
		s.mu.Unlock()
	}
}

// RunNow immediately executes the workflow registered under name, bypassing
// the schedule. Returns ErrNotFound if the name is not registered.
func (s *Scheduler) RunNow(ctx context.Context, name string, opts Options) (Result, error) {
	s.mu.Lock()
	var job *scheduledJob
	for _, j := range s.jobs {
		if j.workflow.Name() == name {
			job = j
			break
		}
	}
	s.mu.Unlock()

	if job == nil {
		return Result{}, fmt.Errorf("scheduler: workflow %q not found", name)
	}
	return s.executor.Run(ctx, job.workflow, opts)
}

// Next returns a snapshot of the upcoming scheduled runs, sorted by time.
func (s *Scheduler) Next() []ScheduledRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]ScheduledRun, 0, len(s.jobs))
	for _, j := range s.jobs {
		runs = append(runs, ScheduledRun{
			WorkflowName: j.workflow.Name(),
			Schedule:     j.schedule.String(),
			NextRun:      j.nextRun,
		})
	}
	sort.Slice(runs, func(i, k int) bool {
		return runs[i].NextRun.Before(runs[k].NextRun)
	})
	return runs
}

// ScheduledRun is a read-only view of a single job's next execution.
type ScheduledRun struct {
	WorkflowName string
	Schedule     string
	NextRun      time.Time
}
