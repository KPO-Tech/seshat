package auto

import (
	"context"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

// ─── BuildTranscriptFromMessages ─────────────────────────────────────────────

func TestBuildTranscriptFromMessages_Nil(t *testing.T) {
	result := BuildTranscriptFromMessages(nil)
	assert.Nil(t, result)
}

func TestBuildTranscriptFromMessages_EmptySlice(t *testing.T) {
	result := BuildTranscriptFromMessages([]types.Message{})
	assert.Empty(t, result)
}

func TestBuildTranscriptFromMessages_UserTextMessage(t *testing.T) {
	messages := []types.Message{
		{
			ID:      "m1",
			Role:    types.RoleUser,
			Content: []types.ContentBlock{types.TextContent{Text: "hello"}},
		},
	}
	result := BuildTranscriptFromMessages(messages)
	require.Len(t, result, 1)
	assert.Equal(t, "user", result[0].Role)
	require.Len(t, result[0].Content, 1)
	assert.Equal(t, "text", result[0].Content[0].Type)
	assert.Equal(t, "hello", result[0].Content[0].Text)
}

func TestBuildTranscriptFromMessages_AssistantToolUse(t *testing.T) {
	messages := []types.Message{
		{
			ID:   "m1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseContent{
					ID:    "tu1",
					Name:  "bash",
					Input: map[string]any{"command": "ls"},
				},
			},
		},
	}
	result := BuildTranscriptFromMessages(messages)
	require.Len(t, result, 1)
	assert.Equal(t, "assistant", result[0].Role)
	require.Len(t, result[0].Content, 1)
	assert.Equal(t, "tool_use", result[0].Content[0].Type)
	assert.Equal(t, "bash", result[0].Content[0].Name)
}

func TestBuildTranscriptFromMessages_SkipsEmptyContent(t *testing.T) {
	messages := []types.Message{
		{ID: "m1", Role: types.RoleUser, Content: nil},
		{
			ID:      "m2",
			Role:    types.RoleUser,
			Content: []types.ContentBlock{types.TextContent{Text: "ok"}},
		},
	}
	// m1 has no content blocks — should be skipped
	result := BuildTranscriptFromMessages(messages)
	require.Len(t, result, 1)
	assert.Equal(t, "ok", result[0].Content[0].Text)
}

func TestBuildTranscriptFromMessages_MixedTurns(t *testing.T) {
	messages := []types.Message{
		{ID: "m1", Role: types.RoleUser, Content: []types.ContentBlock{types.TextContent{Text: "run ls"}}},
		{ID: "m2", Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseContent{ID: "tu1", Name: "bash", Input: map[string]any{"command": "ls"}},
		}},
	}
	result := BuildTranscriptFromMessages(messages)
	require.Len(t, result, 2)
	assert.Equal(t, "user", result[0].Role)
	assert.Equal(t, "assistant", result[1].Role)
}

// ─── SerializeTranscript ─────────────────────────────────────────────────────

func TestSerializeTranscript_Empty(t *testing.T) {
	result := SerializeTranscript(nil)
	assert.Empty(t, result)
}

func TestSerializeTranscript_UserTextBlock(t *testing.T) {
	transcript := []TranscriptEntry{
		{Role: "user", Content: []TranscriptBlock{{Type: "text", Text: "hello world"}}},
	}
	result := SerializeTranscript(transcript)
	require.Len(t, result, 1)
	assert.Contains(t, result[0], "User: hello world")
}

func TestSerializeTranscript_AssistantToolUseBlock(t *testing.T) {
	transcript := []TranscriptEntry{
		{Role: "assistant", Content: []TranscriptBlock{
			{Type: "tool_use", Name: "bash", Input: map[string]any{"command": "ls"}},
		}},
	}
	result := SerializeTranscript(transcript)
	require.Len(t, result, 1)
	assert.Contains(t, result[0], "bash(")
}

func TestSerializeTranscript_AssistantTextBlockSkipped(t *testing.T) {
	// Assistant text blocks (not tool_use) are not serialized
	transcript := []TranscriptEntry{
		{Role: "assistant", Content: []TranscriptBlock{{Type: "text", Text: "thinking..."}}},
	}
	result := SerializeTranscript(transcript)
	assert.Empty(t, result)
}

