# Audit de Frontière `internal/` — Nexus Engine

> Produit le 2026-05-14 à partir d'une lecture complète du code réel.

---

## 1. Résumé exécutif

### Ce qui est clairement core aujourd'hui

Les packages suivants n'ont aucune dépendance produit, aucune hypothèse sur un utilisateur ou un workspace, et peuvent être consommés via le SDK sans aucun serveur :

- `internal/engine` — boucle principale model/tool
- `internal/execution` — orchestration des tools, pipeline, permissions
- `internal/runtime/state` — persistance de session (fs / memory / SQLite)
- `internal/runtime/hooks`, `internal/runtime/memory` — hooks et compaction runtime
- `internal/hooks` — hooks de cycle de vie du moteur
- `internal/memory` — gestion de mémoire de session
- `internal/prompt` — assembleur de prompt 4 couches
- `internal/permissions` — moteur de règles et auto-approve
- `internal/monitoring` — métriques et logging générique
- `internal/modes` — profils d'exécution (browse, plan, pair_programming, cache)
- `internal/providers` (sauf `auth.go`) — clients LLM, retry, circuit breaker, registry
- `internal/auth/types` — types OAuth génériques
- `internal/storage` — ArtifactStore interface + implémentations local/S3
- `internal/vector` — Store interface + implémentations memory/SQLite
- `internal/rag` — primitives génériques d'ingestion et de recherche RAG
- `internal/web` — browser, fetch, crawl, search, scholarly
- `internal/tools` — toutes les implémentations de tools (bash, agent, fs, etc.)
- `internal/types` — types partagés (Message, SessionMetadata, etc.)

### Ce qui est clairement backend aujourd'hui

- `internal/backend` — App, AuthService, QueryService, ResourcesService, MetricsService, Principal
- `internal/db/identity.go` — IdentityStore, User, Role (comptes produit)
- `internal/db/tenancy.go` — Organization, Workspace, WorkspaceMembership (tenancy produit)
- `internal/db/auth.go` — AuthSession, AuthPrincipal, auth session management
- `internal/db/security.go` — HashPassword, VerifyPassword, GenerateSessionToken (utilitaires produit)
- `internal/db/credentials.go` — credential store chiffré AES-GCM (secrets produit)

### Packages ambigus

1. **`internal/providers/auth.go`** — fonctions CLI (SetAPIKey, WaitForLogin, LoginProvider, LoginCommand) mélangées avec la résolution d'API key pour le runtime. La résolution `env var → FileStore → DB` est correcte pour CLI mais incorrecte pour multi-user produit.
2. **`internal/auth/store/` + `internal/auth/oauth/`** — infrastructure CLI-era de stockage de credentials provider dans `~/.nexus/auth.json`. Correcte pour headless/CLI, mais pas pour un serveur produit.
3. **`internal/db/schema.go`** — migrations core (session_storage, rag_vector_store) et backend (identity, tenancy, auth, credentials) dans une seule séquence. Couplage indirect entre core et backend sur la DB.
4. **`internal/db/db.go`** — handle DB partagé consommé par `runtime/state/sqlite_backend` (core) et `internal/backend` via `db.IdentityStore` (backend). Infra partagée légitime mais porte les deux responsabilités.

### Déplacements réellement nécessaires maintenant

Aucun. La frontière est correctement posée. `internal/backend` est le bon endroit. Les packages core sont clean.

Ce qui doit **évoluer** (pas se déplacer) :

1. `internal/backend/query.go` → `QueryService.ListSessions` est non scoped aujourd'hui (liste tout). Slice 1.
2. `internal/backend` → il manque `ProviderSettingsService` pour Slice 4.
3. `internal/db/schema.go` → commenter explicitement quelles migrations appartiennent à quel domaine.

### Déplacements à ne pas faire

- Ne pas toucher `internal/providers` (sauf auth.go → annotation suffisante)
- Ne pas sortir `internal/auth/store/` du core maintenant
- Ne pas fragmenter `internal/db` en "core DB" et "backend DB"
- Ne pas déplacer `internal/rag`, `internal/vector`, `internal/storage`

