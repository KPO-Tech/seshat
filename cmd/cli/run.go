package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/EngineerProjects/nexus-engine/internal/memory"
	engineconfig "github.com/EngineerProjects/nexus-engine/pkg/config"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

func runOnce(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)

	model := flags.String("model", "", "")
	permissionMode := flags.String("permission-mode", "", "")
	cwd := flags.String("cwd", "", "")
	dbPath := flags.String("db", "", "")
	showThinking := flags.Bool("show-thinking", false, "")
	debug := flags.Bool("debug", false, "")
	if err := flags.Parse(args); err != nil {
		return err
	}

	prompt := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if prompt == "" {
		return fmt.Errorf("missing prompt: use `nexus run \"prompt\"`")
	}

	options, err := loadRuntimeOptions(runtimeOverrides{
		Model:          *model,
		PermissionMode: *permissionMode,
		WorkingDir:     *cwd,
		SQLitePath:     *dbPath,
		Debug:          debug,
	})
	if err != nil {
		return err
	}
	if err := validateProviderSetup(options); err != nil {
		return err
	}

	reader := bufio.NewReader(stdin)
	printer := newStreamPrinter(stdout, *showThinking)

	client, err := newClient(
		options,
		func(promptCtx context.Context, request sdk.PromptRequest) (sdk.PromptResponse, error) {
			printer.ensureBoundary()
			return promptOnConsole(promptCtx, request, reader, stdout)
		},
		printer.handleProgress,
		printer.handleChunk,
		nil,
	)
	if err != nil {
		return err
	}
	defer client.Close()

	printer.startTurn()
	response, err := client.Ask(ctx, prompt, nil)
	printer.finishTurn(nil)
	if err != nil {
		return err
	}
	if *showThinking && strings.TrimSpace(response.Thinking) != "" {
		fmt.Fprintln(stdout, "thinking")
		fmt.Fprintln(stdout, indentBlock(strings.TrimSpace(response.Thinking), "  "))
	}
	return nil
}

func runChat(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	flags.SetOutput(stderr)
	noTUI := flags.Bool("no-tui", false, "")

	model := flags.String("model", "", "")
	permissionMode := flags.String("permission-mode", "", "")
	cwd := flags.String("cwd", "", "")
	dbPath := flags.String("db", "", "")
	resumeSessionID := flags.String("resume", "", "")
	showThinking := flags.Bool("show-thinking", false, "")
	debug := flags.Bool("debug", false, "")
	if err := flags.Parse(args); err != nil {
		return err
	}

	options, err := loadRuntimeOptions(runtimeOverrides{
		Model:          *model,
		PermissionMode: *permissionMode,
		WorkingDir:     *cwd,
		SQLitePath:     *dbPath,
		Debug:          debug,
	})
	if err != nil {
		return err
	}

	// Launch the TUI when running interactively. Fall back to the text-mode
	// chat loop when stdout is not a terminal or --no-tui is passed.
	if !*noTUI && isatty.IsTerminal(os.Stdout.Fd()) {
		_ = resumeSessionID // future: pass to workspace for auto-resume
		return runInteractive(ctx, options)
	}

	if err := validateProviderSetup(options); err != nil {
		return err
	}

	reader := bufio.NewReader(stdin)
	printer := newStreamPrinter(stdout, *showThinking)
	client, err := newClient(
		options,
		func(promptCtx context.Context, request sdk.PromptRequest) (sdk.PromptResponse, error) {
			printer.ensureBoundary()
			return promptOnConsole(promptCtx, request, reader, stdout)
		},
		printer.handleProgress,
		printer.handleChunk,
		nil,
	)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := loadChatSession(ctx, client, *resumeSessionID)
	if err != nil {
		return err
	}
	defer session.Close()

	printChatBanner(stdout, options, session, *showThinking)

	for {
		fmt.Fprint(stdout, "\n> ")
		line, err := readLine(ctx, reader)
		if err == io.EOF && strings.TrimSpace(line) == "" {
			fmt.Fprintln(stdout)
			return nil
		}
		if err != nil && err != io.EOF {
			return err
		}

		prompt := strings.TrimSpace(line)
		if prompt == "" {
			if err == io.EOF {
				fmt.Fprintln(stdout)
				return nil
			}
			continue
		}

		if strings.HasPrefix(prompt, "/") {
			updatedSession, exitChat, cmdErr := handleChatCommand(ctx, prompt, client, session, stdout, &options, showThinking)
			if cmdErr != nil {
				fmt.Fprintf(stdout, "error: %v\n", cmdErr)
			}
			if updatedSession != nil && updatedSession != session {
				_ = session.Close()
				session = updatedSession
			}
			if exitChat {
				return nil
			}
			continue
		}

		printer.startTurn()
		response, submitErr := session.SubmitMessage(ctx, prompt)
		printer.finishTurn(response)
		if submitErr != nil {
			fmt.Fprintf(stdout, "error: %v\n", submitErr)
			continue
		}
		if *showThinking {
			if thinking := extractThinkingFromMessages(latestAssistantMessages(response.Messages)); strings.TrimSpace(thinking) != "" && !printer.sawThinking {
				fmt.Fprintln(stdout, "thinking")
				fmt.Fprintln(stdout, indentBlock(strings.TrimSpace(thinking), "  "))
				fmt.Fprintln(stdout)
			}
		}

		if err == io.EOF {
			fmt.Fprintln(stdout)
			return nil
		}
	}
}

