package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/EngineerProjects/nexus-engine/internal/providers"
)

// runLogin implements: nexus login [--provider openai] [--client-id ID]
//
// Flow:
//  1. Start Auth0 device-code flow → get user code + URL
//  2. Print both to stdout so the user can authenticate in a browser
//  3. Poll Auth0 until the token arrives, then persist it to ~/.nexus/auth.json
func runLogin(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)

	providerName := fs.String("provider", "openai", "Provider to authenticate (openai)")
	clientID := fs.String("client-id", "", "OAuth client ID (leave empty to use Codex CLI default)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Only OpenAI / ChatGPT is supported for browser auth; others use API keys.
	if *providerName != "openai" {
		return fmt.Errorf("browser login is only supported for --provider openai (got %q)", *providerName)
	}

	if *clientID != "" {
		providers.InitOAuth(*clientID)
	}

	fmt.Fprintf(stdout, "\nAuthenticating with ChatGPT (OpenAI)...\n\n")

	userCode, verificationURL, err := providers.LoginProvider(ctx, *providerName)
	if err != nil {
		return fmt.Errorf("start login: %w", err)
	}

	fmt.Fprintf(stdout, "  1. Open this URL in your browser:\n\n")
	fmt.Fprintf(stdout, "     %s\n\n", verificationURL)
	fmt.Fprintf(stdout, "  2. Enter this code when prompted:\n\n")
	fmt.Fprintf(stdout, "     %s\n\n", userCode)
	fmt.Fprintf(stdout, "Waiting for authentication (this may take up to 5 minutes)...\n")

	if err := providers.WaitForLogin(ctx, *providerName); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Fprintf(stdout, "\nAuthenticated! Your token is saved to ~/.nexus/auth.json\n")
	fmt.Fprintf(stdout, "You can now use: nexus chat --model openai:gpt-5.5\n\n")
	return nil
}