---

## 2. Cartographie package par package

### `internal/engine`

| Propriété | Valeur |
|---|---|
| Responsabilité | Orchestration du loop model/tool, sessions, streaming, compaction |
| Dépendances | execution, hooks, memory, monitoring, permissions, prompt, providers, runtime/memory, tools/registry, types, web/browser |
| Dépendances sur backend/db | Aucune |
| Niveau de stabilité | Élevé — testé, mature |
| Appartenance | **core** |
| Justification | Aucune notion produit. Consommable via SDK sans serveur. Frontière parfaitement propre. |
| Action | **Garder** |

---

### `internal/execution`

| Propriété | Valeur |
|---|---|
| Responsabilité | Pipeline d'exécution des tools, orchestrateur, hooks, permissions de tool, streaming |
| Dépendances | permissions, tools, providers, types |
| Dépendances sur backend/db | Aucune |
| Niveau de stabilité | Élevé |
| Appartenance | **core** |
| Justification | Primitive générique d'exécution de tool calls. Zéro hypothèse produit. |
| Action | **Garder** |

---

### `internal/runtime/state`

| Propriété | Valeur |
|---|---|
| Responsabilité | Persistance de session (metadata + transcript). Backends : filesystem, memory, SQLite |
| Dépendances | types, `internal/db` (pour sqlite_backend) |
| Dépendances sur backend/db | SQLite via `db.DB` pour le backend SQLite uniquement |
| Niveau de stabilité | Élevé — testé |
| Appartenance | **core** |
| Justification | Gestion de session générique. Aucun `user_id` / `workspace_id` dans les types. |
| Action | **Garder** |

Note : Le sqlite_backend dépend de `db.DB` pour la connexion, mais les tables sessions (`session_metadata`, `session_transcript_entries`, `session_checkpoints`) sont des tables core. Cette dépendance est légitime.

---

### `internal/runtime/hooks`

| Propriété | Valeur |
|---|---|
| Responsabilité | Registry et executor de hooks runtime (before/after tool call) |
| Dépendances | types |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/runtime/memory`

| Propriété | Valeur |
|---|---|
| Responsabilité | Compaction du transcript — réduction du contexte pour les longs échanges |
| Dépendances | providers, types |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/hooks`

| Propriété | Valeur |
|---|---|
| Responsabilité | Hooks de cycle de vie moteur (stop, interrupt, timeout) |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/memory`

| Propriété | Valeur |
|---|---|
| Responsabilité | Gestion de mémoire de session : catalog, learner, manager |
| Dépendances | types |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/prompt`

| Propriété | Valeur |
|---|---|
| Responsabilité | Assemblage de prompts 4 couches (CoreSections, StageTemplates, ToolHints, PromptConfig), cache control |
| Dépendances | types |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/permissions`

| Propriété | Valeur |
|---|---|
| Responsabilité | Moteur de règles pour approbation de tools. Auto-approve via classifieur LLM. |
| Dépendances | providers (pour auto-classifier), types |
| Appartenance | **core** |
| Justification | Les règles de permission sont génériques (patterns filesystem/shell). La politique produit (qui a accès à quoi dans un workspace) est distincte. |
| Action | **Garder** |

---

### `internal/monitoring`

| Propriété | Valeur |
|---|---|
| Responsabilité | Métriques runtime (compteurs, snapshots), logger |
| Dépendances | types |
| Appartenance | **core** |
| Justification | `internal/backend/metrics.go` consomme `monitoring.System` via interface. C'est le bon pattern — le core expose les primitives, le backend les expose côté produit. |
| Action | **Garder** |

---

### `internal/modes`

| Propriété | Valeur |
|---|---|
| Responsabilité | Profils d'exécution : browse, plan, pair_programming, cache |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/providers` (hors `auth.go`)