func loadChatSession(ctx context.Context, client *sdk.Client, sessionID string) (*sdk.Session, error) {
	if strings.TrimSpace(sessionID) != "" {
		return client.LoadSession(ctx, sdk.SessionID(strings.TrimSpace(sessionID)))
	}
	return client.CreateSession(ctx)
}

func printChatBanner(out io.Writer, options runtimeOptions, session *sdk.Session, showThinking bool) {
	fmt.Fprintf(out, "Nexus dev chat\n")
	fmt.Fprintf(out, "workspace: %s\n", options.WorkingDir)
	modelLabel := options.Model.String()
	if options.Model.Provider == "" {
		modelLabel = "(not configured — run `nexus config`)"
	}
	fmt.Fprintf(out, "model: %s\n", modelLabel)
	fmt.Fprintf(out, "permission: %s\n", options.PermissionMode)
	fmt.Fprintf(out, "session: %s\n", session.GetID().String())
	if showThinking {
		fmt.Fprintln(out, "thinking: visible")
	} else {
		fmt.Fprintln(out, "thinking: hidden (use /thinking)")
	}
	fmt.Fprintln(out, "commands: /help /new /sessions /resume <id> /thinking /exit")
}

func handleChatCommand(
	ctx context.Context,
	raw string,
	client *sdk.Client,
	session *sdk.Session,
	stdout io.Writer,
	options *runtimeOptions,
	showThinking *bool,
) (*sdk.Session, bool, error) {
	command, arg := splitCommand(raw)

	switch command {
	case "/help":
		fmt.Fprintln(stdout, "commands: /help /new /sessions /resume <id> /thinking /exit")
		return session, false, nil
	case "/new":
		next, err := client.CreateSession(ctx)
		if err != nil {
			return session, false, err
		}
		fmt.Fprintf(stdout, "new session: %s\n", next.GetID().String())
		return next, false, nil
	case "/sessions":
		return session, false, runSessions(ctx, nil, nil, stdout, io.Discard)
	case "/resume":
		if strings.TrimSpace(arg) == "" {
			return session, false, fmt.Errorf("usage: /resume <session-id>")
		}
		next, err := client.LoadSession(ctx, sdk.SessionID(strings.TrimSpace(arg)))
		if err != nil {
			return session, false, err
		}
		fmt.Fprintf(stdout, "resumed session: %s\n", next.GetID().String())
		return next, false, nil
	case "/thinking":
		*showThinking = !*showThinking
		if *showThinking {
			fmt.Fprintln(stdout, "thinking enabled")
		} else {
			fmt.Fprintln(stdout, "thinking hidden")
		}
		return session, false, nil
	case "/exit", "/quit":
		return session, true, nil
	case "/config":
		return session, false, fmt.Errorf("run `go run ./cmd/cli config` to update provider settings")
	default:
		return session, false, fmt.Errorf("unknown command %q", command)
	}
}

