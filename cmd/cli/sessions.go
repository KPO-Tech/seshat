package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

func runSessions(ctx context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("sessions", flag.ContinueOnError)
	flags.SetOutput(stderr)

	dbPath := flags.String("db", "", "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}

	sub := "list"
	rest := flags.Args()
	if len(rest) > 0 {
		sub = rest[0]
		rest = rest[1:]
	}

	switch sub {
	case "list", "ls":
		return sessionsList(ctx, *dbPath, rest, stdout, stderr)
	case "delete", "rm", "remove":
		return sessionsDelete(ctx, *dbPath, rest, stdout, stderr)
	case "prune":
		return sessionsPrune(ctx, *dbPath, rest, stdout, stderr)
	case "info", "show":
		return sessionsInfo(ctx, *dbPath, rest, stdout, stderr)
	default:
		// Treat bare "sessions" or unknown sub as list.
		return sessionsList(ctx, *dbPath, nil, stdout, stderr)
	}
}

// ─── list ─────────────────────────────────────────────────────────────────────

func sessionsList(ctx context.Context, dbPath string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("sessions list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	statusFilter := flags.String("status", "", "filter by status: active|closed")
	limitN := flags.Int("n", 0, "show at most N sessions (default: all)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	client, err := openSessionClient(ctx, dbPath)
	if err != nil {
		return err
	}
	defer client.Close()

	sessions, err := client.ListSessions()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Fprintln(stdout, "no sessions found")
		return nil
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})

	// Apply filters.
	filtered := sessions[:0]
	for _, s := range sessions {
		if *statusFilter != "" && !strings.EqualFold(string(s.Status), *statusFilter) {
			continue
		}
		filtered = append(filtered, s)
	}
	if *limitN > 0 && len(filtered) > *limitN {
		filtered = filtered[:*limitN]
	}

	if len(filtered) == 0 {
		fmt.Fprintln(stdout, "no sessions match the filter")
		return nil
	}

	// Header
	fmt.Fprintf(stdout, "%-38s  %-8s  %-5s  %-8s  %s\n",
		"ID", "STATUS", "TURNS", "TOKENS", "UPDATED")
	fmt.Fprintln(stdout, strings.Repeat("─", 80))

	for _, s := range filtered {
		updated := time.Unix(s.UpdatedAt, 0).Format("2006-01-02 15:04")
		fmt.Fprintf(stdout, "%-38s  %-8s  %-5d  %-8d  %s\n",
			s.ID.String(), s.Status, s.TotalTurns, s.TotalTokens, updated)
	}

	fmt.Fprintf(stdout, "\n%d session(s)\n", len(filtered))
	return nil
}

// ─── delete ───────────────────────────────────────────────────────────────────

func sessionsDelete(ctx context.Context, dbPath string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("sessions delete", flag.ContinueOnError)
	flags.SetOutput(stderr)
	allFlag := flags.Bool("all", false, "delete all sessions")
	if err := flags.Parse(args); err != nil {
		return err
	}

	client, err := openSessionClient(ctx, dbPath)
	if err != nil {
		return err
	}
	defer client.Close()

	if *allFlag {
		sessions, err := client.ListSessions()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Fprintln(stdout, "no sessions to delete")
			return nil
		}
		deleted := 0
		for _, s := range sessions {
			if err := client.DeleteSession(s.ID); err != nil {
				fmt.Fprintf(stderr, "warning: delete %s: %v\n", s.ID, err)
				continue
			}
			deleted++
		}
		fmt.Fprintf(stdout, "deleted %d session(s)\n", deleted)
		return nil
	}

	ids := flags.Args()
	if len(ids) == 0 {
		return fmt.Errorf("usage: sessions delete <id...> or sessions delete --all")
	}

	deleted := 0
	for _, id := range ids {
		if err := client.DeleteSession(sdk.SessionID(strings.TrimSpace(id))); err != nil {
			fmt.Fprintf(stderr, "error: delete %s: %v\n", id, err)
			continue
		}
		fmt.Fprintf(stdout, "deleted %s\n", id)
		deleted++
	}
	if deleted < len(ids) {
		return fmt.Errorf("%d of %d session(s) could not be deleted", len(ids)-deleted, len(ids))
	}
	return nil
}

