package plan

// Tool name constants
const (
	ToolNameEnterPlanMode = "enter_plan_mode"
	ToolNameExitPlanMode  = "exit_plan_mode"
	ToolNameSubmitPlan    = "submit_plan"
)

// Search hints
const (
	SearchHintEnterPlanMode = "switch to plan mode to design an approach before coding"
	SearchHintExitPlanMode  = "exit plan mode to get user approval for your plan"
	SearchHintSubmitPlan    = "submit the implementation plan for user review and approval"
)

// Descriptions
const (
	DescriptionEnterPlanMode = "Switch to plan mode to analyse the task and design an implementation approach before making any changes. Use when the task is complex, risky, or ambiguous."
	DescriptionExitPlanMode  = "Exit plan mode after the user has approved the plan. Immediately create tasks with task_create to track each implementation step."
	DescriptionSubmitPlan    = "Submit the implementation plan for user review. Include context, trade-offs, ordered steps, files touched, and validation approach. The plan is shown as an interactive artifact; you remain in plan mode until the user approves."
)
