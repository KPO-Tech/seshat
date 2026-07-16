package workflows

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/KPO-Tech/seshat/pkg/automation"
	"github.com/KPO-Tech/seshat/pkg/sdk"
)

const DefaultTechWatchModel = "anthropic:claude-sonnet-4-6"

var defaultTopics = []string{
	"AI agents and multi-agent systems",
	"Go programming language",
	"open-source AI infrastructure",
	"LLM tooling and frameworks",
}

// TechWatch fetches recent tech news on configured topics and produces a digest.
type TechWatch struct {
	Topics []string
	Since  string
}

func (t *TechWatch) Name() string        { return "tech-watch" }
func (t *TechWatch) Description() string { return "Fetch recent tech news and produce a digest" }

func (t *TechWatch) Run(ctx context.Context, session *sdk.Session) error {
	topics := t.Topics
	if len(topics) == 0 {
		topics = defaultTopics
	}
	since := t.Since
	if since == "" {
		since = "24h"
	}
	_, err := session.SubmitMessage(ctx, buildTechWatchPrompt(topics, since))
	return err
}

// compile-time interface check
var _ automation.Workflow = (*TechWatch)(nil)

// RunTechWatch is the CLI entry point for the tech-watch workflow.
func RunTechWatch(args []string) error {
	fs := flag.NewFlagSet("tech-watch", flag.ContinueOnError)
	topicsFlag := fs.String("topics", "", "comma-separated topics (default: AI agents, Go, OSS infra, LLM tooling)")
	sinceFlag := fs.String("since", "24h", "how far back to look (e.g. 24h, 48h, 7d)")
	modelFlag := fs.String("model", "", "model to use, format provider:model (default: "+DefaultTechWatchModel+")")

	fs.Usage = func() {
		fmt.Print(`Usage: seshat-auto tech-watch [flags]

Fetch recent tech news and print a markdown digest to stdout.

Flags:
`)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	model := DefaultTechWatchModel
	if *modelFlag != "" {
		model = *modelFlag
	}

	topics := defaultTopics
	if *topicsFlag != "" {
		topics = splitTopics(*topicsFlag)
	}

	runnerCfg, err := automation.RunnerConfigFromEnv(model)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	executor, err := automation.NewExecutor(automation.ExecutorConfig{
		RunnerConfig: runnerCfg,
		Middleware: []automation.Middleware{
			automation.WithLogging(nil),
		},
		Sinks: []automation.Sink{
			&automation.StdoutSink{},
		},
		State: automation.NewMemoryStateStore(),
	})
	if err != nil {
		return fmt.Errorf("build executor: %w", err)
	}
	defer executor.Close()

	fmt.Fprintf(os.Stderr, "[seshat-auto] tech-watch · topics: %s · since: %s · model: %s\n",
		strings.Join(topics, ", "), *sinceFlag, model)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	opts := automation.DefaultOptions()
	_, err = executor.Run(ctx, &TechWatch{Topics: topics, Since: *sinceFlag}, opts)
	return err
}

func buildTechWatchPrompt(topics []string, since string) string {
	lines := make([]string, len(topics))
	for i, t := range topics {
		lines[i] = "- " + t
	}
	return fmt.Sprintf(`You are a technology intelligence agent. Produce a concise tech watch digest.

Topics:
%s

Time range: last %s

Instructions:
1. Search for recent news, releases, papers, and discussions on each topic
2. For each topic find 2-3 of the most relevant items published in the time range
3. For each item provide: title, source/link, a 2-sentence summary, and one line on why it matters
4. Skip marketing content, minor patch releases, and duplicate coverage
5. Format as clean markdown with one section per topic

Prioritize: tool and framework releases, research breakthroughs, notable community discussions, architectural ideas.`,
		strings.Join(lines, "\n"),
		since,
	)
}

func splitTopics(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
