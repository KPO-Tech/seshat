# Sandbox Setup — Hackathon Judges Access

## Step 1 — Invite judges to the Slack workspace

Go to: slack.com → workspace `nexus-axh6106` → **Settings & administration → Manage members → Invite people**

Invite both:
- `slackhack@salesforce.com`
- `testing@devpost.com`

## Step 2 — Sandbox URL for DevPost submission

```
https://nexus-axh6106.slack.com
```

## Step 3 — Make the bot accessible

Make sure `@Seshat` is added to a public channel the judges can see.
Either `#general` or create a dedicated `#nexus-demo` channel.

Judges should be able to type `@Seshat hello` and get a response.

## Step 4 — Keep the bot running

The bot must be running when judges test it:

```bash
make slack-bot
```

Consider running it as a background service (screen, tmux, or systemd) so it stays up.
