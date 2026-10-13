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
	SearchHintExitPlanMode  = "leave plan mode once the user has approved the plan"
	SearchHintSubmitPlan    = "submit the implementation plan for user review and approval"
)

// Descriptions
const (
	DescriptionEnterPlanMode = "Switch to plan mode to design an implementation approach and get the user's alignment before making any changes. Use when a decision belongs to the user and settling it before you touch a file costs less than settling it after: several defensible designs and no indication which one they want, work that is hard to undo or expensive to redo, or acceptance criteria you cannot read off the repository. A large or unambiguous task is not a reason on its own."
	DescriptionExitPlanMode  = "Exit plan mode once the user has approved the plan, so implementation can begin. Create or refresh the tracked task list with task_create so each implementation step is tracked."
	DescriptionSubmitPlan    = "Submit the implementation plan for user review. Include context, trade-offs, ordered steps, files touched, and validation approach. The plan is shown as an interactive artifact; you remain in plan mode until the user approves."
)
