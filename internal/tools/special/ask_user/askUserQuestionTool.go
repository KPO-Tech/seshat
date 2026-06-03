package askuser

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ToolName is the tool name.
const ToolName = "ask_user_question"

// Description is the tool description.
const Description = `Asks the user multiple choice questions to gather information, clarify ambiguity, understand preferences, make decisions or offer them choices.

Use this tool when progress is blocked by missing user input.

This tool is for real clarification, preference gathering, or decision points during execution. It is not for narration, status updates, or asking for approval that another tool already handles.

## Good reasons to use it

- The request is ambiguous and multiple valid implementations exist
- A product or UX preference must be chosen by the user
- You need a concrete decision before continuing
- You need structured answers for recommendations, research direction, or implementation scope
- You need the user to choose between tradeoffs that you cannot resolve from evidence alone

## Do NOT use it when

- you already have enough information to proceed safely
- you are only giving a status update
- you are asking "should I continue?"
- you are in plan mode and simply need plan approval; use ` + "`exit_plan_mode`" + ` for that
- you could instead verify the answer from the repository or a reliable source

## Question design rules

- Ask the minimum number of questions needed to unblock progress
- Prefer one focused question over a large survey unless batching is clearly useful
- Keep each question concise and concrete
- Make sure each option represents a real decision the user can understand
- Use 2 to 10 options
- Use ` + "`multiSelect: true`" + ` only when multiple answers are genuinely useful
- Put the recommended option first and suffix the label with ` + "\"(Recommended)\"" + `
- Do NOT include an explicit ` + "\"Other\"" + ` option; the UI/runtime adds a free-text path automatically
- If a free-text answer is likely, keep the structured choices broad and complementary rather than overly exhaustive

## Examples

Good:
- "Which deployment target should I optimize for first?"
- "Which of these auth approaches matches your preference?"
- "What kind of project should I research for you?"
- "Which of these rollout strategies do you prefer for this change?"

Bad:
- "Should I proceed?"
- "Is my plan good?"
- "Anything else?"
- "Can you confirm this fact?" when the fact could be verified from tools

## Plan mode note

In plan mode, use this tool only to resolve requirement gaps before finalizing the plan. Do not ask the user to approve the plan through this tool.`

// SearchHint is a hint for tool search functionality.
const SearchHint = "prompt the user with a multiple choice question"

// PreviewFeatureDescriptionMarkdown is the preview feature guidance for markdown format.
const PreviewFeatureDescriptionMarkdown = `
Preview feature:
Use the optional ` + "`preview`" + ` field on options when presenting concrete artifacts that users need to visually compare:
- ASCII mockups of UI layouts or components
- Code snippets showing different implementations
- Diagram variations
- Configuration examples

Preview content is rendered as markdown in a monospace box. Multi-line text with newlines is supported. When any option has a preview, the UI switches to a side-by-side layout with a vertical option list on the left and preview on the right. Do not use previews for simple preference questions where labels and descriptions suffice. Note: previews are only supported for single-select questions (not multiSelect).
`

// PreviewFeatureDescriptionHTML is the preview feature guidance for HTML format.
const PreviewFeatureDescriptionHTML = `
Preview feature:
Use the optional ` + "`preview`" + ` field on options when presenting concrete artifacts that users need to visually compare:
- HTML mockups of UI layouts or components
- Formatted code snippets showing different implementations
- Visual comparisons or diagrams

Preview content must be a self-contained HTML fragment (no <html>/<body> wrapper, no <script> or <style> tags — use inline style attributes instead). Do not use previews for simple preference questions where labels and descriptions suffice. Note: previews are only supported for single-select questions (not multiSelect).
`

// GetDescription returns the full description, optionally including preview feature guidance.
func GetDescription(includePreviewMarkdown bool, includePreviewHTML bool) string {
	description := Description
	if includePreviewMarkdown {
		description += PreviewFeatureDescriptionMarkdown
	}
	if includePreviewHTML {
		description += PreviewFeatureDescriptionHTML
	}
	return description
}

