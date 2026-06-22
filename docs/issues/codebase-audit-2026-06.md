# Codebase Audit — Seshat CLI (June 2026)

Audit statique complet de `internal/`, `cmd/`, `pkg/` effectué après la session
d'optimisation DB + HNSW. Chaque finding est classé **NOW** (à corriger dans le sprint
courant) ou **LATER** (issue communauté open-source).

---

## NOW — Critiques (correctness / data integrity)

### C1 · Session leak dans le task manager ✅ FIXÉ
**Fichier :** `internal/runtime/tasks/manager.go`  
**Problème :** Si `session.RegisterTools()` échoue après `client.NewSession()`, la session
n'est jamais fermée → fuite de handle de session.  
**Fix :** Pattern `committed bool` + defer conditionnel.

---

### C2 · HNSW — partial write non détecté ✅ FIXÉ
**Fichier :** `internal/vector/hnsw_store.go`  
**Problème :** Si `graph.Save()` réussit mais `saveMeta()` échoue (ou les deux échouent),
seule la première erreur est retournée. Index et métadonnées sont désynchronisés en
silence.  
**Fix :** `errors.Join(saveErr, metaErr)` → les deux erreurs remontent.

---

### C3 · FTS5 — erreurs silencieusement ignorées ✅ FIXÉ
**Fichier :** `internal/db/migrations_sqlite_core.go`  
**Problème :** `_ = err` dans `migrateSQLiteVectorFTS5` et `migrateSQLiteTranscriptFTS5`.
Si la migration FTS5 échoue, la recherche hybride dégrade en O(n) LIKE sans aucun signal
à l'opérateur.  
**Fix :** `log.Printf("[db] fts5 migration warning: …")` — non-fatal mais visible.

---

### C4 · JSON unmarshal metadata — silencieux ✅ FIXÉ
**Fichiers :** `internal/vector/hnsw_store.go:92`, `internal/vector/sqlite_store.go:293`  
**Problème :** `_ = json.Unmarshal(...)` sur les métadonnées stockées. Corruption en base
→ record retourné avec metadata nil sans aucun signal.  
**Fix :** `log.Printf("[vector] metadata unmarshal warning: …")`.

---

### C5 · `context.Background()` hardcodé dans sqlite_backend ✅ FIXÉ
**Fichier :** `internal/runtime/state/sqlite_backend.go:103,140,162`  
**Problème :** `DeleteSession`, `AppendTranscriptEntries`, `ReplaceTranscript` utilisent
`context.Background()` → impossible d'annuler une opération longue ou d'enforcer un
timeout depuis l'appelant.  
**Fix (pragmatique) :** `context.WithTimeout(context.Background(), N*time.Second)`.  
**Fix (complet, LATER) :** Ajouter `ctx context.Context` sur l'interface `Backend` et
propager jusqu'aux callers (refactor C5-full ci-dessous).

---

### M1 · Dimension des embeddings jamais validée ✅ FIXÉ
**Fichier :** `internal/rag/embedder/embedder.go`  
**Problème :** Le nombre de vecteurs retournés est vérifié mais pas leur dimension. Si le
provider renvoie 768 dims au lieu de 1536, les vecteurs sont stockés silencieusement et
les recherches donnent des résultats incohérents.  
**Fix :** Vérifier `len(out[0])` contre la dimension attendue.

---

### M2 · Fallback FTS5 → LIKE non loggué
**Fichier :** `internal/runtime/state/sqlite_backend.go` — `SearchTranscriptsByContent`  
**Problème :** La dégradation FTS5 → LIKE scan est silencieuse. Chaque search fait un
full-scan O(n) sans que l'opérateur le sache.  
**Statut :** À adresser avec C5-full (même fichier).

---

## LATER — Issues communauté open-source

Ces items sont adaptés pour être des GitHub Issues avec label `good first issue` ou
`help wanted` selon la complexité.

---

### L-A · Propagation complète de `ctx` sur l'interface `Backend`
**Complexité :** Élevée (~15 fichiers)  
**Fichiers :** `internal/runtime/state/backend.go`, `sqlite_backend.go`,
`filesystem_backend.go`, `store.go`, `sync.go`, `engine/session.go`, `pkg/sdk/client.go`,
`cmd/cli/sessions.go`  
**Description :** L'interface `Backend` (SaveSession, LoadSession, DeleteSession,
AppendTranscriptEntries, ReplaceTranscript) n'accepte pas de `ctx context.Context`. Cela
empêche toute propagation de timeout ou d'annulation depuis les callers.  
**Label :** `refactor`, `good first issue` (bien documenté, impact clair)

---

### L-B · Nettoyage des fichiers de sortie des tasks
**Complexité :** Faible  
**Fichier :** `internal/runtime/tasks/manager.go`  
**Description :** Les fichiers `.output` et `.exit` générés par les tasks ne sont jamais
supprimés — accumulation disque infinie. Ajouter une TTL (ex : 7 jours) et un GC
périodique.  
**Label :** `bug`, `good first issue`

---

### L-C · Goroutines sans shutdown propre dans `internal/agent/`
**Complexité :** Moyenne  
**Fichiers :** `internal/agent/runner.go:195,322`, `internal/agent/events.go:142,332`  
**Description :** Les goroutines créées avec `context.WithCancel(context.Background())`
ne sont pas arrêtées à la fermeture du client. Sur des runs longues, cela accumule des
goroutines en fuite.  
**Label :** `bug`, `help wanted`

---