func splitCommand(raw string) (string, string) {
	command := strings.TrimSpace(raw)
	name, rest, found := strings.Cut(command, " ")
	if !found {
		return name, ""
	}
	return name, strings.TrimSpace(rest)
}

func validateProviderSetup(options runtimeOptions) error {
	if options.Model.Provider == "" {
		return nil // nothing configured yet; TUI will guide the user
	}
	config, err := engineconfig.Load()
	if err != nil {
		return err
	}
	config.Model = options.Model.String()
	config.APIKey = options.APIKey
	if validateErr := engineconfig.ValidateProviderSetup(config, options.Model.Provider); validateErr != nil {
		return fmt.Errorf("%w; run `nexus config` first", validateErr)
	}
	return nil
}

func latestAssistantMessages(messages []sdk.Message) []sdk.Message {
	lastIndex := -1
	var turnID string
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role != sdk.RoleAssistant {
			continue
		}
		lastIndex = index
		if messages[index].Metadata != nil {
			turnID = messages[index].Metadata.TurnID
		}
		break
	}
	if lastIndex < 0 {
		return nil
	}
	if strings.TrimSpace(turnID) == "" {
		return []sdk.Message{messages[lastIndex]}
	}

	assistantMessages := make([]sdk.Message, 0)
	for _, message := range messages {
		if message.Role != sdk.RoleAssistant || message.Metadata == nil || message.Metadata.TurnID != turnID {
			continue
		}
		assistantMessages = append(assistantMessages, message)
	}
	if len(assistantMessages) == 0 {
		return []sdk.Message{messages[lastIndex]}
	}
	return assistantMessages
}

func extractThinkingFromMessages(messages []sdk.Message) string {
	var builder strings.Builder
	for _, message := range messages {
		for _, block := range message.Content {
			thinking, ok := block.(sdk.ThinkingContent)
			if !ok {
				continue
			}
			builder.WriteString(thinking.Thinking)
		}
	}
	return builder.String()
}

// ============================================================================
// Memory Commands
// ============================================================================

func runMemory(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("memory", flag.ContinueOnError)
	flags.SetOutput(stderr)

	projectPath := flags.String("project", "", "")
	scope := flags.String("scope", "user", "")
	action := flags.String("action", "show", "")
	key := flags.String("key", "", "")
	value := flags.String("value", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}

	// Default to current directory
	if *projectPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		*projectPath = cwd
	}

	memManager, err := memory.NewManager()
	if err != nil {
		return fmt.Errorf("create memory manager: %w", err)
	}

	// Load based on scope
	switch *scope {
	case "user":
		if err := memManager.LoadUser(); err != nil {
			return fmt.Errorf("load user memory: %w", err)
		}
	case "project":
		if err := memManager.LoadProject(*projectPath); err != nil {
			return fmt.Errorf("load project memory: %w", err)
		}
	case "cross":
		if err := memManager.LoadCrossSession(); err != nil {
			return fmt.Errorf("load cross-session: %w", err)
		}
	default:
		if err := memManager.LoadAll(*projectPath); err != nil {
			return fmt.Errorf("load memory: %w", err)
		}
	}

	// Handle actions
	switch *action {
	case "show":
		return showMemory(memManager, *scope, stdout)
	case "set":
		if *key == "" || *value == "" {
			return fmt.Errorf("key and value required for set action")
		}
		return setMemory(memManager, *scope, *key, *value, stdout)
	case "clear":
		return clearMemory(memManager, *scope, stdout)
	case "context":
		return showContext(memManager, stdout)
	default:
		// Default: show all
		return showMemory(memManager, *scope, stdout)
	}
}