| Propriété | Valeur |
|---|---|
| Responsabilité | Clients LLM (Anthropic, OpenAI, Gemini, Ollama, etc.), registry, retry, circuit breaker, transport |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/providers/auth.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | Résolution d'API key pour les providers : env var → FileStore → (absent : DB). CLI OAuth device flow. |
| Dépendances | auth/oauth, auth/store, auth/types |
| Appartenance | **mixte / ambigu** |
| Justification | Deux fonctions coexistent : (1) résolution de clé API pour le runtime (core, utile headless) ; (2) OAuth device flow + persistance FileStore (CLI-era, inutile en serveur produit). Dans un serveur multi-user, `getAPIKeyForProvider` devrait lire depuis `internal/db/credentials.go` scopé user/workspace via un `ProviderSettingsService` backend. |
| Action | **Laisser provisoirement** avec dette explicitée. L'extraction est liée à Slice 4. |

Dette : La fonction `getAPIKeyForProvider` utilise une chaîne de fallback `env → FileStore` qui est correcte en CLI mais ignorera le credential store DB scopé en contexte produit.

---

### `internal/auth/oauth/`

| Propriété | Valeur |
|---|---|
| Responsabilité | OAuth device-code flow et callback server HTTP local. Client Auth0 pour ChatGPT. |
| Appartenance | **core (CLI-era)** |
| Justification | Primitive technique OAuth réutilisable. Actuellement utilisée uniquement par `providers/auth.go` pour le flow ChatGPT/OpenAI CLI. Un serveur produit n'en a pas besoin — il gère son propre OAuth via un IDP. |
| Action | **Laisser provisoirement**. À archiver ou déplacer vers un sous-package `cli/` quand le projet adopte une architecture CLI vs server explicite. |

---

### `internal/auth/store/`

| Propriété | Valeur |
|---|---|
| Responsabilité | FileStore : persiste les credentials OAuth et API keys dans `~/.nexus/auth.json` |
| Appartenance | **core (CLI-era)** |
| Justification | Outil de persistance CLI, non pertinent pour un serveur. Ne pas intégrer dans le backend produit. Ne pas non plus supprimer — il sert le path headless CLI/SDK. |
| Action | **Laisser provisoirement** |

---

### `internal/auth/types/`

| Propriété | Valeur |
|---|---|
| Responsabilité | Types Credentials, Token, AuthMethod |
| Appartenance | **core** |
| Action | **Garder** |

---

### `internal/auth/loader.go`

À lire si des doutes persistent, mais probablement chargement de config auth. Présumé core.

---

### `internal/db/db.go` + `config.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | Handle SQLite partagé, configuration, ouverture de connexion |
| Appartenance | **infrastructure partagée** |
| Justification | Consommé par `runtime/state/sqlite_backend` (core) et `internal/backend` (via IdentityStore). Ce partage est intentionnel et légitime. |
| Action | **Garder** |

---

### `internal/db/schema.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | Migrations SQLite de toutes les tables — sessions (core), identity/tenancy/auth/credentials (backend), vector_records (core) |
| Appartenance | **mixte — infrastructure partagée avec périmètre trop large** |
| Problème | Les migrations core (001_session_storage, 005_rag_vector_store) et backend (002, 003, 004) sont couplées dans une séquence unique. On ne peut pas bootstrapper le runtime sans créer les tables produit. |
| Action | **Laisser provisoirement**. Annoter les groupes. La séparation physique devient utile lors du passage à PostgreSQL (Slice 5). |

---

### `internal/db/identity.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | IdentityStore — CRUD users et roles, auth session, assignation de rôles, résolution de principal |
| Appartenance | **backend** |
| Justification | User, Role, AuthSession sont des objets produit. IdentityStore est consommé exclusivement par `internal/backend`. |
| Action | **Garder à sa place** (il est dans internal/db ce qui est acceptable — il sera la cible de Slice 5 pour PostgreSQL) |

---

### `internal/db/tenancy.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | Organization, Workspace, WorkspaceMembership — CRUD multi-tenant |
| Appartenance | **backend** |
| Action | **Garder** |

---

### `internal/db/auth.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | auth_session DB operations (create, touch, revoke, resolve principal from token) |
| Appartenance | **backend** |
| Action | **Garder** |

---