// ValidateHTMLPreview validates HTML preview content.
// Returns an error message if invalid, nil if valid.
func ValidateHTMLPreview(preview string) string {
	if preview == "" {
		return ""
	}

	// Check for full document tags
	if hasFullDocumentTags(preview) {
		return "preview must be an HTML fragment, not a full document (no <html>, <body>, or <!DOCTYPE>)"
	}

	// Check for script/style tags
	if hasExecutableTags(preview) {
		return "preview must not contain <script> or <style> tags. Use inline styles via the style attribute if needed."
	}

	// Check that it contains HTML tags
	if !hasHTMLTags(preview) {
		return "preview must contain HTML. Wrap content in a tag like <div> or <pre>."
	}

	return ""
}

func hasFullDocumentTags(s string) bool {
	lower := toLowerASCII(s)
	return contains(lower, "<html") || contains(lower, "<body") || contains(lower, "<!doctype")
}

func hasExecutableTags(s string) bool {
	lower := toLowerASCII(s)
	return contains(lower, "<script") || contains(lower, "<style")
}

func hasHTMLTags(s string) bool {
	// Simple check for any HTML-like tag
	for i := 0; i < len(s); i++ {
		if s[i] == '<' && i+1 < len(s) {
			// Check if it's a letter (start of a tag)
			if (s[i+1] >= 'a' && s[i+1] <= 'z') || (s[i+1] >= 'A' && s[i+1] <= 'Z') {
				return true
			}
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLowerASCII(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		result[i] = c
	}
	return string(result)
}

// Tool represents the AskUserQuestion tool.
type Tool struct {
	reader   *bufio.Reader
	timeout  time.Duration
	promptFn types.PromptFn
}

// Config represents tool configuration.
type Config struct {
	InputReader *bufio.Reader
	Timeout     time.Duration
	PromptFn    types.PromptFn
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		InputReader: bufio.NewReader(os.Stdin),
		Timeout:     10 * time.Minute,
	}
}

// NewTool creates a new AskUserQuestion tool.
func NewTool(config *Config) *Tool {
	if config == nil {
		config = DefaultConfig()
	}

	return &Tool{
		reader:   config.InputReader,
		timeout:  config.Timeout,
		promptFn: config.PromptFn,
	}
}

// Definition returns the tool definition.
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "AskUserQuestion",
		SearchHint:  SearchHint,
		Description: GetDescription(false, false),
		Category:    "interaction",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questions": map[string]any{
					"type":        "array",
					"description": "Questions to ask the user (1-4 questions, each with 2-10 options)",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"question": map[string]any{
								"type":        "string",
								"description": "The complete question to ask the user",
							},
							"header": map[string]any{
								"type":        "string",
								"description": "Short label displayed as a chip/tag",
							},
							"options": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"label": map[string]any{
											"type":        "string",
											"description": "Display text for the option",
										},
										"description": map[string]any{
											"type":        "string",
											"description": "Explanation of the option",
										},
										"preview": map[string]any{
											"type":        "string",
											"description": "Optional preview content for the option",
										},
									},
								},
							},
							"multiSelect": map[string]any{
								"type":        "boolean",
								"description": "Allow multiple selections",
							},
						},
					},
				},
				"answers": map[string]any{
					"type":        "object",
					"description": "Optional answers already collected for the questions",
				},
				"annotations": map[string]any{
					"type":        "object",
					"description": "Optional annotations about selected previews or notes",
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Optional metadata for analytics or source tracking",
				},
			},
			"required": []string{"questions"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

// Call executes the tool.
func (t *Tool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	_ = permissionCheck

	parsedInput, err := parseInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("invalid input: %w", err)), nil
	}

	if err := parsedInput.Validate(); err != nil {
		return tool.NewErrorResult(fmt.Errorf("invalid input: %w", err)), nil
	}

	answers := cloneStringMap(parsedInput.Answers)
	annotations := cloneAnnotationMap(parsedInput.Annotations)
	toolCtx := input.ToolContextValue()

	if len(answers) == 0 {
		answers, annotations, err = t.askQuestions(ctx, parsedInput, toolCtx.ToolUseID)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to get user answers: %w", err)), nil
		}
	}

	output := Output{
		Questions:   parsedInput.Questions,
		Answers:     answers,
		Annotations: annotations,
	}

	result := tool.NewJSONResult(output)
	result.Content = t.formatOutput(output)
	result.Metadata = &tool.ResultMetadata{
		Additional: map[string]any{
			"question_count": len(output.Questions),
			"answer_count":   len(output.Answers),
		},
	}

	return result, nil
}

