# Audit du codebase Seshat — Analyse comparative face aux assistants de code concurrents

> Date : 2026-10-15
> Public : architecte confirmé
> Base de comparaison : `docs/recherche-assistants-code-terminal.md`
> Méthode : lecture directe du code source (~947 fichiers Go mappés), grep ciblés par sous-système, lecture du `TODO.md` et de l'`AGENTS.md`

---

## Résumé en 5 lignes

Seshat est un runtime d'agent IA en Go extrêmement riche (~947 fichiers Go), avec des sous-systèmes de permissions, sandbox Docker, mémoire multi-couches (incluant un knowledge graph), sous-agents avec worktrees, MCP, skills, hooks et compaction — souvent plus complets que ses concurrents. Les faiblesses principales sont : l'absence de sandbox OS natif (seul Codex en a un), une politique de télémétrie/confidentialité non documentée, des bugs connus non résolus (TODO.md : H9 — plan approval gate), et plusieurs fonctionnalités qui existent dans le code mais ne sont pas connectées au runtime. Par rapport aux concurrents, Seshat devrait en priorité : ajouter un sandbox OS léger en alternative à Docker, documenter sa politique de confidentialité, ajouter le mode MCP server, et fiabiliser le circuit d'approbation du plan mode.

---

## 1. Ce qui est bien fait (et souvent supérieur aux concurrents)

### 1.1 Permissions — le système le plus mature du comparatif

**Fichiers clés :** `internal/permissions/engine.go`, `internal/permissions/integration.go`, `internal/permissions/auto/`, `internal/execution/orchestrator.go`

Seshat dispose d'un moteur de permissions plus sophistiqué que tous ses concurrents :

