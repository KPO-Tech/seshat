// Package social groups tools that interact with social media and developer
// community platforms: Hacker News, dev.to, Reddit, and Twitter/X.
//
// Direct-messaging platforms (WhatsApp, Slack, Discord, Telegram, email) live
// in internal/tools/notifications/ — they are one-way delivery channels, not
// community spaces.
//
// Read-only tools (HN, dev.to public endpoints) are fully implemented.
// Write and authenticated tools carry stubs that return ErrNotImplemented
// until the relevant credentials are configured.
//
// Platform status:
//
//	Hacker News — fully implemented (public Firebase + Algolia APIs)
//	dev.to      — fully implemented (public read; write needs DEV_TO_API_KEY)
//	Reddit      — stub (needs REDDIT_CLIENT_ID / REDDIT_CLIENT_SECRET)
//	Twitter/X   — stub (needs TWITTER_BEARER_TOKEN)
//	LinkedIn    — external MCP via mcp-server-linkedin; native stub planned
package social