### `internal/db/credentials.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | KV store chiffré AES-GCM dans `credentials` table SQLite |
| Appartenance | **backend (partiel)** |
| Problème | La clé de chiffrement est chargée depuis `~/.nexus_secret` — non configurable, non scopée. Dans un serveur multi-user ou multi-instance, cette approche ne tient pas. |
| Action | **Laisser provisoirement** avec dette sur la gestion de clé. Slice 4 devra rendre la clé configurable et le scoping explicite. |

---

### `internal/db/security.go`

| Propriété | Valeur |
|---|---|
| Responsabilité | HashPassword (bcrypt), VerifyPassword, GenerateSessionToken |
| Appartenance | **backend (utilitaires)** |
| Action | **Garder** |

---

### `internal/db/bootstrap.go`

Seeding initial (seed admin, seed default roles, etc.). **Backend.** Garder.

---

### `internal/backend`

| Propriété | Valeur |
|---|---|
| Responsabilité | App, AuthService, QueryService, ResourcesService, MetricsService, Principal, errors |
| Appartenance | **backend** — c'est son rôle |
| Niveau de maturité | Correct dans sa structure mais trop mince en périmètre |

**Correctement implémenté :**
- Transport-agnostique ✓
- Interfaces claires (`QueryRuntime`, `SessionManager`, `StreamingQueryRuntime`) ✓
- Principal avec policy d'accès (HasRole, CanViewOrganization, CanViewWorkspace) ✓
- Erreurs typées ✓

**Ce qui manque (à ajouter, pas à déplacer) :**
- Session ownership dans `QueryService.ListSessions` — retourne toutes les sessions sans filtre owner
- `ProviderSettingsService` — résolution de credentials provider scopés user/workspace
- `FileService` — domaine fichiers/documents (Slice 2)
- `KnowledgeService` — domaine corpora/ingestion/retrieval (Slice 3)

| Action | **Garder et enrichir** |

---

### `internal/storage`

| Propriété | Valeur |
|---|---|
| Responsabilité | ArtifactStore interface + implémentations local/S3 |
| Appartenance | **core** |
| Justification | Interface purement générique. Pas de user/workspace. Utilisable via SDK. |
| Action | **Garder** |

---

### `internal/vector`

| Propriété | Valeur |
|---|---|
| Responsabilité | Store interface (Upsert/Search/DeleteNamespace/DeleteKeys) + implémentations memory/SQLite |
| Appartenance | **core** |
| Justification | Interface générique. `Namespace` est un string opaque — le backend produit y mettra un `corpus_id` mais l'interface reste neutre. |
| Action | **Garder** |

---

### `internal/rag`

| Propriété | Valeur |
|---|---|
| Responsabilité | Service d'ingestion (chunk → embed → store) et de recherche RAG |
| Dépendances | storage.ArtifactStore, vector.Store, Embedder, Chunker |
| Appartenance | **core** |
| Justification | `rag.Service` est une primitive générique. Elle prend un `CorpusID` string opaque — le domaine corpus (avec ownership, metadata, membership) sera une couche backend au-dessus. |
| Action | **Garder** |

---

### `internal/web`

| Propriété | Valeur |
|---|---|
| Responsabilité | Browser (Playwright), fetch/crawl, search providers (Tavily, Exa, DuckDuckGo, etc.), scholarly, policy réseau |
| Appartenance | **core** |
| Justification | Capabilities génériques consommables via tools. `web/policy.go` définit les actions autorisées — c'est une primitive runtime, pas une politique produit. Les policies produit (quotas, domain filters, admin settings) seront dans le backend mais utiliseront ces primitives. |
| Action | **Garder** |

---

### `internal/tools`

| Propriété | Valeur |
|---|---|
| Responsabilité | Toutes les implémentations de tools : bash, agent, browser, filesystem, builtin, registry |
| Appartenance | **core** |
| Action | **Garder** |

---

## 3. Frontière cible

### Core runtime (rester stable, tester via SDK)