// Description returns human-readable description.
func (t *Tool) Description(ctx context.Context) (string, error) {
	return Description, nil
}

// ValidateInput validates AskUserQuestion input.
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	_ = ctx
	parsedInput, err := parseInput(input)
	if err != nil {
		return nil, err
	}
	if err := parsedInput.Validate(); err != nil {
		return nil, err
	}

	// Validate HTML previews if present
	for _, q := range parsedInput.Questions {
		for _, opt := range q.Options {
			if opt.Preview != "" {
				if err := ValidateHTMLPreview(opt.Preview); err != "" {
					return nil, fmt.Errorf("option %q in question %q: %s", opt.Label, q.Question, err)
				}
			}
		}
	}

	return input, nil
}

// CheckPermissions marks AskUserQuestion as locally allowed.
func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	_ = ctx
	_ = input
	_ = toolCtx
	return types.AllowWithUpdatedInput(input)
}

// IsConcurrencySafe reports that AskUserQuestion can run concurrently.
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	_ = input
	return true
}

// IsReadOnly reports that AskUserQuestion does not modify state.
func (t *Tool) IsReadOnly(input map[string]any) bool {
	_ = input
	return true
}

// IsEnabled returns whether this tool is currently active.
func (t *Tool) IsEnabled() bool { return true }

// FormatResult serialises the tool output into the tool_result content string.
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput enriches a shallow clone of the parsed input with derived fields.
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// SetReader sets a custom input reader (for testing).
func (t *Tool) SetReader(reader *bufio.Reader) {
	t.reader = reader
}

// SetTimeout sets the input timeout.
func (t *Tool) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}

// SetPromptFn sets a prompt function for structured prompting.
func (t *Tool) SetPromptFn(promptFn types.PromptFn) {
	t.promptFn = promptFn
}

// NewInput creates a new Input from a question.
func NewInput(question, header string, options ...string) *Input {
	opts := make([]QuestionOption, len(options))
	for i, o := range options {
		opts[i] = QuestionOption{Label: o}
	}

	return &Input{
		Questions: []Question{{
			Question: question,
			Header:   header,
			Options:  opts,
		}},
	}
}

func parseInput(parsed map[string]any) (*Input, error) {
	rawQuestions, ok := parsed["questions"].([]any)
	if !ok {
		return nil, fmt.Errorf("questions is required and must be an array")
	}

	questions := make([]Question, 0, len(rawQuestions))
	for _, rawQuestion := range rawQuestions {
		qMap, ok := rawQuestion.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("question entry must be an object")
		}

		question := Question{}
		if value, ok := qMap["question"].(string); ok {
			question.Question = value
		}
		if value, ok := qMap["header"].(string); ok {
			question.Header = value
		}
		if value, ok := qMap["multiSelect"].(bool); ok {
			question.MultiSelect = value
		}

		rawOptions, ok := qMap["options"].([]any)
		if !ok {
			return nil, fmt.Errorf("question options must be an array")
		}
		for _, rawOption := range rawOptions {
			oMap, ok := rawOption.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("question option must be an object")
			}
			option := QuestionOption{}
			if value, ok := oMap["label"].(string); ok {
				option.Label = value
			}
			if value, ok := oMap["description"].(string); ok {
				option.Description = value
			}
			if value, ok := oMap["preview"].(string); ok {
				option.Preview = value
			}
			question.Options = append(question.Options, option)
		}

		questions = append(questions, question)
	}

	answers := map[string]string{}
	if rawAnswers, ok := parsed["answers"].(map[string]any); ok {
		for question, rawAnswer := range rawAnswers {
			answers[question] = fmt.Sprintf("%v", rawAnswer)
		}
	}

	annotations := map[string]Annotation{}
	if rawAnnotations, ok := parsed["annotations"].(map[string]any); ok {
		for question, rawAnnotation := range rawAnnotations {
			annotationMap, ok := rawAnnotation.(map[string]any)
			if !ok {
				continue
			}
			annotation := Annotation{}
			if preview, ok := annotationMap["preview"].(string); ok {
				annotation.Preview = preview
			}
			if notes, ok := annotationMap["notes"].(string); ok {
				annotation.Notes = notes
			}
			annotations[question] = annotation
		}
	}

	return &Input{
		Questions:   questions,
		Answers:     answers,
		Annotations: annotations,
	}, nil
}