### L-D · Rebuild automatique si index HNSW corrompu
**Complexité :** Moyenne  
**Fichier :** `internal/vector/hnsw_store.go`  
**Description :** Si le fichier `.hnsw` est corrompu, `LoadSavedGraph` retourne une
erreur qui bloque le namespace entier pour toute la durée du process. Ajouter une logique
de reconstruction depuis le fichier `.meta.json` (les vecteurs ne sont pas dans le meta,
donc reconstruction complète nécessite re-embedding — à documenter).  
**Label :** `enhancement`, `help wanted`

---

### L-E · Race window entre SaveSession et AppendTranscriptEntries
**Complexité :** Moyenne  
**Fichier :** `internal/runtime/state/sqlite_backend.go`  
**Description :** Les deux opérations ne sont pas dans la même transaction SQL. Si une
suppression concurrente intervient entre les deux, la FK CASCADE supprime les rows de
session_metadata avant que le transcript soit écrit, laissant des entries orphelines.  
**Fix :** Wrapper les deux opérations dans une transaction explicite.  
**Label :** `bug`, `help wanted`

---

### L-F · Validation de la dimension vectorielle à l'Upsert
**Complexité :** Faible  
**Fichiers :** `internal/vector/*.go` (tous backends)  
**Description :** Aucun backend ne valide que toutes les entrées d'un même namespace ont
la même dimension. Des vecteurs de dimensions mixtes sont acceptés silencieusement et
corrompent les résultats de search.  
**Label :** `bug`, `good first issue`

---

### L-G · Fallback pgvector `searchHybrid` → `searchVector` sans log
**Complexité :** Très faible  
**Fichier :** `internal/vector/pgvector_store.go`  
**Description :** En cas d'erreur `ts_rank`, le fallback vers pure-vector search est
silencieux. Ajouter un `log.Printf` avant le fallback.  
**Label :** `good first issue`

---

### L-H · Config.Validate() au chargement
**Complexité :** Moyenne  
**Fichier :** `pkg/config/config.go`  
**Description :** La config est chargée sans validation d'incohérences internes. Ex :
`VectorBackend=pgvector` avec une DB SQLite → erreur seulement au premier query.
`RAG_EMBEDDING_URL` set mais `RAG_EMBEDDING_MODEL` vide → erreur au premier appel RAG.  
**Label :** `enhancement`, `good first issue`

---

### L-I · Métriques de latence pour les opérations store
**Complexité :** Moyenne  
**Fichiers :** `internal/vector/*`, `internal/runtime/state/*`  
**Description :** Aucune métrique exposée (Prometheus ou autre) pour les latences de
search, append, ou compaction. Impossible de détecter des régressions de performance en
production.  
**Label :** `enhancement`, `observability`

---

### L-J · Tests manquants — `internal/db/`
**Complexité :** Moyenne  
**Fichiers :** `internal/db/*.go`  
**Description :** Zéro fichier de test dans le package DB. Migrations, triggers FTS5,
cascade deletes, deduplication session_files — aucune couverture.  
Priorité : idempotence des migrations (run twice → même schéma).  
**Label :** `testing`, `good first issue`

---

### L-K · Tests manquants — RAG end-to-end
**Complexité :** Moyenne  
**Fichier :** `internal/rag/`  
**Description :** Pas de test du flow complet `Ingest → chunk → embed → store → Search`.
La couverture est partielle : seules les unités individuelles sont testées.  
**Label :** `testing`, `help wanted`

---

### L-L · Tests manquants — agent runner + task manager
**Complexité :** Élevée  
**Fichiers :** `internal/agent/runner.go`, `internal/runtime/tasks/manager.go`  
**Description :** Tests d'intégration pour le cycle complet task create → run → complete
→ output absent.  
**Label :** `testing`, `help wanted`

---

### L-M · `PRAGMA integrity_check` au démarrage
**Complexité :** Très faible  
**Fichier :** `internal/db/db.go`  
**Description :** Aucune vérification d'intégrité SQLite au démarrage. Une corruption de
la table FTS5 ou du WAL peut produire des erreurs opaques difficiles à diagnostiquer.
Ajouter un `PRAGMA integrity_check(1)` (quick check) lors de `Open` en mode SQLite.  
**Label :** `reliability`, `good first issue`

---

## Résumé des statuts

| ID | Titre | Bucket | Statut |
|----|-------|--------|--------|
| C1 | Session leak task manager | NOW | ✅ Fixé |
| C2 | HNSW partial write | NOW | ✅ Fixé |
| C3 | FTS5 silent errors | NOW | ✅ Fixé |
| C4 | JSON unmarshal metadata silent | NOW | ✅ Fixé |
| C5 | context.Background() sqlite_backend | NOW | ✅ Fixé (timeout) |
| M1 | Dimension embeddings non validée | NOW | ✅ Fixé |
| M2 | Fallback FTS5→LIKE non loggué | NOW | Lié à C5 |
| L-A | ctx complet sur interface Backend | LATER | Issue |
| L-B | Nettoyage files tasks | LATER | Issue |
| L-C | Goroutines agent sans shutdown | LATER | Issue |
| L-D | Rebuild HNSW sur corruption | LATER | Issue |
| L-E | Race SaveSession + AppendTranscript | LATER | Issue |
| L-F | Validation dimension vectorielle | LATER | Issue |
| L-G | pgvector hybrid fallback log | LATER | Issue |
| L-H | Config.Validate() | LATER | Issue |
| L-I | Métriques latence store | LATER | Issue |
| L-J | Tests internal/db | LATER | Issue |
| L-K | Tests RAG end-to-end | LATER | Issue |
| L-L | Tests agent + task manager | LATER | Issue |
| L-M | PRAGMA integrity_check startup | LATER | Issue |