// ─── TruncateString ──────────────────────────────────────────────────────────

func TestTruncateString_ShortPassesThrough(t *testing.T) {
	s := "hello"
	assert.Equal(t, s, TruncateString(s, 100))
}

func TestTruncateString_ExactLengthPassesThrough(t *testing.T) {
	s := "hello"
	assert.Equal(t, s, TruncateString(s, 5))
}

func TestTruncateString_LongGetsTruncated(t *testing.T) {
	s := strings.Repeat("a", 200)
	result := TruncateString(s, 100)
	assert.Len(t, result, 100+len("... [truncated]"))
	assert.True(t, strings.HasSuffix(result, "... [truncated]"))
}

// ─── WrapWithTranscriptTags ──────────────────────────────────────────────────

func TestWrapWithTranscriptTags_WrapsCorrectly(t *testing.T) {
	blocks := []string{"block1", "block2"}
	wrapped := WrapWithTranscriptTags(blocks)

	require.GreaterOrEqual(t, len(wrapped), 4) // open + 2 blocks + close
	assert.Equal(t, "<transcript>\n", wrapped[0])
	assert.Equal(t, "</transcript>\n", wrapped[len(wrapped)-1])
	assert.Contains(t, wrapped, "block1")
	assert.Contains(t, wrapped, "block2")
}

func TestWrapWithTranscriptTags_EmptyBlocks(t *testing.T) {
	wrapped := WrapWithTranscriptTags(nil)
	assert.Equal(t, "<transcript>\n", wrapped[0])
	assert.Equal(t, "</transcript>\n", wrapped[len(wrapped)-1])
}

// ─── CombineUsage ────────────────────────────────────────────────────────────

func TestCombineUsage_BothNil(t *testing.T) {
	result := CombineUsage(nil, nil)
	assert.Nil(t, result)
}

func TestCombineUsage_FirstNil(t *testing.T) {
	b := &ClassifierUsage{InputTokens: 10, OutputTokens: 5}
	result := CombineUsage(nil, b)
	assert.Equal(t, b, result)
}

func TestCombineUsage_SecondNil(t *testing.T) {
	a := &ClassifierUsage{InputTokens: 10, OutputTokens: 5}
	result := CombineUsage(a, nil)
	assert.Equal(t, a, result)
}

func TestCombineUsage_Sums(t *testing.T) {
	a := &ClassifierUsage{InputTokens: 100, OutputTokens: 20, CacheReadInputTokens: 5, CacheCreationInputTokens: 3}
	b := &ClassifierUsage{InputTokens: 200, OutputTokens: 40, CacheReadInputTokens: 10, CacheCreationInputTokens: 7}
	result := CombineUsage(a, b)
	require.NotNil(t, result)
	assert.Equal(t, 300, result.InputTokens)
	assert.Equal(t, 60, result.OutputTokens)
	assert.Equal(t, 15, result.CacheReadInputTokens)
	assert.Equal(t, 10, result.CacheCreationInputTokens)
}

// ─── BuildSystemPrompt ───────────────────────────────────────────────────────

func TestBuildSystemPrompt_NotEmpty(t *testing.T) {
	prompt := BuildSystemPrompt()
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "security classifier")
}

func TestBuildSystemPromptWithConfig_UserAllowRules(t *testing.T) {
	config := &TemplateConfig{
		UseExternalTemplate: true,
		AllowRules:          []string{"Allow running make test", "Allow reading logs"},
	}
	prompt := BuildSystemPromptWithConfig(config)
	assert.Contains(t, prompt, "Allow running make test")
	assert.Contains(t, prompt, "Allow reading logs")
}

func TestBuildSystemPromptWithConfig_UserDenyRules(t *testing.T) {
	config := &TemplateConfig{
		UseExternalTemplate: true,
		DenyRules:           []string{"Deny rm commands"},
	}
	prompt := BuildSystemPromptWithConfig(config)
	assert.Contains(t, prompt, "Deny rm commands")
}

func TestBuildSystemPromptWithConfig_AnthropicTemplate(t *testing.T) {
	config := &TemplateConfig{UseAnthropicTemplate: true}
	prompt := BuildSystemPromptWithConfig(config)
	assert.Contains(t, prompt, "Internal Build")
}

