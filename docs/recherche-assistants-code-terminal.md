# Rapport de recherche : Gestion des permissions, mémoire et sous-agents dans les assistants de code en terminal

> Sujet : Comment les meilleurs assistants de code en terminal (Claude Code, Codex CLI, Gemini CLI, Aider, OpenCode) gèrent-ils les permissions, la mémoire et les sous-agents, et que devrait retenir un nouvel agent open source comme le mien ?
>
> Date : 2026-10-15
> Public : architecte confirmé
> Statut : recherche basée sur documentation officielle, dépôts GitHub et articles techniques (2025-2026)

---

## Résumé en 5 lignes

Les cinq outils étudiés adoptent des philosophies de sécurité radicalement différentes : Codex CLI mise sur un sandbox OS natif (seatbelt/landlock/Windows), Gemini CLI sur un sandbox Docker auto-activé, tandis que Claude Code et OpenCode privilégient un système de permissions granulaire au niveau applicatif. Pour la mémoire, `AGENTS.md` s'impose comme le standard de facto (Codex, OpenCode, Gemini via configuration), complété par des systèmes de compaction automatique chez Codex, Claude Code et OpenCode. Les sous-agents sont natifs chez Claude Code, Codex, Gemini CLI et OpenCode (chacun avec isolation de contexte et modèles distincts), mais absents d'Aider qui reste un assistant mono-session. Aider se distingue par son repo map AST-based unique, mais ne supporte ni MCP ni sous-agents. **Pour un nouvel agent open source, le combo AGENTS.md + permissions granulaires pattern-matching + sous-agents isolés + MCP + sandbox optionnel constitue le minimum viable concurrentiel.**

---

## Tableau comparatif

