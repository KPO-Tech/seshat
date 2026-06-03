package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

const (
	promptTypeText    = "text"
	promptTypeChoice  = "choice"
	promptTypeConfirm = "confirm"
)

type streamPrinter struct {
	mu                sync.Mutex
	out               io.Writer
	showThinking      bool
	sawText           bool
	sawThinking       bool
	thinkingAnnounced bool
	lineOpen          bool
}

func newStreamPrinter(out io.Writer, showThinking bool) *streamPrinter {
	return &streamPrinter{
		out:          out,
		showThinking: showThinking,
	}
}

func (p *streamPrinter) startTurn() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sawText = false
	p.sawThinking = false
	p.thinkingAnnounced = false
	p.lineOpen = false
}

func (p *streamPrinter) handleChunk(chunk sdk.ResponseChunk) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch chunk.Type {
	case sdk.ResponseChunkTypeContentBlockStart:
		switch block := chunk.ContentBlock.(type) {
		case sdk.ThinkingContent:
			if p.showThinking {
				p.ensureBoundaryLocked()
				fmt.Fprintln(p.out, "thinking")
			} else if !p.thinkingAnnounced {
				p.ensureBoundaryLocked()
				fmt.Fprintln(p.out, "[thinking]")
				p.thinkingAnnounced = true
			}
		case sdk.ToolUseContent:
			p.ensureBoundaryLocked()
			fmt.Fprintf(p.out, "[tool] %s\n", block.Name)
			fmt.Fprintln(p.out, indentBlock(formatMap(block.Input), "  "))
		}
	case sdk.ResponseChunkTypeContentBlockDelta:
		switch chunk.DeltaType {
		case "text_delta", "":
			if !p.sawText {
				p.ensureBoundaryLocked()
				p.sawText = true
			}
			fmt.Fprint(p.out, chunk.Delta)
			p.lineOpen = true
		case "thinking_delta":
			p.sawThinking = true
			if p.showThinking {
				fmt.Fprint(p.out, chunk.Delta)
				p.lineOpen = true
			}
		case "input_json_delta":
			if strings.TrimSpace(chunk.PartialJSON) != "" {
				p.ensureBoundaryLocked()
				fmt.Fprintf(p.out, "  input += %s\n", strings.TrimSpace(chunk.PartialJSON))
			}
		}
	case sdk.ResponseChunkTypeMessageStop:
		p.ensureBoundaryLocked()
	case sdk.ResponseChunkTypeError:
		if chunk.Error != nil {
			p.ensureBoundaryLocked()
			fmt.Fprintf(p.out, "error: %v\n", chunk.Error)
		}
	}
}

func (p *streamPrinter) handleProgress(progress sdk.ToolProgress) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureBoundaryLocked()
	label := strings.TrimSpace(progress.Message)
	if label == "" {
		label = string(progress.Stage)
	}
	if strings.TrimSpace(progress.ToolName) != "" {
		fmt.Fprintf(p.out, "[status] %s · %s\n", progress.ToolName, label)
		return
	}
	fmt.Fprintf(p.out, "[status] %s\n", label)
}

func (p *streamPrinter) finishTurn(response *sdk.SessionResponse) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureBoundaryLocked()

	if response == nil || p.showThinking || !p.sawThinking {
		return
	}
}

func (p *streamPrinter) ensureBoundary() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureBoundaryLocked()
}

func (p *streamPrinter) ensureBoundaryLocked() {
	if p.lineOpen {
		fmt.Fprintln(p.out)
		fmt.Fprintln(p.out)
		p.lineOpen = false
	}
}

func promptOnConsole(
	ctx context.Context,
	request sdk.PromptRequest,
	reader *bufio.Reader,
	stdout io.Writer,
) (sdk.PromptResponse, error) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, request.Message)
	switch string(request.Type) {
	case promptTypeConfirm:
		fmt.Fprint(stdout, "[y/N] ")
		value, err := readLine(ctx, reader)
		if err != nil {
			return sdk.PromptResponse{}, err
		}
		return sdk.PromptResponse{Value: strings.EqualFold(value, "y") || strings.EqualFold(value, "yes")}, nil
	case promptTypeChoice:
		for index, option := range request.Options {
			fmt.Fprintf(stdout, "%d. %s\n", index+1, option.Label)
		}
		fmt.Fprint(stdout, "choice> ")
		value, err := readLine(ctx, reader)
		if err != nil {
			return sdk.PromptResponse{}, err
		}
		for index, option := range request.Options {
			if value == fmt.Sprintf("%d", index+1) || strings.EqualFold(value, option.Label) {
				return sdk.PromptResponse{Value: option.Value}, nil
			}
		}
		return sdk.PromptResponse{Cancelled: true}, nil
	default:
		fmt.Fprint(stdout, "> ")
		value, err := readLine(ctx, reader)
		if err != nil {
			return sdk.PromptResponse{}, err
		}
		return sdk.PromptResponse{Value: value}, nil
	}
}

func readLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	type result struct {
		value string
		err   error
	}

	ch := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- result{value: strings.TrimSpace(line), err: err}
	}()

	select {
	case result := <-ch:
		if result.err != nil && result.err != io.EOF {
			return result.value, result.err
		}
		return result.value, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func indentBlock(text, prefix string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func formatMap(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
