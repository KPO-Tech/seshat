# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| `main` (latest) | ✅ Active |
| Older tags | ❌ No backports |

We only apply security fixes to the latest code on `main`. If you are using an older release, upgrade first.

---

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Use GitHub's private vulnerability reporting:

1. Go to **Security → Advisories** on the repository page.
2. Click **Report a vulnerability**.
3. Fill in the affected component, a description, and steps to reproduce.
4. Submit — the report is private and only visible to maintainers.

We will acknowledge receipt within **72 hours** and aim to provide a patch or mitigation within **14 days** for critical issues.

---

## Scope

Security issues we want to know about:

- **Sandbox escapes** — ways to execute code or read files outside the agent's working directory when Landlock sandboxing is active.
- **Permission bypass** — ways to invoke destructive tools in `onRequest` or `never` mode without triggering the permission check.
- **Credential leakage** — API keys or OAuth tokens exposed in logs, error messages, or persisted sessions in plaintext.
- **MCP prompt injection** — malicious tool results that cause the agent to exfiltrate data or execute unintended actions.
- **gRPC / HTTP exposure** — unauthenticated endpoints that allow arbitrary agent execution on a deployed instance.
- **Session isolation** — ways to read or write another user's session data when nexus-engine is deployed as a shared service.

Out of scope for this repository:

- Vulnerabilities in LLM providers (report directly to Anthropic, OpenAI, etc.).
- Social engineering or phishing.
- Issues requiring physical access to the host.
- Denial-of-service attacks on local single-user deployments.

---

## Security model

nexus-engine is a **local-first** runtime. By default:

- It runs as the invoking user — no privilege escalation.
- Bash commands are sandboxed on Linux via **Landlock** (kernel-level filesystem isolation scoped to the working directory).
- No telemetry is sent anywhere.
- Credentials are read from environment variables or the local `~/.nexus/auth.json` store — never sent to external services except the configured LLM provider.
- The gRPC server (`cmd/grpc`) has **no authentication layer** and is intended for local or trusted-network use only. Do not expose it publicly without adding your own auth proxy.

When deploying as a shared service (via nexus-product), additional security controls (user auth, session isolation, encrypted credential storage) are the responsibility of the product layer.

---

## Responsible disclosure

We follow a 90-day responsible disclosure policy. After a fix is shipped, we will publicly disclose the vulnerability in a GitHub Security Advisory with credit to the reporter (unless anonymity is requested).