| Dimension | Claude Code | Codex CLI | Gemini CLI | Aider | OpenCode |
|---|---|---|---|---|---|
| **Permissions** | Allow/deny par outil + auto mode avec safety review IA | 3 modes sandbox OS + 4 policies d'approbation + rules par préfixe | 3 modes (default/auto_edit/yolo) + allowlist tools | `--yes-always` ou confirmation manuelle, pas de sandbox intégré | Allow/deny/ask par outil + pattern matching + auto mode |
| **Sandbox** | Non (système de permissions applicatif) | **Oui, natif OS** : macOS seatbelt, Linux landlock, Windows elevated | **Oui, Docker** : auto-activé en yolo, image `gemini-cli-sandbox` | Docker manuel (`paulgauthier/aider`), non intégré | Non (permissions applicatives) |
| **Mémoire projet** | `CLAUDE.md` hiérarchique (global → projet → sous-dossier) | `AGENTS.md` 3 niveaux (global → projet → dossier) | `GEMINI.md` hiérarchique + configurable vers `AGENTS.md` | `CONVENTIONS.md` via `--read` (non auto-découvert) | `AGENTS.md` (canonique, `CLAUDE.md` en fallback) |
| **Persistance sessions** | `~/.claude/projects/` (transcripts plaintext, `cleanupPeriodDays`) | `~/.codex/`, `--resume`, `/resume`, `/fork` | `~/.gemini/`, checkpointing, `/restore` | `.aider.chat.history.md` + `.aider.input.history` | Sessions locales sur disque, `/sessions`, `/resume` |
| **Contexte / compaction** | Auto-compaction des longues conversations | Auto-compaction à ~220K tokens (v0.40+), `/compact` | 1M tokens (Gemini 3), token caching | Repo map AST/ctags (~1k tokens), pas de compaction | Agent `compaction` caché, auto + `/compact` |
| **Sous-agents** | **Oui** : Task tool, contexte isolé, parallélisme | **Oui** : agents TOML (`default`/`worker`/`explorer` + custom) | **Oui** : fichiers `.md` avec frontmatter dans `.gemini/agents/` | **Non** (architect mode = 2 modèles, pas de vrais sous-agents) | **Oui** : `task` tool, `@mention`, child sessions, modèles distincts |
| **MCP** | **Oui** : client complet, `.mcp.json` / settings | **Oui** : client + server (`codex mcp-server`), `config.toml` | **Oui** : `/mcp`, stdio + SSE | **Non** | **Oui** : `mcpServers` dans `opencode.json` (stdio/sse/remote) |
| **Hooks / Skills** | Hooks pre/post outil, Skills | Hooks (beta), Skills (`SKILL.md`), Plugins | Custom commands, pas de hooks formels | Pas de hooks, pas de skills | Skills globaux, pas de hooks documentés |
| **Télémétrie** | Statsig + Sentry, désactivable (`DISABLE_TELEMETRY=1`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`) | `disable_response_storage` pour ZDR, télémétrie non documentée publiquement | Non documentée publiquement (à vérifier) | **Aucune** (open source pur, pas de tracking) | Partage de sessions via `/share` (serveurs OpenCode), pas de télémétrie documentée |
| **Confidentialité** | Code → API Anthropic, opt-in training depuis août 2025 | Code → API OpenAI, ChatGPT auth ou API key | Code → API Google, OAuth Google | **100% local possible** (Ollama, LiteLLM, 100+ providers) | **100% local possible** (Ollama, LM Studio, 75+ providers) |
| **License** | Propriétaire (client) | Apache 2.0 | Apache 2.0 | Apache 2.0 | MIT |

---

## Constats par thème

### A. Permissions & sandbox

**Le clivage principal : sandbox OS vs permissions applicatives.**

**Codex CLI** est le seul à implémenter un véritable sandbox au niveau OS. Sur macOS, il utilise `sandbox-exec` (seatbelt) avec une policy `READ_ONLY_SEATBELT_POLICY` qui restreint l'accès filesystem et bloque le réseau. Sur Linux, il s'appuie sur landlock/seccomp. Sur Windows, un sandbox elevated avec `CreateRestrictedToken` isole l'exécution via un utilisateur restreint. Les trois modes (`read-only`, `workspace-write`, `danger-full-access`) combinés aux quatre policies d'approbation (`untrusted`, `on-failure`, `on-request`, `never`) offrent le contrôle le plus fin. Le système de **rules** permet en plus d'autoriser/refuser/demander pour des préfixes de commandes spécifiques (ex: `git push` → deny, `git status` → allow).

**Gemini CLI** opte pour un sandbox Docker (image `gemini-cli-sandbox`), automatiquement activé en mode yolo. Il supporte aussi podman, sandbox-exec, runsc et lxc. Le fichier `.gemini/sandbox.Dockerfile` permet de personnaliser l'image. C'est une approche plus lourde mais efficace.

**Claude Code** et **OpenCode** n'ont pas de sandbox OS. Ils s'appuient sur un système de permissions applicatif : chaque outil (bash, edit, read, glob, grep, task) peut être configuré en allow/deny/ask avec pattern matching. OpenCode se distingue par sa granularité (ex: `bash: {"": "ask", "git status ": "allow", "git push ": "deny"}`) et le fait que les règles `deny` sont **toujours** appliquées, même en mode auto. Claude Code ajoute un "auto mode" avec un filtre de sécurité IA pré-exécution qui évalue le risque de chaque action.

**Aider** est le moins équipé : `--yes-always` (anciennement `--yes`) pour tout auto-approuver, ou confirmation manuelle à chaque action. Le Docker est une option de déploiement manuel, pas un sandbox intégré.

**Incertain** : il n'est pas clair si Claude Code prévoit d'ajouter un sandbox OS natif. La documentation actuelle n'en mentionne pas.

### B. Mémoire & contexte persistant

**Le standard émergent : `AGENTS.md`.** Codex CLI a popularisé ce format avec une hiérarchie à 3 niveaux : `~/.codex/AGENTS.md` (global), `./AGENTS.md` (projet), `./sous-dossier/AGENTS.md` (contexte local). OpenCode l'a adopté comme fichier canonique (avec `CLAUDE.md` en fallback). Gemini CLI lit `GEMINI.md` par défaut mais peut configurer `contextFileName` pour lire `AGENTS.md`. Claude Code utilise `CLAUDE.md` avec la même logique hiérarchique.

**Persistance des sessions :** tous stockent les sessions localement, avec des structures variables :
- Claude Code : `~/.claude/projects/` en plaintext, avec `cleanupPeriodDays` pour la rétention
- Codex : `~/.codex/`, reprise via `--resume` ou `/resume`, fork via `/fork`
- Gemini : `~/.gemini/`, checkpointing de fichiers via `--checkpointing` + `/restore`
- Aider : `.aider.chat.history.md` et `.aider.input.history` dans le repo, `--restore-chat-history`
- OpenCode : sessions sur disque local, `/sessions` pour lister, `/resume` pour reprendre

**Gestion du context window :**
- **Codex** et **OpenCode** compactent automatiquement la conversation lorsqu'elle approche de la limite (agent `compaction` caché chez OpenCode, auto-compaction à ~220K tokens chez Codex depuis v0.40)
- **Claude Code** fait de l'auto-compaction/summarization
- **Gemini CLI** bénéficie de 1M tokens de contexte avec Gemini 3 + token caching
- **Aider** n'a pas de compaction. Il compense par son **repo map** unique : un graphe AST/ctags des classes et fonctions du repo, ranking algorithm pour sélectionner les plus pertinents, budget par défaut ~1024 tokens (`--map-tokens`). C'est l'approche la plus économe en tokens.

**Incertain** : le détail de l'algorithme de compaction de Claude Code n'est pas documenté publiquement.

### C. Sous-agents & orchestration

**4 outils sur 5 supportent les sous-agents nativement.**

**Claude Code** permet de spawner des sous-agents via le Task tool, avec un contexte isolé du parent. Le parallélisme est possible. Les sous-agents peuvent avoir des outils et permissions différents.

**Codex CLI** définit des agents en TOML dans `config.toml` : `[agents.name]` avec `description`, `config_file`, modèle, reasoning, sandbox et MCP propres. Trois agents built-in : `default`, `worker`, `explorer`. Les agents custom peuvent être spécialisés (reviewer, factchecker, securityaudit, codegen, etc.). **Limitation notable** : depuis 2026, la communication parent→sous-agent transite par le cloud et n'est plus visible en clair dans les logs locaux, ce qui réduit la transparence.

**Gemini CLI** définit les sous-agents comme des fichiers Markdown avec frontmatter YAML dans `.gemini/agents/` ou `~/.gemini/agents/`. Chaque agent peut avoir son propre modèle, ses propres tools (avec wildcards `mcp_*`), et même ses propres `mcpServers` isolés.

**OpenCode** a le système le plus structuré : 2 agents primaires (Build, Plan) + 3 sous-agents (General, Explore, Scout) + agents custom en JSON ou Markdown. Le Task tool crée une **child session** avec son propre context window, potentiellement un modèle différent. L'utilisateur peut invoquer un sous-agent via `@mention`. Les sous-agents read-only (Explore, Scout) n'ont pas accès aux outils de modification.

**Aider** est l'exception : pas de sous-agents. L'**architect mode** utilise deux modèles (un planner + un éditeur) mais dans une seule session séquentielle, pas une orchestration parallèle. C'est une limitation structurelle de son architecture mono-session.

### D. Extensibilité (MCP, plugins, hooks)

**MCP est devenu le standard d'extensibilité**, supporté par 4 outils sur 5 :
- **Codex CLI** : le plus complet — client ET server (`codex mcp-server`), config TOML, stdio + Streamable HTTP, OAuth MCP auth
- **Claude Code** : client complet, config via `.mcp.json` ou settings
- **Gemini CLI** : `/mcp` pour gérer, stdio + SSE
- **OpenCode** : `mcpServers` dans `opencode.json`, même modèle de permissions que les outils built-in
- **Aider** : pas de support MCP natif

**Au-delà de MCP :**
- **Codex** et **Claude Code** ont des **Skills** (fichiers `SKILL.md` / skills markdown) et des **Plugins** (bundles distributables)
- **Claude Code** a des **Hooks** pre/post exécution d'outils
- **Codex** a des **Hooks** (beta) — scripts shell dans la boucle agentique
- **OpenCode** a des **Skills** globaux mais pas de hooks documentés

### E. Persistance & confidentialité

**Le clivage : cloud-attached vs local-first.**

**Aider** et **OpenCode** peuvent fonctionner **100% localement** (Ollama, LM Studio, LiteLLM). Aider n'a aucune télémétrie. OpenCode stocke les sessions localement mais le `/share` envoie sur ses serveurs.

**Claude Code** envoie tout le code à l'API Anthropic. La télémétrie (Statsig, Sentry) est désactivable via `DISABLE_TELEMETRY=1` ou `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`. L'opt-in pour le training est actif depuis août 2025 (5 ans de rétention si activé). Les transcripts sont en plaintext dans `~/.claude/projects/`.

**Codex CLI** envoie le code à l'API OpenAI. `disable_response_storage` existe pour les comptes ZDR (Zero Data Retention). Le détail de la télémétrie n'est pas publiquement documenté.

**Gemini CLI** envoie le code à l'API Google. La politique de télémétrie n'est pas clairement documentée dans les sources consultées.

**Incertain** : les politiques exactes de télémétrie de Codex CLI et Gemini CLI n'ont pas pu être vérifiées précisément dans les sources publiques.

---

## Recommandations priorisées pour un nouvel agent open source

### P1 — Fondations (à implémenter en premier)

1. **Adopter `AGENTS.md` comme format de mémoire projet**, avec hiérarchie à 3 niveaux (global → projet → sous-dossier). C'est le standard de facto adopté par Codex, OpenCode et Gemini. Supporter `CLAUDE.md` en fallback pour interopérabilité.

2. **Système de permissions granulaire pattern-matching** inspiré d'OpenCode : allow/deny/ask par outil (bash, edit, read, glob, grep, task) avec matching sur les patterns. Les règles `deny` doivent toujours être appliquées, même en mode auto. C'est plus flexible que le binaire `--yes` d'Aider et plus léger qu'un sandbox OS.

3. **Sous-agents natifs avec isolation de contexte** : Task tool créant une child session avec son propre context window, son propre modèle et ses propres permissions. Inclure au minimum un agent read-only (exploration) et un agent généraliste. Permettre l'invocation via `@mention`.

4. **Support MCP client** dès le départ : configuration des serveurs MCP (stdio + HTTP), les outils MCP suivant le même modèle de permissions que les outils built-in.

### P2 — Différenciation (à implémenter ensuite)

5. **Sandbox optionnel** : ne pas imposer un sandbox lourd par défaut, mais offrir une option (Docker comme Gemini, ou intégration native comme Codex). Le sandbox doit s'auto-activer en mode "full auto" pour la sécurité.

6. **Auto-compaction intelligente** : agent caché qui compacte la conversation quand elle approche de la limite, comme Codex (seuil configurable) et OpenCode (agent `compaction`). Combiner avec un **repo map** à la Aider (AST-based, budget de tokens configurable) pour donner une vue structurelle du repo sans saturer le contexte.

7. **Persistance locale par défaut, cloud en opt-in** : sessions stockées localement en plaintext structuré (markdown ou JSON), nettoyer automatiquement après N jours configurables. Aucune télémétrie par défaut (suivre le modèle Aider). Supporter les modèles locaux (Ollama, LM Studio) pour le mode air-gapped.

8. **Checkpointing** : snapshot des fichiers avant chaque modification tool, avec restauration facile (inspiré de Gemini CLI `/restore` et OpenCode `/undo`).

### P3 — Polish (à implémenter en dernier)

9. **Skills** : fichiers markdown réutilisables encapsulant un workflow (comme Codex `SKILL.md` et Claude Code skills).

10. **Hooks** : scripts shell pre/post exécution d'outils (comme Claude Code et Codex beta) pour permettre l'intégration CI/CD et la personnalisation avancée.

11. **Plugins distribuables** : bundles de skills + MCP servers + configs partageables (comme Codex plugins).

12. **Modes d'approbation nommés et presets** : plutôt que des flags éparses, offrir des presets clairs (Plan = read-only, Build = full access, Auto = approve safe commands) inspirés d'OpenCode (Build/Plan) et Codex (read-only/workspace-write/danger-full-access).

---

## Sources et leurs limites

| Source | URL | Ce qui a été vérifié | Limites |
|---|---|---|---|
| Docs Anthropic — Data usage | code.claude.com/docs/en/data-usage | Télémétrie Claude Code, variables d'env, rétention | Doc officielle, peut évoluer |
| Anthropic Privacy Center | privacy.claude.com | Politique de rétention (5 ans si opt-in training), opt-out | Texte légal, interprétation |
| getvoibe.com — Claude Code Privacy | getvoibe.com/resources/claude-code-privacy-settings | Détail des variables d'env, comportement par provider | Source tierce, non officielle |
| GitHub — openai/codex | github.com/openai/codex | README, installation, structure | README high-level, pas le code source détaillé |
| ChatGPT Learn — Agent approvals & security | learn.chatgpt.com/docs/agent-approvals-security | Modes sandbox, approval policies, `--sandbox` flags | Doc officielle OpenAI |
| ChatGPT Learn — Configuration Reference | learn.chatgpt.com/docs/config-file/config-reference | `config.toml`, `agents`, `disable_response_storage`, compaction | Doc officielle, exhaustive |
| ChatGPT Learn — AGENTS.md | learn.chatgpt.com/docs/agent-configuration/agents-md | Hiérarchie AGENTS.md, `project_doc_fallback_filenames` | Doc officielle |
| philschmid.de — Codex CLI deep dive | philschmid.de/openai-codex-cli | Sandbox macOS seatbelt, `handleExecCommand`, `canAutoApprove` | Article tierce (avril 2025), peut être daté |
| OpenAI — Building Windows sandbox | openai.com/index/building-codex-windows-sandbox | Sandbox Windows, `CreateRestrictedToken`, `LogonUserW` | Article officiel OpenAI |
| inventivehq.com — Sandbox modes compared | inventivehq.com/blog/ai-coding-cli-sandbox-approval-modes-compared | Comparatif sandbox/approval des 3 CLI | Source tierce, vérifiée août 2026 |
| Gemini CLI — Configuration | geminicli.com/docs/reference/configuration | Flags `--yolo`, `--approval-mode`, `--sandbox`, settings.json | Doc officielle |
| Gemini CLI — Sandboxing | geminicli.com/docs/cli/sandbox | Méthodes sandbox (docker, podman, sandbox-exec, runsc, lxc) | Doc officielle |
| Gemini CLI — Subagents | geminicli.com/docs/core/subagents | Format .md + YAML frontmatter, `tools`, `mcpServers`, `model` | Doc officielle |
| localskills.sh — GEMINI.md guide | localskills.sh/blog/gemini-md-guide | `contextFileName` configurable, import `@./AGENTS.md` | Source tierce |
| Addy Osmani — Gemini CLI Tips | addyo.substack.com/p/gemini-cli-tips-and-tricks | Checkpointing, `/restore`, `/memory show`, hiérarchie GEMINI.md | Source tierce, fiable |
| aider.chat — Repository map | aider.chat/docs/repomap.html | Repo map, `--map-tokens` (1k défaut), graph ranking | Doc officielle |
| aider.chat — Options reference | aider.chat/docs/config/options.html | `--yes-always`, `.aider.chat.history.md`, `--restore-chat-history` | Doc officielle |
| aider.chat — FAQ | aider.chat/docs/faq.html | `.aiderignore`, `/read`, mono-repo | Doc officielle |
| agents.4geeks.com — Aider | agents.4geeks.com/agent/aider | Pas de MCP natif, architect mode | Source tierce |
| codegen.com — Aider Review | codegen.com/ai-tools/aider | Auto-commits, `--no-auto-commits`, limitations | Source tierce |
| OpenCode — Permissions | opencode.ai/docs/permissions | Système allow/deny/ask, pattern matching, auto mode | Doc officielle |
| OpenCode — Agents | opencode.ai/docs/agents | Primary/subagent, Build/Plan/General/Explore/Scout, task tool | Doc officielle |
| cefboud.com — OpenCode deep dive | cefboud.com/posts/coding-agents-internals-opencode-deepdive | Task tool internals, child sessions, permission bash parsing | Source tierce, analyse de code |
| fastino.ai — OpenCode guide | fastino.ai/blog/the-complete-guide-to-opencode-open-source-ai-coding-agents | AGENTS.md, multi-session, 75+ providers | Source tierce |
| proflead.dev — Codex subagents | proflead.dev/posts/codex-subagents-explained | Agents TOML, `default`/`worker`/`explorer`, config_file | Source tierce |
| yage.ai — Codex subagent transparency | yage.ai/share/codex-subagent-transparency-en-20260715.html | Perte de visibilité parent→subagent dans logs locaux | Source tierce, analyse |
| blakecrosley.com — Codex CLI Guide | blakecrosley.com/guides/codex | Timeline versions, auto-compaction 220K tokens (v0.40) | Source tierce, bien référencée |
| RoggeOhta/awesome-codex-cli | github.com/RoggeOhta/awesome-codex-cli | Vue d'ensemble : skills, subagents, plugins, MCP, hooks | Liste communautaire |
| memorylake.ai — Aider conventions | memorylake.ai/en/blogs/pin-conventions-into-aider | `CONVENTIONS.md` via `--read`, pas auto-découvert | Source tierce |

### Limites globales de cette recherche

- Le code source des outils n'a pas été lu directement (sauf extraits via articles tiers) ; les faits reposent sur la documentation officielle et des analyses secondaires.
- Les politiques de télémétrie de Codex CLI et Gemini CLI n'ont pas pu être vérifiées précisément.
- Les versions évoluent rapidement (Codex v0.78+, OpenCode v2, Gemini CLI v0.4+) ; certaines informations peuvent être datées d'ici quelques mois.
- Claude Code étant propriétaire (le client), son code source n'est pas auditable directement.
