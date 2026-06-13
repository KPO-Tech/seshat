package tools

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/lsp"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
)

const LSPRestartToolName = "lsp_restart"

//go:embed lsp_restart.md
var lspRestartDescription string

type LSPRestartParams struct {
	// Name is the optional name of a specific LSP client to restart.
	// If empty, all LSP clients will be restarted.
	Name string `json:"name,omitempty"`
}

func NewLSPRestartTool(lspManager *lsp.Manager) tool.Tool {
	t, _ := tool.NewBuilder(LSPRestartToolName).
		WithDescription(lspRestartDescription).
		NoPermission().
		WithHandler(func(ctx context.Context, input tool.CallInput, _ tool.ToolUseContext) (tool.CallResult, error) {
			name, _ := input.Parsed["name"].(string)
			if lspManager == nil || lspManager.Clients().Len() == 0 {
				return tool.NewTextResult("no LSP clients available to restart"), nil
			}

			clientsToRestart := make(map[string]*lsp.Client)
			if name == "" {
				maps.Insert(clientsToRestart, lspManager.Clients().Seq2())
			} else {
				client, exists := lspManager.Clients().Get(name)
				if !exists {
					return tool.NewTextResult(fmt.Sprintf("LSP client '%s' not found", name)), nil
				}
				clientsToRestart[name] = client
			}

			var restarted, failed []string
			var mu sync.Mutex
			var wg sync.WaitGroup
			for clientName, client := range clientsToRestart {
				wg.Go(func() {
					if err := client.Restart(); err != nil {
						slog.Error("Failed to restart LSP client", "name", clientName, "error", err)
						mu.Lock()
						failed = append(failed, clientName)
						mu.Unlock()
						return
					}
					mu.Lock()
					restarted = append(restarted, clientName)
					mu.Unlock()
				})
			}
			wg.Wait()

			var output string
			if len(restarted) > 0 {
				output = fmt.Sprintf("Successfully restarted %d LSP client(s): %s\n", len(restarted), strings.Join(restarted, ", "))
			}
			if len(failed) > 0 {
				output += fmt.Sprintf("Failed to restart %d LSP client(s): %s\n", len(failed), strings.Join(failed, ", "))
			}
			return tool.NewTextResult(output), nil
		}).
		Build()
	return t
}
