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
	DescriptionEnterPlanMode = "Switch to plan mode, a read-only mode, to design an approach without changing anything. Use it when the user asks for plan mode or asks you to design or investigate without touching files. To put a plan in front of the user for approval you do not need plan mode: call submit_plan directly. A large or unambiguous task is not a reason on its own."
	DescriptionExitPlanMode  = "Exit plan mode once the user has approved the plan, so implementation can begin. Create or refresh the tracked task list with task_create so each implementation step is tracked."
	DescriptionSubmitPlan    = "Put a plan in front of the user for review and approval before you start. Works in any mode: plan mode is not needed to propose a plan. Include context, trade-offs, ordered steps, files touched, and validation approach. The plan is shown as an interactive artifact; wait for the user's answer before acting."
)
