package nodes

import (
	"context"
	"errors"
	"time"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

// ScheduleTrigger is the "schedule_trigger" node type: a graph's time-based
// entry point. It carries schedule config (mode/cronExpr/intervalSeconds/
// runAt) rather than transforming data - a caller that hosts the graph
// (e.g. seshat-server's job scheduler) reads these parameters to decide when
// to run the graph at all, mirroring the same three trigger modes a
// scheduled job already supports. Execute is a pure passthrough: Run already
// seeds every node with no predecessors (this one included) with the run's
// input, so there is no real per-node work to do here - the node exists for
// its config and its IsTrigger marker, not its Execute logic.
type ScheduleTrigger struct{}

// NewScheduleTrigger creates a ScheduleTrigger node.
func NewScheduleTrigger() ScheduleTrigger { return ScheduleTrigger{} }

const (
	ScheduleTriggerModeCron     = "cron"
	ScheduleTriggerModeInterval = "interval"
	ScheduleTriggerModeOnce     = "once"
)

// Description returns the catalog entry of the schedule trigger node: it starts the graph on a schedule, given as a cron expression, a fixed interval or a single future time.
func (ScheduleTrigger) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "schedule_trigger", Name: "Schedule Trigger", Category: "Trigger",
		IsTrigger: true,
		Description: "Starts this graph on a schedule - cron expression, fixed interval, or a single future run. " +
			"Parameters: mode (cron|interval|once, required); cronExpr (string, required if mode=cron); " +
			"intervalSeconds (integer, required if mode=interval); runAt (RFC3339 datetime, required if mode=once).",
		Properties: []dataflow.NodeProperty{
			{Name: "mode", DisplayName: "Mode", Type: dataflow.PropOptions, Required: true, Default: ScheduleTriggerModeCron,
				Options: []dataflow.NodePropertyOption{
					{Label: "Cron", Value: ScheduleTriggerModeCron},
					{Label: "Interval", Value: ScheduleTriggerModeInterval},
					{Label: "Once", Value: ScheduleTriggerModeOnce},
				}},
			{Name: "cronExpr", DisplayName: "Cron expression", Type: dataflow.PropString,
				Placeholder: "0 0 * * *", Description: "5-field cron expression.",
				DisplayIf: &dataflow.DisplayCondition{Field: "mode", Equals: []string{ScheduleTriggerModeCron}}},
			{Name: "intervalSeconds", DisplayName: "Interval (seconds)", Type: dataflow.PropNumber, Default: 3600,
				DisplayIf: &dataflow.DisplayCondition{Field: "mode", Equals: []string{ScheduleTriggerModeInterval}}},
			{Name: "runAt", DisplayName: "Run at", Type: dataflow.PropString,
				Placeholder: "2026-01-01T09:00:00Z", Description: "RFC3339 datetime, once.",
				DisplayIf: &dataflow.DisplayCondition{Field: "mode", Equals: []string{ScheduleTriggerModeOnce}}},
		}}
}

// ValidateParameters deliberately checks presence/shape only, not full cron
// syntax - pkg/automation (which can actually parse a cron expression) isn't
// importable here without an import cycle (pkg/automation pulls in
// internal/automation, whose own test suite imports pkg/dataflow/nodes to
// exercise the dataflow adapter). Real cron validation still happens where
// it already did, server-side, when the job is saved.
func (ScheduleTrigger) ValidateParameters(params map[string]any) error {
	switch dataflow.StringParam(params, "mode", "") {
	case ScheduleTriggerModeCron:
		if dataflow.StringParam(params, "cronExpr", "") == "" {
			return errors.New("cronExpr is required when mode is \"cron\"")
		}
	case ScheduleTriggerModeInterval:
		if dataflow.IntParam(params, "intervalSeconds", 0) <= 0 {
			return errors.New("intervalSeconds must be a positive integer when mode is \"interval\"")
		}
	case ScheduleTriggerModeOnce:
		raw := dataflow.StringParam(params, "runAt", "")
		if raw == "" {
			return errors.New("runAt is required when mode is \"once\"")
		}
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return errors.New("runAt must be an RFC3339 datetime: " + err.Error())
		}
	default:
		return errors.New(`mode must be one of "cron", "interval", "once"`)
	}
	return nil
}

// Execute passes its input on unchanged: the schedule itself is handled by the automation that owns the graph.
func (ScheduleTrigger) Execute(_ context.Context, _ *dataflow.Runtime, input []dataflow.Item, _ map[string]any) (dataflow.Output, error) {
	return dataflow.Main(input), nil
}