func (t *Tool) askQuestions(ctx context.Context, input *Input, toolUseID string) (map[string]string, map[string]Annotation, error) {
	answers := make(map[string]string, len(input.Questions))
	annotations := make(map[string]Annotation)

	for _, question := range input.Questions {
		answer, annotation, err := t.askSingleQuestion(ctx, question, toolUseID)
		if err != nil {
			return nil, nil, err
		}
		answers[question.Question] = answer
		if annotation.Preview != "" || annotation.Notes != "" {
			annotations[question.Question] = annotation
		}
	}

	return answers, annotations, nil
}

func (t *Tool) askSingleQuestion(ctx context.Context, question Question, toolUseID string) (string, Annotation, error) {
	if t.promptFn != nil {
		return t.askWithPromptFn(ctx, question, toolUseID)
	}
	return t.askWithReader(ctx, question)
}

func (t *Tool) askWithPromptFn(ctx context.Context, question Question, toolUseID string) (string, Annotation, error) {
	options := make([]types.PromptOption, 0, len(question.Options)+1)
	for _, option := range question.Options {
		options = append(options, types.PromptOption{
			Label:       option.Label,
			Value:       option.Label,
			Description: option.Description,
		})
	}
	options = append(options, types.PromptOption{Label: "Other", Value: "__other__", Description: "Provide custom input"})

	response, err := t.promptFn(ctx, types.PromptRequest{
		Type:    types.PromptTypeChoice,
		Message: question.Question,
		Options: options,
		Metadata: map[string]any{
			"header":      question.Header,
			"multiSelect": question.MultiSelect,
			"tool_name":   ToolName,
			"tool_use_id": toolUseID,
		},
	})
	if err != nil {
		return "", Annotation{}, err
	}
	if response.Cancelled {
		return "", Annotation{}, fmt.Errorf("user cancelled")
	}

	if question.MultiSelect {
		selections, custom, err := normalizeMultiPromptResponse(response.Value)
		if err != nil {
			return "", Annotation{}, err
		}
		if custom {
			customText, err := t.askCustomText(ctx, question, toolUseID)
			return customText, Annotation{}, err
		}

		labels := make([]string, 0, len(selections))
		annotation := Annotation{}
		for _, selection := range selections {
			if option := question.GetOptionByLabel(selection); option != nil {
				labels = append(labels, option.Label)
				if option.Preview != "" && annotation.Preview == "" {
					annotation.Preview = option.Preview
				}
			} else {
				labels = append(labels, selection)
			}
		}
		sort.Strings(labels)
		return strings.Join(labels, ", "), annotation, nil
	}

	selection := fmt.Sprintf("%v", response.Value)
	if selection == "__other__" {
		customText, err := t.askCustomText(ctx, question, toolUseID)
		return customText, Annotation{}, err
	}

	annotation := Annotation{}
	if option := question.GetOptionByLabel(selection); option != nil && option.Preview != "" {
		annotation.Preview = option.Preview
	}
	return selection, annotation, nil
}

func (t *Tool) askCustomText(ctx context.Context, question Question, toolUseID string) (string, error) {
	if t.promptFn == nil {
		return "", fmt.Errorf("no prompt function available for custom text input")
	}

	response, err := t.promptFn(ctx, types.PromptRequest{
		Type:    types.PromptTypeText,
		Message: fmt.Sprintf("%s (custom answer)", question.Question),
		Metadata: map[string]any{
			"header":      question.Header,
			"tool_name":   ToolName,
			"tool_use_id": toolUseID,
		},
	})
	if err != nil {
		return "", err
	}
	if response.Cancelled {
		return "", fmt.Errorf("user cancelled")
	}
	return strings.TrimSpace(fmt.Sprintf("%v", response.Value)), nil
}

func (t *Tool) askWithReader(ctx context.Context, question Question) (string, Annotation, error) {
	if t.reader == nil {
		return "", Annotation{}, fmt.Errorf("no input reader configured")
	}

	fmt.Printf("\n[%s] %s\n", question.Header, question.Question)
	fmt.Println(strings.Repeat("-", 60))
	for idx, opt := range question.Options {
		fmt.Printf("  %d) %s", idx+1, opt.Label)
		if opt.Description != "" {
			fmt.Printf("\n      %s", opt.Description)
		}
		fmt.Println()
	}
	fmt.Println("  0) Other (custom input)")
	if question.MultiSelect {
		fmt.Println("Select multiple choices with comma-separated numbers.")
	}
	fmt.Println()

	answer, err := t.readLineWithTimeout(ctx, "Your choice: ")
	if err != nil {
		return "", Annotation{}, err
	}

	return t.interpretReaderAnswer(ctx, question, answer)
}