// ─── ReplaceOutputFormatWithXml ──────────────────────────────────────────────

func TestReplaceOutputFormatWithXml_ReplacesLine(t *testing.T) {
	input := "Some intro.\nUse the classify_result tool to report your classification.\nSome footer."
	result := ReplaceOutputFormatWithXml(input)
	assert.NotContains(t, result, "classify_result tool")
	assert.Contains(t, result, "<block>yes</block>")
	assert.Contains(t, result, "<block>no</block>")
}

func TestReplaceOutputFormatWithXml_NoopWhenMissing(t *testing.T) {
	input := "No classify_result line here."
	result := ReplaceOutputFormatWithXml(input)
	assert.Equal(t, input, result)
}

// ─── randomString (classifier_api.go) ────────────────────────────────────────

func TestRandomStringIsNotDeterministic(t *testing.T) {
	// Generate multiple IDs — they should not all be identical.
	seen := make(map[string]struct{})
	for i := 0; i < 20; i++ {
		seen[randomString(8)] = struct{}{}
	}
	// With 36^8 combinations, 20 calls should virtually always produce at least 2 distinct values.
	assert.Greater(t, len(seen), 1, "randomString should produce different values across calls")
}

func TestRandomStringLength(t *testing.T) {
	for _, n := range []int{4, 8, 16, 32} {
		s := randomString(n)
		assert.Len(t, s, n, "randomString(%d) should return string of length %d", n, n)
	}
}

// mockClassifier implements Classifier for testing
type mockClassifier struct {
	allowed    bool
	confidence float64
	err        error
}

func (m *mockClassifier) Classify(ctx context.Context, toolName string, input map[string]any) (Classification, error) {
	return Classification{
		Allowed:    m.allowed,
		Confidence: m.confidence,
		Reason:     "mock classifier result",
	}, m.err
}

func TestMode_Classify_PowerShell_Headless(t *testing.T) {
	mode := NewMode(nil, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:                     "powershell",
		ToolInput:                    map[string]any{"command": "ls"},
		ShouldAvoidPermissionPrompts: true, // headless mode
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny in headless mode, got %v", result.Behavior)
	}
	if result.DecisionReason == nil || result.DecisionReason.Source != "powershell" {
		t.Errorf("expected powershell decision reason")
	}
}