```
internal/
  engine/
  execution/
  runtime/
    state/
    hooks/
    memory/
  hooks/
  memory/
  prompt/
  permissions/
  monitoring/
  modes/
  providers/      ← sauf auth.go (dette CLI)
  auth/
    types/        ← garder
    oauth/        ← dette CLI, laisser
    store/        ← dette CLI, laisser
  storage/
  vector/
  rag/
  web/
  tools/
  types/
  utils/
```

### Infrastructure partagée (core + backend l'utilisent légitimement)

```
internal/
  db/
    db.go
    config.go
    schema.go     ← annoter les groupes mais garder unifié
```

### Backend produit (évoluer indépendamment, ajouter des services)

```
internal/
  backend/
    app.go
    auth.go
    query.go      ← ajouter session ownership (Slice 1)
    resources.go
    metrics.go
    principal.go
    errors.go
    provider_settings.go  ← à créer (Slice 4)
    files.go              ← à créer (Slice 2)
    knowledge.go          ← à créer (Slice 3)
  db/
    identity.go
    tenancy.go
    auth.go
    credentials.go
    security.go
    bootstrap.go
```

### Règle de dépendance validée dans le code

```
cmd/api → internal/backend → internal/db + core packages
cmd/grpc → core packages
pkg/sdk → core packages
internal/backend → JAMAIS ← core packages
```

Cette règle est **respectée** dans le code actuel.

---

## 4. Plan de remaniement

### Slice A — Annoter le schéma DB (1h, zéro risque)

**Objectif** : Rendre visible dans schema.go quelles migrations appartiennent à quel domaine.

**Fichiers** : `internal/db/schema.go`

**Action** : Ajouter des commentaires de groupe :

```go
// ─── Core: session storage ────────────────────────────────────────────────────
{ id: "001_session_storage", ... }

// ─── Backend: identity ────────────────────────────────────────────────────────
{ id: "002_identity_core", ... }

// ─── Backend: tenancy + auth sessions ─────────────────────────────────────────
{ id: "003_tenancy_and_auth", ... }

// ─── Backend: credentials store ───────────────────────────────────────────────
{ id: "004_credentials_store", ... }

// ─── Core: vector store ───────────────────────────────────────────────────────
{ id: "005_rag_vector_store", ... }
```

**Risques** : Aucun.

**Ce qui ne doit pas être cassé** : Migrations existantes, ordre de passage.

---

### Slice B — Session ownership dans QueryService (Slice 1 de la roadmap)

**Objectif** : `QueryService.ListSessions` filtre par principal (user_id / workspace_id).

**Problème actuel** : `ListSessions` retourne toutes les sessions sans filtre. Aucun concept d'ownership en DB.

**Fichiers concernés** :
- `internal/db/schema.go` — nouvelle migration `006_session_ownership` (table `session_owners`)
- `internal/runtime/state/sqlite_backend.go` — permettre `ListSessionsByOwner`
- `internal/backend/query.go` — `ListSessions(ctx, principal)` filtre par owner
- `cmd/api/query.go` — passe `principal` au service

**Justification** : Un utilisateur ne doit pas voir les sessions d'un autre. Cette règle est produit → backend.

**Risques** : Breaking change sur `ListSessions`. Migrer les sessions existantes avec owner = system/bootstrap.

**Ce qui ne doit pas être cassé** : `runtime/state` core reste sans notion d'ownership. La couche backend ajoute le mapping séparé.

---

### Slice C — ProviderSettingsService (Slice 4 de la roadmap)

**Objectif** : Résolution des credentials LLM via le DB store scopé, pas via FileStore.

**Problème actuel** : `providers/auth.go:getAPIKeyForProvider` utilise la chaîne `env var → FileStore`. Pas de scoping user/workspace.

**Fichiers concernés** :
- `internal/backend/provider_settings.go` — nouveau service `ProviderSettingsService`
- `internal/db/credentials.go` — méthode `ListCredentialKeys` + scoping par préfixe
- `cmd/api/main.go` — injecter `ProviderSettingsService` dans les handlers qui en ont besoin

**Justification** : En contexte serveur multi-user, les API keys doivent être scopées par workspace et admin-configurables, pas lues depuis `~/.nexus/auth.json`.

