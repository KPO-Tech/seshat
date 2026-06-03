package webfetch

import (
	"context"
	"fmt"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	fetchcore "github.com/EngineerProjects/nexus-engine/internal/web/fetch"
)

// Call executes the tool end-to-end while keeping the orchestration logic separate from the fetch backends.
func (t *Tool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	start := time.Now()

	parsedInput, err := parseInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsedInput.Validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}

	normalizedURL, parsedURL, err := fetchcore.NormalizeURL(parsedInput.URL)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	mode, err := fetchcore.NormalizeRenderMode(parsedInput.RenderMode)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	isPreapproved := fetchcore.IsPreapprovedPath(parsedURL.Hostname(), parsedURL.EscapedPath())

	if err := t.authorizeFetch(ctx, permissionCheck, normalizedURL, parsedURL.Hostname(), parsedInput, isPreapproved, mode); err != nil {
		return tool.NewErrorResult(err), nil
	}

	sessionID := input.ToolContextValue().SessionID
	if sessionID == "" {
		sessionID = input.SessionID
	}
	fetched, err := t.service.Fetch(ctx, fetchcore.Request{
		URL:        normalizedURL,
		RenderMode: mode,
		SessionID:  sessionID,
	})
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("fetch failed: %w", err)), nil
	}
	if fetched.Redirect != nil {
		return t.redirectResult(start, normalizedURL, fetched.Redirect), nil
	}

	output := Output{
		Bytes:         fetched.Bytes,
		Code:          fetched.Code,
		CodeText:      fetched.CodeText,
		Result:        t.applyPrompt(fetched.Content, parsedInput.Prompt, isPreapproved),
		DurationMs:    time.Since(start).Milliseconds(),
		URL:           fetched.FinalURL,
		Mode:          fetched.Mode,
		PersistedPath: fetched.PersistedPath,
		PersistedSize: fetched.PersistedSize,
	}

	result := tool.NewJSONResult(output)
	result.Content = t.formatOutput(output)
	result.Metadata = &tool.ResultMetadata{
		ExecutionDuration: output.DurationMs,
		Additional: map[string]any{
			"content_type":   fetched.ContentType,
			"preapproved":    isPreapproved,
			"prompt":         parsedInput.Prompt,
			"render_mode":    fetched.Mode,
			"persisted_path": fetched.PersistedPath,
			"persisted_size": fetched.PersistedSize,
		},
	}
	return result, nil
}

func (t *Tool) authorizeFetch(
	ctx context.Context,
	permissionCheck types.CanUseToolFn,
	normalizedURL string,
	domain string,
	input *Input,
	isPreapproved bool,
	mode string,
) error {
	if permissionCheck == nil || isPreapproved {
		return nil
	}

	permResult := permissionCheck(ctx, types.ToolPermissionRequest{
		ToolName: ToolName,
		ToolInput: map[string]any{
			"url":         normalizedURL,
			"prompt":      input.Prompt,
			"domain":      domain,
			"render_mode": mode,
		},
	})
	if permResult.Behavior == types.PermissionBehaviorAllow {
		return nil
	}

	reason := permResult.Reason
	if reason == "" {
		reason = fmt.Sprintf("fetching %s requires approval", domain)
	}
	return fmt.Errorf("permission denied: %s", reason)
}

func (t *Tool) redirectResult(start time.Time, requestedURL string, redirect *fetchcore.RedirectInfo) tool.CallResult {
	message := fmt.Sprintf(
		"REDIRECT DETECTED: The URL redirects to a different host.\n\nOriginal URL: %s\nRedirect URL: %s\nStatus: %d %s\n\nTo complete your request, fetch the redirected URL explicitly.",
		redirect.OriginalURL,
		redirect.RedirectURL,
		redirect.StatusCode,
		getStatusText(redirect.StatusCode),
	)
	output := Output{
		Bytes:      len(message),
		Code:       redirect.StatusCode,
		CodeText:   getStatusText(redirect.StatusCode),
		Result:     message,
		DurationMs: time.Since(start).Milliseconds(),
		URL:        requestedURL,
		Mode:       fetchcore.RenderModeHTTP,
	}

	result := tool.NewJSONResult(output)
	result.Content = t.formatOutput(output)
	result.Metadata = &tool.ResultMetadata{
		ExecutionDuration: output.DurationMs,
		Additional: map[string]any{
			"redirect":     true,
			"redirect_url": redirect.RedirectURL,
		},
	}
	return result
}