- **6 modes** : `Auto`, `Bypass`, `OnRequest`, `Never`, `AcceptEdits`, `Granular` — contre 3 chez Codex et Gemini
- **Pattern matching par outil** : allow/deny/ask avec `PermissionRule` structurée (value, pattern, behavior, priority, source, reason) — équivalent à OpenCode mais avec priorité et source
- **Auto-mode avec classifieur IA** : `internal/permissions/auto/classifier.go` — un classifieur LLM évalue le risque de chaque action, avec fallback `FailClosed` et gestion du cas "aucun classifieur configuré" (demande à l'utilisateur en interactif, deny en headless). Claude Code a un safety review IA mais moins configurable
- **DangerousPatternChecker** : `types.NewDangerousPatternChecker()` dans l'orchestrator — détecte les patterns dangereux (ex: `rm -rf ~`, écriture dans `/etc/passwd`) avant l'exécution. Aucun concurrent n'a cela de façon aussi explicite
- **Grants turn-scoped et session-scoped** : les permissions accordées peuvent être limitées à un tour (`turnGrants` en mémoire) ou persistées (`permissions.json`). C'est plus fin que le "Always allow" binaire de Gemini
- **Shell rules** : filtrage par préfixe de commande shell (ex: `git push` → deny, `git status` → allow) — équivalent à Codex rules
- **Sandbox auto-allow** : `canSandboxAutoAllow` — auto-approuve les commandes qui tournent dans le sandbox. Aucun concurrent ne fait cette distinction

### 1.2 Sandbox Docker — équivalent à Gemini CLI

**Fichiers clés :** `internal/sandbox/docker.go`, `internal/sandbox/executor.go`, `internal/sandbox/filesystem.go`, `internal/sandbox/command.go`

- **DockerExecutor** avec containers persistants par environnement (réutilisation via `docker exec`)
- **FilesystemPolicy** et **CommandPolicy** pour restrictions
- **CPU/memory caps** (`--cpus`, `--memory`) et **isolation réseau optionnelle** (`--network=none`)
- **Labels Docker** pour identification et nettoyage des orphelins
- **NoopExecutor** pour le mode sans sandbox
- Docker labels pour identification (`com.seshat.sandbox`, `com.seshat.environment-id`)

C'est fonctionnellement équivalent au sandbox Docker de Gemini CLI, avec l'avantage des containers persistants. Le commentaire sur les limitations de Dagger (v0.21) montre une réflexion architecturale sérieuse.

### 1.3 Mémoire — le système le plus complet du comparatif

**Fichiers clés :** `internal/memory/types.go`, `internal/memory/manager.go`, `internal/memory/longterm/`, `internal/memory/learner.go`

Seshat a un système de mémoire à plusieurs couches qu'aucun concurrent n'égale :

- **ProjectMemory** : mémoire par projet avec entries typées
- **UserMemory** : préférences utilisateur globales
- **CrossSession** : résumés de sessions passées, `SessionSummary` avec outils utilisés
- **Long-term memory (knowledge graph)** : `internal/memory/longterm/` — un vrai knowledge graph SQLite avec entities, observations, relations. Extracteur LLM qui extrait automatiquement les entités des conversations. `RetrieveForContext` pour injecter la mémoire pertinente. **Aucun concurrent n'a cela** — Claude Code n'a que `~/.claude/` en plaintext, Codex n'a que AGENTS.md
- **Learner** : `ToolStats`, `ErrorLearner` (patterns d'erreurs), `ContextBuilder` — apprentissage des patterns d'usage d'outils
- **MemoryConfig** avec `MemoryRetentionPolicy` — politique de rétention configurable
- **FileStore** : persistance sur disque (LoadProjectMemory, SaveProjectMemory, LoadUserMemory, SaveUserMemory, LoadCrossSession, SaveCrossSession)

### 1.4 Sous-agents — système mature avec isolation

**Fichiers clés :** `internal/agent/runner.go`, `internal/agent/async.go`, `internal/agent/types.go`, `internal/agent/registry.go`, `internal/agent/loader.go`

- **AgentDefinition** complète : type, tools (glob patterns), disallowedTools, skills, mcpServers, model, effort, permissionMode, maxTurns, memory, background, isolation
- **Worktree isolation** : `IsolationMode: "worktree"` — isolation git worktree pour les sous-agents. Seul Claude Code offre cela nativement
- **AsyncAgentManager** : agents en arrière-plan avec events, wall-clock timeout, statut
- **ForkFromMessages** : héritage de contexte depuis le parent
- **EventFn** : propagation des RuntimeEvents du sous-agent vers le parent
- **Hooks SubagentStart/SubagentStop** : hooks dédiés au cycle de vie des sous-agents
- **SourceRef** : collecte automatique des sources consultées (fichiers, URLs, recherches)
- **AgentRegistry** : résolution d'agents dynamiques (skill-based)
- **RestrictSessionSkills** : restriction des skills disponibles pour un sous-agent

### 1.5 MCP — client complet

**Fichiers clés :** `pkg/mcp/mcp.go`, `internal/tools/system/mcp/`

- **4 transports** : stdio, HTTP, SSE, WebSocket — plus que la plupart des concurrents
- **4 scopes** : Project, User, Local, Enterprise — le scope Enterprise est unique
- **AddServer/RemoveServer/SetServerEnabled** : API complète de gestion
- **Env var expansion** : `${VAR}` dans les configs MCP (fixé dans TODO.md M4)
- **Outils MCP suivent le même modèle de permissions** que les outils built-in

### 1.6 Hooks — le système le plus extensible

**Fichiers clés :** `internal/hooks/executor.go`, `internal/hooks/hook.go`, `internal/hooks/timeout.go`

- **40+ événements hook** : SessionStart/End, QueryStart/Complete, Iteration, ToolUses, Turn, PreToolUse, PostToolUse, PreCompact, PostCompact, PreAPICall, PostAPICall, SubagentStart/Stop, PermissionRequest/Denied, WorktreeCreate/Remove, InstructionsLoaded, CwdChanged, FileChanged, etc.
- **HookTimeoutManager** : gestion des timeouts par hook
- **ExecuteAsync** pour hooks non bloquants
- **ExecuteFirst** pour hooks qui retournent au premier résultat non-Continue

Claude Code et Codex ont des hooks, mais aucun n'atteint cette granularité.

### 1.7 Compaction — présente et fonctionnelle

**Fichiers clés :** `internal/engine/loop.go` (`maybeAutoCompact`), `internal/runtime/memory/`

- **Auto-compaction** avec `CompactionStrategy` (AutoCompact, CheckBlockingLimit)
- **MicroCompactor** dans `internal/runtime/memory/`
- **Pre/Post compact hooks** : `HookEventPreCompact`, `HookEventPostCompact`
- **CompactionRuntimeEvent** pour le suivi
- Tests de régression (fakeCompactionStrategy, countingCompactionStrategy, blockingLimitExceededCompactor)

### 1.8 Autres atouts notables

- **RepoMap** (`pkg/repomap/`) : mapping AST du repo, équivalent à Aider
- **RAG** (`internal/rag/`, `pkg/rag/`) : avec embedder, enricher, reranker — aucun concurrent CLI n'a cela
- **16+ providers** dont Ollama (local) — équivalent à Aider/OpenCode
- **Plan mode** avec `enter_plan_mode`/`exit_plan_mode`/`submit_plan`
- **Goal system** (`internal/agent/goal/`)
- **Checkpoints SQLite** : `session_checkpoints` pour la récupération de session
- **Document reader** (PDF, DOCX, PPTX, XLSX), audio (STT/TTS), notebook (Jupyter)

---

## 2. Ce qui ne va pas ou manque (par ordre de priorité)

### P0 — Bugs et problèmes de sécurité

#### 2.1 Plan approval gate ne fonctionne pas (TODO.md H9)

**Statut :** partiellement fixé mais incomplet

`exit_plan_mode` peut être appelé par le modèle sans approbation utilisateur. Le TODO.md documente que trois mécanismes ont été construits pour livrer l'approbation et que les trois sont inertes :
- `ExitPlanModeTool.Definition().RequiresPermission` = `true` mais lu par rien
- `ToolUseContext.RequestPrompt` déclaré mais jamais assigné
- `ExitPlanModeTool.RequiresUserInteraction` retourne `false`

Le fix partiel transforme l'ask en Deny, mais personne ne voit le preview et il n'y a pas de moyen d'approuver un plan hors TUI. **C'est un problème de sécurité** : la garantie read-only du plan mode est advisory.

#### 2.2 Auto-mode classifier — faille de sécurité résolue mais à surveiller

**Statut :** fixé (TODO.md M2)

Le classifier auto-mode allowanceait toutes les actions en auto mode quand `e.autoMode` était `nil` (cas commun). C'est maintenant fixé (`ensureAutoMode()` au construction), mais l'incident montre que le code de permissions a des chemins critiques qui peuvent être silently bypassés. **Recommandation : audit systématique de tous les chemins de fallback dans `checkGlobalPermission`.**

#### 2.3 Télémétrie non documentée

**Fichiers concernés :** `internal/monitoring/tracer.go`, `internal/permissions/auto/telemetry.go`

Seshat utilise OpenTelemetry (`go.opentelemetry.io/otel`) pour le tracing et a un module `telemetry.go` dans l'auto-mode qui log les classifications. Mais :
- **Aucune documentation** sur ce qui est collecté et envoyé
- **Pas de variable d'opt-out** claire (contrairement à Claude Code : `DISABLE_TELEMETRY=1`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`)
- **Pas de politique de confidentialité** dans le repo

C'est un manque sérieux pour un outil open source. Aider n'a aucune télémétrie (avantage), Claude Code et Codex ont des opt-out documentés.

### P1 — Fonctionnalités manquantes vs concurrents

#### 2.4 Pas de sandbox OS natif

Seshat a un sandbox Docker, mais pas de sandbox OS léger (seatbelt macOS, landlock Linux, Windows elevated). Codex CLI a les deux : sandbox OS ET option Docker. Le sandbox Docker nécessite Docker installé, ce qui n'est pas le cas sur toutes les machines.

**Recommandation :** ajouter un sandbox OS natif en alternative légère à Docker, inspiré de Codex (`sandbox-exec` sur macOS, landlock sur Linux). Le sandbox Docker reste pour les cas nécessitant une isolation forte.

#### 2.5 Pas de mode MCP server

Codex CLI peut agir comme **client ET server MCP** (`codex mcp-server`). Seshat est client uniquement. Être server MCP permettrait à d'autres outils d'invoquer Seshat comme backend.

**Recommandation :** ajouter un mode `seshat mcp-server` qui expose les capacités de Seshat via MCP.

#### 2.6 Pas de système de plugins distribuables

Codex a des plugins (bundles de skills + MCP servers + configs partageables). Seshat a skills et MCP séparément mais pas de format de packaging combiné.

**Recommandation :** définir un format `plugin.toml` ou `plugin.json` qui bundle skills + MCP + config, avec un `seshat plugin install <url>`.

#### 2.7 Hiérarchie AGENTS.md à vérifier

Le rapport de recherche montre que Codex/OpenCode utilisent une hiérarchie à 3 niveaux : `~/.codex/AGENTS.md` (global) → `./AGENTS.md` (projet) → `./sous-dossier/AGENTS.md` (contexte local). Seshat a un `AGENTS.md` à la racine et lit `SESHAT.md` mais il faut vérifier si la hiérarchie multi-niveaux est implémentée.

**Recommandation :** vérifier et documenter la hiérarchie supportée. Si absente, l'implémenter (global → projet → sous-dossier).

#### 2.8 Interopérabilité des fichiers de contexte

Gemini CLI permet de configurer `contextFileName` pour lire `AGENTS.md` au lieu de `GEMINI.md`. Seshat lit `AGENTS.md` et `SESHAT.md` mais il n'est pas clair si `CLAUDE.md` est supporté en fallback (comme le fait OpenCode).

**Recommandation :** supporter explicitement `AGENTS.md` > `SESHAT.md` > `CLAUDE.md` en fallback, et le documenter.

#### 2.9 Checkpointing au niveau fichier

Gemini CLI a un checkpointing de fichiers (`--checkpointing` + `/restore`) qui snapshot les fichiers avant chaque modification. Seshat a des checkpoints de session (SQLite) mais pas de snapshots au niveau fichier individuel.

**Recommandation :** ajouter un snapshot fichier avant chaque tool de modification (edit, write, patch), avec `/undo` pour restaurer. OpenCode fait cela via Git-based snapshots.

### P2 — Améliorations qualitatives

#### 2.10 Presets de permissions nommés

OpenCode a des presets clairs : Build (full access), Plan (read-only). Seshat a des `ExecutionMode` (execute, plan) et des `PermissionMode` (6 modes) mais la combinaison n'est pas présentée comme des presets simples à l'utilisateur.

**Recommandation :** offrir des presets nommés (Build, Plan, Auto, ReadOnly) qui combinent execution mode + permission mode + sandbox, à la manière d'OpenCode.

#### 2.11 Transparency des sous-agents

Le rapport note que Codex CLI a perdu la transparence parent→sous-agent (la communication transite par le cloud). Seshat, étant open source et local, devrait garantir la transparence totale : logs lisibles de ce que le parent dit au sous-agent.

**Recommandation :** documenter que tous les échanges parent→sous-agent sont visibles dans les logs locaux, et ajouter un mode verbose pour tracer ces échanges.

#### 2.12 Partage de sessions

OpenCode a `/share` pour partager des sessions via un lien public. Seshat n'a pas cela. Ce n'est pas critique mais c'est un atout pour le debugging collaboratif.

**Recommandation :** ajouter un `seshat session export` et un `seshat session share` (optionnel, opt-in).

#### 2.13 Variables d'environnement de désactivation

Claude Code a `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` qui désactive tout le trafic optionnel d'un coup. Seshat devrait avoir un équivalent.

**Recommandation :** ajouter `SESHAT_DISABLE_NONESSENTIAL_TRAFFIC=1` qui désactive télémétrie, auto-update, feedback, etc.

#### 2.14 RAG intégré au context window

Seshat a un système RAG (`pkg/rag/`) mais il n'est pas clair s'il est automatiquement utilisé pour enrichir le contexte de l'agent avec des passages pertinents du codebase. Aider le fait via son repo map, Claude Code via des recherches grep/glob.

**Recommandation :** vérifier que le RAG est connecté au pipeline de contexte, et si oui, le documenter. Si non, le connecter.

---

## 3. Problèmes de qualité code identifiés

### 3.1 Fonctionnalités mortes (TODO.md)

Le TODO.md liste plusieurs fonctions qui existent dans le code mais ne sont jamais appelées :
- `ConnectWithOAuth` (MCP) — supprimé (stub sans OAuth réel)
- `RegisterPluginMcpServer`/`GetPluginMcpServers` — supprimés (pas de convention de découverte)
- `RegisterMCPSkillBuilder` — supprimé (jamais lu)
- `OnSkillChange`/`NotifySkillChange` — supprimés (pas de publisher)
- `InvalidateSkillCache`/`ClearSkillCache`/`ActivateConditionalSkillsForPaths` — supprimés (supersédés)
- `SetSeshatMdProvider` — supprimé (race condition multi-tenant)

**Bonne nouvelle :** la plupart ont été supprimés dans les fixes récents. Mais le pattern de "code écrit mais jamais connecté" indique un manque de tests d'intégration bout-en-bout.

### 3.2 Complexité du système de permissions

Le moteur de permissions (`internal/permissions/engine.go`) est extrêmement complexe avec de multiples chemins de décision (rules, hooks, classifier, sandbox auto-allow, safety checker, mode transformations). Cette complexité rend les bugs de sécurité difficiles à détecter (comme l'a montré H9 et M2).

**Recommandation :** ajouter une suite de tests de sécurité exhaustive qui couvre tous les chemins de décision, et documenter l'ordre de résolution des permissions dans un diagramme.

### 3.3 Taille du binary

Le repo contient `grpc.exe` (80 MB) committé dans le repo. Ce n'est pas une bonne pratique — les binaires ne devraient pas être versionnés.

**Recommandation :** retirer `grpc.exe` du repo, l'ajouter au `.gitignore`, et le builder via CI/CD.

---

## 4. Tableau de synthèse : Seshat vs concurrents

| Dimension | Seshat | vs Claude Code | vs Codex CLI | vs Gemini CLI | vs Aider | vs OpenCode |
|---|---|---|---|---|---|---|
| **Permissions** | ✅ Supérieur (6 modes, classifier IA, pattern matching, dangerous patterns) | ≈ Égal (auto mode + safety review) | ≈ Égal (sandbox OS + rules) | ✅ Supérieur (3 modes basiques) | ✅ Supérieur (--yes binaire) | ✅ Supérieur (granularité équivalente + priority/source) |
| **Sandbox** | ⚠️ Docker seulement | ✅ Supérieur (pas de sandbox mais permissions) | ⚠️ Inférieur (pas de sandbox OS natif) | ≈ Égal (Docker) | ✅ Supérieur (Docker intégré) | ✅ Supérieur (pas de sandbox mais permissions) |
| **Mémoire** | ✅ Supérieur (knowledge graph + learner + cross-session) | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur (repo map unique) | ✅ Supérieur |
| **Sous-agents** | ✅ Supérieur (worktree, async, registry, hooks, source tracking) | ≈ Égal | ≈ Égal | ≈ Égal | ✅ Supérieur (pas de sous-agents) | ≈ Égal |
| **MCP** | ⚠️ Client seulement (pas de server mode) | ≈ Égal | ⚠️ Inférieur (Codex = client + server) | ≈ Égal | ✅ Supérieur (pas de MCP) | ≈ Égal |
| **Skills** | ✅ Complet (fork, shell.before, aliases, restriction) | ≈ Égal | ≈ Égal | ✅ Supérieur | ✅ Supérieur (pas de skills) | ≈ Égal |
| **Hooks** | ✅ Supérieur (40+ events, timeout manager) | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur (pas de hooks) | ✅ Supérieur (pas de hooks documentés) |
| **Compaction** | ✅ Présente (auto + micro + hooks) | ≈ Égal | ≈ Égal | ✅ Supérieur (1M tokens) | ✅ Supérieur (repo map économe) | ≈ Égal |
| **Télémétrie** | ❌ Non documentée, pas d'opt-out clair | ⚠️ Mieux (opt-out documenté) | ⚠️ Mieux (disable_response_storage) | ❌ Non documentée aussi | ✅ Supérieur (aucune) | ⚠️ Mieux (pas de télémétrie documentée) |
| **Confidentialité** | ✅ Local possible (Ollama) | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur | ≈ Égal | ≈ Égal |
| **RepoMap** | ✅ Présent (`pkg/repomap/`) | ✅ Supérieur (pas de repomap) | ✅ Supérieur | ✅ Supérieur | ⚠️ Inférieur (Aider a le repomap de référence) | ✅ Supérieur |
| **RAG** | ✅ Présent (embedder, enricher, reranker) | ✅ Supérieur (pas de RAG) | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur | ✅ Supérieur |

**Légende :** ✅ = Seshat supérieur | ≈ = Égal | ⚠️ = Seshat inférieur | ❌ = Seshat déficient

---

## 5. Recommandations priorisées

### P0 — Critique (sécurité et confiance)

1. **Documenter la politique de télémétrie/confidentialité** et ajouter `SESHAT_DISABLE_NONESSENTIAL_TRAFFIC=1` — un utilisateur open source doit savoir ce qui sort de sa machine
2. **Finir de fixer H9** (plan approval gate) — l'approbation du plan doit atteindre l'utilisateur, avec un hook SDK pour approuver hors TUI
3. **Retirer `grpc.exe` (80 MB) du repo** — `.gitignore` + build CI
4. **Auditer tous les chemins de fallback** dans `checkGlobalPermission` — M2 a montré qu'un fallback silencieux pouvait tout allowance

### P1 — Important (compétitivité)

5. **Ajouter un sandbox OS natif** (seatbelt macOS, landlock Linux) en alternative légère à Docker
6. **Ajouter le mode MCP server** (`seshat mcp-server`) — permet à d'autres outils d'utiliser Seshat comme backend
7. **Vérifier et documenter la hiérarchie AGENTS.md** (global → projet → sous-dossier) + supporter `CLAUDE.md` en fallback
8. **Ajouter le checkpointing au niveau fichier** (snapshot avant edit/write + `/undo`)
9. **Connecter le RAG au pipeline de contexte** si ce n'est pas déjà fait, et le documenter

### P2 — Améliorations (polish)

10. **Presets de permissions nommés** (Build, Plan, Auto, ReadOnly) combinant execution mode + permission mode + sandbox
11. **Système de plugins distribuables** (bundle skills + MCP + config)
12. **Partage de sessions** (`seshat session export/share`)
13. **Transparency des sous-agents** : documenter et tracer les échanges parent→sous-agent
14. **Suite de tests de sécurité** exhaustive pour le moteur de permissions

---

## 6. Conclusion

Seshat est un runtime d'agent IA exceptionnellement riche, souvent plus complet que ses concurrents sur les dimensions permissions, mémoire, sous-agents et hooks. Les fondations sont solides et l'architecture est claire (`internal/` vs `pkg/`, séparation des responsabilités).

Les faiblesses principales ne sont pas des manques de fonctionnalités mais des problèmes de **fiabilité** (bugs TODO.md), de **transparence** (télémétrie non documentée) et de **légereté** (sandbox Docker uniquement, pas de sandbox OS). Le code mort supprimé récemment montre une bonne hygiène de maintenance, mais le pattern de "code écrit mais jamais connecté" suggère un besoin de tests d'intégration plus poussés.

Pour un nouvel agent open source qui veut se positionner face à Claude Code, Codex CLI et OpenCode, Seshat a déjà tous les atouts techniques. Le travail restant est principalement du **durcissement** (sécurité, tests, documentation de confidentialité) et de l'**écosystème** (MCP server, plugins, interopérabilité des fichiers de contexte).