**Invariant à respecter** : Le path CLI/SDK headless reste via `providers/auth.go` → FileStore. Ne pas casser la résolution actuelle pour les utilisateurs CLI.

**Risques** : Deux systèmes de credentials coexistent temporairement. C'est acceptable et explicite.

---

### Slice D — Séparer les migrations DB core/backend (lors de Slice 5 PostgreSQL)

**Objectif** : Avoir deux séquences de migrations distinctes — une pour le runtime core (SQLite), une pour le backend produit (Postgres).

**Problème actuel** : Schema.go mélange les deux. Acceptable aujourd'hui (tout en SQLite), problématique lors de la migration Postgres.

**Action lors de Slice 5** :
- `internal/db/schema_core.go` — migrations 001, 005 (sessions, vectors)
- `internal/db/schema_backend.go` — migrations 002, 003, 004 (identity, tenancy, credentials)
- `internal/db/schema.go` — dispatcher selon config (SQLite = both, Postgres = backend only)

**Ne pas faire avant Slice 5** : Ce refactor n'a pas de valeur sans le driver Postgres.

---

## 5. No-go list

### Déplacements à ne pas faire

| Déplacement tentant | Pourquoi c'est mauvais |
|---|---|
| Déplacer `internal/rag` dans `internal/backend` | rag.Service est une primitive core. La couche corpus/knowledge qui va par-dessus est backend, pas rag lui-même. |
| Déplacer `internal/vector` dans `internal/backend` | vector.Store est une interface générique. Le scoping par corpus appartient au backend mais pas l'interface. |
| Déplacer `internal/storage` dans `internal/backend` | ArtifactStore est consommé par web/browser, rag, engine — tous core. |
| Déplacer `internal/web` dans `internal/backend` | Les capabilities web sont des primitives runtime, pas de la logique produit. |
| Déplacer `internal/monitoring` dans `internal/backend` | Le core l'utilise directement. MetricsService dans le backend l'expose côté produit — c'est suffisant. |
| Séparer `internal/db` en core/backend maintenant | La valeur apparaît seulement lors du passage à Postgres. Avant : coût sans bénéfice. |
| Fusionner `internal/auth/store/FileStore` avec `internal/db/credentials.go` | Ce sont deux stores pour deux paths différents (CLI vs serveur). Les unifier forcerait une dépendance DB dans le path headless. |
| Extraire `ProviderSettingsService` en package séparé | Un service dans `internal/backend` suffit. Pas besoin de nouveau package. |
| Ajouter un "gateway" entre core et backend | Pattern Enterprise-Java. Le contrat est les interfaces déjà présentes dans `internal/backend/query.go` (`QueryRuntime`, `SessionManager`). C'est suffisant. |

### Abstractions à ne pas créer

- Pas d'interface `CredentialResolver` générique dans le core — inutile avant que deux implémentations coexistent réellement dans le même path
- Pas de "domain model" séparé pour core vs backend — les types `db.User`, `db.Organization` etc. sont déjà les bons types DB
- Pas de repository layer intermédiaire entre `internal/backend` et `internal/db` — `db.IdentityStore` est déjà ce repository

### Refactors prématurés à éviter

- Ne pas normaliser les noms de packages avant d'avoir une raison réelle
- Ne pas créer `internal/domain/` ou `internal/application/` — over-architecture DDD
- Ne pas abstraire `internal/db/DB` derrière une interface — inutile tant que le seul driver est SQLite
- Ne pas ajouter d'event bus ou de message queue interne tant que les slices 1-4 ne sont pas terminées

---

## Critère de réussite vérifié

| Critère | État |
|---|---|
| On sait précisément ce qui reste le core | ✓ — documenté section 2 |
| On sait précisément ce qui doit aller dans le backend | ✓ — documenté section 2 |
| On a un plan de déplacement réaliste et incrémental | ✓ — section 4 |
| On peut faire évoluer le core indépendamment du backend | ✓ — aucun core package ne dépend de `internal/backend` |
| On réduit le flou dans `internal/` sans casser l'architecture | ✓ — 4 slices ciblés, aucun remaniement structurel immédiat |