// ─── prune ────────────────────────────────────────────────────────────────────

func sessionsPrune(ctx context.Context, dbPath string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("sessions prune", flag.ContinueOnError)
	flags.SetOutput(stderr)
	olderThanDays := flags.Int("older-than", 30, "delete sessions not updated in N days")
	closedOnly := flags.Bool("closed", true, "only prune closed sessions")
	if err := flags.Parse(args); err != nil {
		return err
	}

	client, err := openSessionClient(ctx, dbPath)
	if err != nil {
		return err
	}
	defer client.Close()

	sessions, err := client.ListSessions()
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -*olderThanDays).Unix()
	var toDelete []*sdk.SessionInfo
	for _, s := range sessions {
		if *closedOnly && s.Status != sdk.SessionStatusClosed {
			continue
		}
		if s.UpdatedAt < cutoff {
			toDelete = append(toDelete, s)
		}
	}

	if len(toDelete) == 0 {
		fmt.Fprintf(stdout, "no sessions older than %d days to prune\n", *olderThanDays)
		return nil
	}

	deleted := 0
	for _, s := range toDelete {
		if err := client.DeleteSession(s.ID); err != nil {
			fmt.Fprintf(stderr, "warning: delete %s: %v\n", s.ID, err)
			continue
		}
		deleted++
	}
	fmt.Fprintf(stdout, "pruned %d session(s) older than %d days\n", deleted, *olderThanDays)
	return nil
}

// ─── info ─────────────────────────────────────────────────────────────────────

func sessionsInfo(ctx context.Context, dbPath string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("sessions info", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	ids := flags.Args()
	if len(ids) == 0 {
		return fmt.Errorf("usage: sessions info <id...>")
	}

	client, err := openSessionClient(ctx, dbPath)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, id := range ids {
		session, err := client.LoadSession(ctx, sdk.SessionID(strings.TrimSpace(id)))
		if err != nil {
			fmt.Fprintf(stderr, "error: load %s: %v\n", id, err)
			continue
		}
		msgs := session.GetMessages()
		session.Close()

		// Find the SessionInfo for this ID.
		all, _ := client.ListSessions()
		var info *sdk.SessionInfo
		for _, s := range all {
			if s.ID == session.GetID() {
				info = s
				break
			}
		}

		fmt.Fprintf(stdout, "─── Session %s\n", id)
		if info != nil {
			fmt.Fprintf(stdout, "  status  : %s\n", info.Status)
			fmt.Fprintf(stdout, "  turns   : %d\n", info.TotalTurns)
			fmt.Fprintf(stdout, "  tokens  : %d\n", info.TotalTokens)
			fmt.Fprintf(stdout, "  created : %s\n", time.Unix(info.CreatedAt, 0).Format(time.RFC3339))
			fmt.Fprintf(stdout, "  updated : %s\n", time.Unix(info.UpdatedAt, 0).Format(time.RFC3339))
		}
		fmt.Fprintf(stdout, "  messages: %d\n", len(msgs))

		// Show a brief transcript preview.
		if len(msgs) > 0 {
			fmt.Fprintln(stdout, "\n  transcript preview:")
			shown := 0
			for _, msg := range msgs {
				if shown >= 6 {
					fmt.Fprintf(stdout, "    … (%d more messages)\n", len(msgs)-shown)
					break
				}
				for _, block := range msg.Content {
					if text, ok := block.(sdk.TextContent); ok {
						preview := strings.TrimSpace(text.Text)
						if len(preview) > 120 {
							preview = preview[:120] + "…"
						}
						preview = strings.ReplaceAll(preview, "\n", " ")
						if preview != "" {
							fmt.Fprintf(stdout, "    [%s] %s\n", msg.Role, preview)
							shown++
						}
					}
				}
			}
		}
		fmt.Fprintln(stdout)
	}
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func openSessionClient(ctx context.Context, dbPathOverride string) (*sdk.Client, error) {
	opts, err := loadRuntimeOptions(runtimeOverrides{SQLitePath: dbPathOverride})
	if err != nil {
		return nil, err
	}
	return sdk.NewClient(&sdk.ClientConfig{
		APIKey:            opts.APIKey,
		Model:             opts.Model,
		PersistSessions:   true,
		SessionSQLitePath: opts.SQLitePath,
	})
}