func showMemory(mem *memory.Manager, scope string, stdout io.Writer) error {
	switch scope {
	case "user":
		user := mem.GetUser()
		if user == nil || len(user.Entries) == 0 {
			fmt.Fprintln(stdout, "No user memory")
			return nil
		}
		fmt.Fprintln(stdout, "## User Preferences")
		for _, e := range user.Entries {
			fmt.Fprintf(stdout, "- %s: %s (%.0f%% confidence)\n", e.Key, e.Value, e.Confidence*100)
		}
	case "project":
		project := mem.GetProject()
		if project == nil || len(project.Entries) == 0 {
			fmt.Fprintln(stdout, "No project memory")
			return nil
		}
		fmt.Fprintln(stdout, "## Project Memory")
		for _, e := range project.Entries {
			fmt.Fprintf(stdout, "- %s: %s\n", e.Key, e.Value)
		}
	case "cross":
		cross := mem.GetCrossSession()
		if cross == nil {
			fmt.Fprintln(stdout, "No cross-session memory")
			return nil
		}
		fmt.Fprintln(stdout, "## Cross-Session Patterns")
		count := 0
		for _, p := range cross.GlobalPatterns {
			fmt.Fprintf(stdout, "- %s: %s (used %d times, %.0f%% success)\n", p.Key, p.Description, p.Frequency, p.SuccessRate*100)
			count++
			if count >= 20 {
				break
			}
		}
		if count == 0 {
			fmt.Fprintln(stdout, "No patterns learned yet")
		}
	default:
		user := mem.GetUser()
		if user != nil && len(user.Entries) > 0 {
			fmt.Fprintln(stdout, "## User Preferences")
			for _, e := range user.Entries {
				fmt.Fprintf(stdout, "- %s: %s\n", e.Key, e.Value)
			}
		}
		project := mem.GetProject()
		if project != nil && len(project.Entries) > 0 {
			fmt.Fprintln(stdout, "## Project Memory")
			for _, e := range project.Entries {
				fmt.Fprintf(stdout, "- %s: %s\n", e.Key, e.Value)
			}
		}
	}
	return nil
}

func setMemory(mem *memory.Manager, scope, key, value string, stdout io.Writer) error {
	switch scope {
	case "user":
		if err := mem.LearnPreference(memory.MemoryScopeUser, key, value, "cli"); err != nil {
			return fmt.Errorf("set preference: %w", err)
		}
		if err := mem.SaveUser(); err != nil {
			return fmt.Errorf("save user memory: %w", err)
		}
	case "project":
		fmt.Fprintf(stdout, "Project memory set requires project path\n")
		return nil
	default:
		return fmt.Errorf("unknown scope: %s", scope)
	}
	fmt.Fprintf(stdout, "Set %s: %s = %s\n", scope, key, value)
	return nil
}

func clearMemory(mem *memory.Manager, scope string, stdout io.Writer) error {
	switch scope {
	case "user":
		user := mem.GetUser()
		if user != nil {
			user.Entries = make(map[string]*memory.Entry)
			if err := mem.SaveUser(); err != nil {
				return fmt.Errorf("save user memory: %w", err)
			}
		}
	case "cross":
		cross := mem.GetCrossSession()
		if cross != nil {
			cross.GlobalPatterns = make(map[string]*memory.PatternEntry)
			cross.SessionSummaries = make(map[string]*memory.SessionSummary)
			if err := mem.SaveCrossSession(); err != nil {
				return fmt.Errorf("save cross-session: %w", err)
			}
		}
	default:
		return fmt.Errorf("clear not supported for %s", scope)
	}
	fmt.Fprintf(stdout, "Cleared %s memory\n", scope)
	return nil
}

func showContext(mem *memory.Manager, stdout io.Writer) error {
	ctx := mem.Context()
	if ctx == "" {
		fmt.Fprintln(stdout, "No memory context")
		return nil
	}
	fmt.Fprintln(stdout, ctx)
	return nil
}