func (t *Tool) interpretReaderAnswer(ctx context.Context, question Question, answer string) (string, Annotation, error) {
	trimmed := strings.TrimSpace(answer)
	annotation := Annotation{}

	if question.MultiSelect {
		parts := splitCSV(trimmed)
		if len(parts) == 0 {
			return "", Annotation{}, fmt.Errorf("at least one selection is required")
		}

		labels := make([]string, 0, len(parts))
		for _, part := range parts {
			if part == "0" || strings.EqualFold(part, "other") {
				custom, err := t.readLineWithTimeout(ctx, "Custom answer: ")
				return strings.TrimSpace(custom), Annotation{}, err
			}
			index := parseChoiceIndex(part)
			if index < 1 || index > len(question.Options) {
				return "", Annotation{}, fmt.Errorf("invalid selection: %s", part)
			}
			option := question.Options[index-1]
			labels = append(labels, option.Label)
			if option.Preview != "" && annotation.Preview == "" {
				annotation.Preview = option.Preview
			}
		}
		sort.Strings(labels)
		return strings.Join(labels, ", "), annotation, nil
	}

	if trimmed == "0" || strings.EqualFold(trimmed, "other") {
		custom, err := t.readLineWithTimeout(ctx, "Custom answer: ")
		return strings.TrimSpace(custom), Annotation{}, err
	}

	index := parseChoiceIndex(trimmed)
	if index < 1 || index > len(question.Options) {
		return "", Annotation{}, fmt.Errorf("invalid selection: %s", trimmed)
	}
	selected := question.Options[index-1]
	if selected.Preview != "" {
		annotation.Preview = selected.Preview
	}
	return selected.Label, annotation, nil
}

func (t *Tool) readLineWithTimeout(ctx context.Context, prompt string) (string, error) {
	fmt.Print(prompt)
	done := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := t.reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			errCh <- err
			return
		}
		done <- strings.TrimSpace(line)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(t.timeout):
		return "", fmt.Errorf("timeout waiting for user input")
	case err := <-errCh:
		return "", err
	case line := <-done:
		return line, nil
	}
}

func (t *Tool) formatOutput(output Output) string {
	var builder strings.Builder
	builder.WriteString("User answered your questions:\n\n")

	questionOrder := make([]string, 0, len(output.Questions))
	for _, question := range output.Questions {
		questionOrder = append(questionOrder, question.Question)
	}

	for _, questionText := range questionOrder {
		answer := output.Answers[questionText]
		builder.WriteString(fmt.Sprintf("Q: %s\n", questionText))
		builder.WriteString(fmt.Sprintf("A: %s\n", answer))
		if annotation, ok := output.Annotations[questionText]; ok {
			if annotation.Preview != "" {
				builder.WriteString(fmt.Sprintf("Preview: %s\n", annotation.Preview))
			}
			if annotation.Notes != "" {
				builder.WriteString(fmt.Sprintf("Notes: %s\n", annotation.Notes))
			}
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

func normalizeMultiPromptResponse(value any) ([]string, bool, error) {
	switch v := value.(type) {
	case []string:
		return v, containsOther(v), nil
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, fmt.Sprintf("%v", item))
		}
		return items, containsOther(items), nil
	case string:
		parts := splitCSV(v)
		if len(parts) == 0 {
			return nil, false, fmt.Errorf("no answer provided")
		}
		return parts, containsOther(parts), nil
	default:
		return nil, false, fmt.Errorf("unsupported multi-select response type: %T", value)
	}
}

func containsOther(values []string) bool {
	for _, value := range values {
		if value == "__other__" || strings.EqualFold(value, "other") {
			return true
		}
	}
	return false
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseChoiceIndex(value string) int {
	var index int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &index)
	return index
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneAnnotationMap(values map[string]Annotation) map[string]Annotation {
	if len(values) == 0 {
		return map[string]Annotation{}
	}
	cloned := make(map[string]Annotation, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