func TestMode_Classify_PowerShell_Interactive(t *testing.T) {
	mode := NewMode(nil, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:                     "powershell",
		ToolInput:                    map[string]any{"command": "ls"},
		ShouldAvoidPermissionPrompts: false, // interactive mode
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorAsk {
		t.Errorf("expected Ask in interactive mode, got %v", result.Behavior)
	}
}

func TestMode_Classify_SafeTool_Allowed(t *testing.T) {
	mode := NewMode(nil, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "read_file",
		ToolInput: map[string]any{"file_path": "/etc/passwd"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorAllow {
		t.Errorf("expected Allow for safe tool, got %v", result.Behavior)
	}
	if result.DecisionReason == nil || result.DecisionReason.Reason != "tool is on the safe allowlist" {
		t.Errorf("expected safe allowlist reason, got %v", result.DecisionReason)
	}
}

func TestMode_Classify_ClassifierAllowed(t *testing.T) {
	classifier := &mockClassifier{allowed: true, confidence: 0.9}
	mode := NewMode(classifier, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "echo hello"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorAllow {
		t.Errorf("expected Allow when classifier allows, got %v", result.Behavior)
	}
	if result.Confidence == nil || *result.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %v", result.Confidence)
	}
}

func TestMode_Classify_ClassifierDenied(t *testing.T) {
	classifier := &mockClassifier{allowed: false, confidence: 0.8}
	mode := NewMode(classifier, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "rm -rf /"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny when classifier denies, got %v", result.Behavior)
	}
	if result.DenialTracking == nil {
		t.Errorf("expected DenialTracking to be set")
	}
}

func TestMode_Classify_ClassifierError_FailClosed(t *testing.T) {
	classifier := &mockClassifier{err: context.DeadlineExceeded}
	mode := NewMode(classifier, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "ls"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny on classifier error in fail-closed mode, got %v", result.Behavior)
	}
}

func TestMode_Classify_ClassifierError_FailOpen(t *testing.T) {
	classifier := &mockClassifier{err: context.DeadlineExceeded}
	mode := NewMode(classifier, &ModeConfig{FailClosed: false})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "ls"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorAsk {
		t.Errorf("expected Ask on classifier error in fail-open mode, got %v", result.Behavior)
	}
}

func TestMode_Classify_NoClassifier_FailClosed(t *testing.T) {
	mode := NewMode(nil, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "ls"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny when no classifier in fail-closed mode, got %v", result.Behavior)
	}
}

func TestMode_Classify_NoClassifier_FailOpen(t *testing.T) {
	mode := NewMode(nil, &ModeConfig{FailClosed: false})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "ls"},
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorAsk {
		t.Errorf("expected Ask when no classifier in fail-open mode, got %v", result.Behavior)
	}
}

func TestMode_Classify_DenialLimitExceeded_Interactive(t *testing.T) {
	classifier := &mockClassifier{allowed: false, confidence: 0.9}
	mode := NewMode(classifier, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:                     "bash",
		ToolInput:                    map[string]any{"command": "rm -rf /"},
		ShouldAvoidPermissionPrompts: false,
	}

	// Simulate 3 consecutive denials
	pctx.DenialTracking = &types.DenialTrackingState{}
	pctx.DenialTracking.RecordDenial()
	pctx.DenialTracking.RecordDenial()
	pctx.DenialTracking.RecordDenial()

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After 3 denials, should fallback to Ask
	if result.Behavior != types.PermissionBehaviorAsk {
		t.Errorf("expected Ask after denial limit exceeded, got %v", result.Behavior)
	}
	if result.DecisionReason == nil || result.DecisionReason.Source != "denialTracking" {
		t.Errorf("expected denialTracking source, got %v", result.DecisionReason)
	}
}

func TestMode_Classify_DenialLimitExceeded_Headless(t *testing.T) {
	classifier := &mockClassifier{allowed: false, confidence: 0.9}
	mode := NewMode(classifier, &ModeConfig{FailClosed: true})

	ctx := context.Background()
	pctx := &ClassifierContext{
		ToolName:                     "bash",
		ToolInput:                    map[string]any{"command": "rm -rf /"},
		ShouldAvoidPermissionPrompts: true, // headless
	}

	// Simulate 3 consecutive denials
	pctx.DenialTracking = &types.DenialTrackingState{}
	pctx.DenialTracking.RecordDenial()
	pctx.DenialTracking.RecordDenial()
	pctx.DenialTracking.RecordDenial()

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In headless mode with limit exceeded, should Deny (abort)
	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny in headless mode with limit exceeded, got %v", result.Behavior)
	}
}

func TestMode_Classify_SuccessResetsDenialTracking(t *testing.T) {
	classifier := &mockClassifier{allowed: true, confidence: 0.9}
	mode := NewMode(classifier, &ModeConfig{FailClosed: true})

	ctx := context.Background()

	// Start with some denials
	pctx := &ClassifierContext{
		ToolName:  "bash",
		ToolInput: map[string]any{"command": "echo hello"},
	}
	pctx.DenialTracking = &types.DenialTrackingState{}
	pctx.DenialTracking.RecordDenial()
	pctx.DenialTracking.RecordDenial()

	initialDenials := pctx.DenialTracking.GetConsecutiveDenials()
	if initialDenials != 2 {
		t.Errorf("expected 2 initial denials, got %d", initialDenials)
	}

	result, err := mode.Classify(ctx, pctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Behavior != types.PermissionBehaviorAllow {
		t.Errorf("expected Allow, got %v", result.Behavior)
	}

	// After success, consecutive denials should be reset to 0
	if pctx.DenialTracking.GetConsecutiveDenials() != 0 {
		t.Errorf("expected consecutive denials to be reset to 0, got %d", pctx.DenialTracking.GetConsecutiveDenials())
	}
}

func TestDenialLimitConfig_ShouldFallback(t *testing.T) {
	config := DefaultDenialLimitConfig()

	// Test consecutive denials
	state := &types.DenialTrackingState{}
	state.RecordDenial()
	state.RecordDenial()
	state.RecordDenial()

	if !config.ShouldFallback(state) {
		t.Errorf("expected ShouldFallback to return true after 3 consecutive denials")
	}

	// Test total denials
	state2 := &types.DenialTrackingState{}
	for i := 0; i < 10; i++ {
		state2.RecordDenial()
	}

	if !config.ShouldFallback(state2) {
		t.Errorf("expected ShouldFallback to return true after 10 total denials")
	}

	// Test nil state
	if config.ShouldFallback(nil) {
		t.Errorf("expected ShouldFallback to return false for nil state")
	}
}

func TestHandleDenialLimitExceeded_NilState(t *testing.T) {
	result := HandleDenialLimitExceeded(nil, nil, false, "reason")
	if result != nil {
		t.Errorf("expected nil result for nil state")
	}
}

func TestHandleDenialLimitExceeded_Interactive(t *testing.T) {
	config := DefaultDenialLimitConfig()
	state := &types.DenialTrackingState{}
	state.RecordDenial()
	state.RecordDenial()
	state.RecordDenial()

	result := HandleDenialLimitExceeded(state, &config, false, "test reason")

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Behavior != types.PermissionBehaviorAsk {
		t.Errorf("expected Ask behavior in interactive mode, got %v", result.Behavior)
	}
}

func TestHandleDenialLimitExceeded_Headless(t *testing.T) {
	config := DefaultDenialLimitConfig()
	state := &types.DenialTrackingState{}
	state.RecordDenial()
	state.RecordDenial()
	state.RecordDenial()

	result := HandleDenialLimitExceeded(state, &config, true, "test reason")

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Behavior != types.PermissionBehaviorDeny {
		t.Errorf("expected Deny behavior in headless mode, got %v", result.Behavior)
	}
	if result.Reason != "Agent aborted: too many classifier denials in headless mode" {
		t.Errorf("expected abort message, got %s", result.Reason)
	}
}

func TestIsSafeTool(t *testing.T) {
	safeTools := []string{"read_file", "grep", "glob", "ask_user_question", "web_search", "web_fetch"}
	unsafeTools := []string{"bash", "write_file", "edit_file", "powershell", "task_stop", "mcp__server__add"}

	for _, tool := range safeTools {
		if !isSafeTool(tool) {
			t.Errorf("expected %s to be safe", tool)
		}
	}

	for _, tool := range unsafeTools {
		if isSafeTool(tool) {
			t.Errorf("expected %s to be unsafe", tool)
		}
	}
}

func newParser() *XMLParser { return NewXMLParser() }

// ─── ParseBlock ──────────────────────────────────────────────────────────────

func TestParseBlock_Yes(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>yes</block>")
	require.NotNil(t, result)
	assert.True(t, *result)
}

func TestParseBlock_No(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>no</block>")
	require.NotNil(t, result)
	assert.False(t, *result)
}

func TestParseBlock_True(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>true</block>")
	require.NotNil(t, result)
	assert.True(t, *result)
}

func TestParseBlock_False(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>false</block>")
	require.NotNil(t, result)
	assert.False(t, *result)
}

func TestParseBlock_One(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>1</block>")
	require.NotNil(t, result)
	assert.True(t, *result)
}

func TestParseBlock_Zero(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>0</block>")
	require.NotNil(t, result)
	assert.False(t, *result)
}

func TestParseBlock_CaseInsensitive(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<BLOCK>YES</BLOCK>")
	require.NotNil(t, result)
	assert.True(t, *result)
}

func TestParseBlock_WithSpaces(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>  no  </block>")
	require.NotNil(t, result)
	assert.False(t, *result)
}

func TestParseBlock_EmptyString(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("")
	assert.Nil(t, result)
}

func TestParseBlock_MissingTag(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("no block tag here")
	assert.Nil(t, result)
}

func TestParseBlock_UnknownValue(t *testing.T) {
	p := newParser()
	result := p.ParseBlock("<block>maybe</block>")
	assert.Nil(t, result)
}

func TestParseBlock_InFullResponse(t *testing.T) {
	p := newParser()
	response := "<block>yes</block><reason>dangerous command</reason>"
	result := p.ParseBlock(response)
	require.NotNil(t, result)
	assert.True(t, *result)
}

// ─── ParseReason ─────────────────────────────────────────────────────────────

func TestParseReason_Present(t *testing.T) {
	p := newParser()
	result := p.ParseReason("<block>yes</block><reason>rm -rf is dangerous</reason>")
	assert.Equal(t, "rm -rf is dangerous", result)
}

func TestParseReason_Trimmed(t *testing.T) {
	p := newParser()
	result := p.ParseReason("<reason>  spaces around  </reason>")
	assert.Equal(t, "spaces around", result)
}

func TestParseReason_Missing(t *testing.T) {
	p := newParser()
	result := p.ParseReason("<block>no</block>")
	assert.Equal(t, "", result)
}

// ─── ParseThinking ───────────────────────────────────────────────────────────

func TestParseThinking_Present(t *testing.T) {
	p := newParser()
	response := "<thinking>The command could delete files.</thinking><block>yes</block>"
	result := p.ParseThinking(response)
	assert.Equal(t, "The command could delete files.", result)
}

func TestParseThinking_Missing(t *testing.T) {
	p := newParser()
	result := p.ParseThinking("<block>no</block>")
	assert.Equal(t, "", result)
}

// ─── ParseFullResponse ───────────────────────────────────────────────────────

func TestParseFullResponse_AllTags(t *testing.T) {
	p := newParser()
	response := "<thinking>Analysis here.</thinking><block>yes</block><reason>dangerous</reason>"
	blocked, reason, thinking := p.ParseFullResponse(response)

	require.NotNil(t, blocked)
	assert.True(t, *blocked)
	assert.Equal(t, "dangerous", reason)
	assert.Equal(t, "Analysis here.", thinking)
}

func TestParseFullResponse_AllowedNoReason(t *testing.T) {
	p := newParser()
	blocked, reason, thinking := p.ParseFullResponse("<block>no</block>")
	require.NotNil(t, blocked)
	assert.False(t, *blocked)
	assert.Equal(t, "", reason)
	assert.Equal(t, "", thinking)
}

func TestParseBlockWithReason_Both(t *testing.T) {
	p := newParser()
	blocked, reason := p.ParseBlockWithReason("<block>yes</block><reason>shell injection</reason>")
	require.NotNil(t, blocked)
	assert.True(t, *blocked)
	assert.Equal(t, "shell injection", reason)
}

// ─── IsParseFailure ──────────────────────────────────────────────────────────

func TestIsParseFailure_ShortResponse(t *testing.T) {
	p := newParser()
	assert.True(t, p.IsParseFailure("short"))
	assert.True(t, p.IsParseFailure(""))
}

func TestIsParseFailure_ErrorPattern(t *testing.T) {
	p := newParser()
	assert.True(t, p.IsParseFailure("error: could not parse request"))
	assert.True(t, p.IsParseFailure("cannot process this input"))
}

func TestIsParseFailure_ValidResponse(t *testing.T) {
	p := newParser()
	assert.False(t, p.IsParseFailure("<block>no</block>"))
	assert.False(t, p.IsParseFailure("<block>yes</block><reason>dangerous</reason>"))
}

// ─── FormatToolUseCompact ─────────────────────────────────────────────────────

func TestFormatToolUseCompact_WithInput(t *testing.T) {
	result := FormatToolUseCompact("bash", map[string]any{"command": "ls -la"})
	assert.Contains(t, result, "bash(")
	assert.Contains(t, result, "command=")
	assert.Contains(t, result, "ls -la")
	assert.Contains(t, result, ")")
}

func TestFormatToolUseCompact_NilInput(t *testing.T) {
	result := FormatToolUseCompact("read_file", nil)
	assert.Equal(t, "read_file()", result)
}

func TestFormatToolUseCompact_EmptyInput(t *testing.T) {
	result := FormatToolUseCompact("glob", map[string]any{})
	assert.Equal(t, "glob()", result)
}

func TestFormatToolUseCompact_LongValueTruncated(t *testing.T) {
	longVal := make([]byte, 600)
	for i := range longVal {
		longVal[i] = 'x'
	}
	result := FormatToolUseCompact("write_file", map[string]any{"content": string(longVal)})
	assert.Contains(t, result, "...")
}
