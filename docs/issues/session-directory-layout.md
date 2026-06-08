# Session-Scoped Directory Layout

## Problème actuel

Le répertoire de travail `~/.config/nexus-cli/` est un patchwork de conventions disparates :

```
~/.config/nexus-cli/
├── nexus.yaml
├── secret.key
├── data/
│   ├── nexus.db          # SQLite : sessions, transcripts, credentials
│   └── hnsw/             # index vectoriel
├── plans/
│   └── {slug}.md         # slug aléatoire, pas lié au session_id dans le nom
├── storage/
│   └── artifacts/
│       └── browser/
│           ├── screenshots/{session_id}/{page_id}/{date}/{ts-file}
│           └── downloads/{session_id}/{page_id}/{date}/{ts-file}
├── logs/
│   └── cli.log
├── cache/
└── tmp/
    ├── tasks/
    └── bash-tasks/
```

**Points de friction concrets :**

1. **Plans non traçables** — les fichiers de plan utilisent un slug aléatoire (`algorithm-spectrum.md`). Le lien slug↔session_id n'existe qu'en mémoire vive. Si le processus redémarre, on ne peut plus retrouver le plan d'une session passée.

2. **Suppression en plusieurs étapes** — supprimer une session force à :
   - lister les artifacts par préfixe dans le store (`artifacts/browser/screenshots/{id}/…`)
   - supprimer chaque fichier un par un
   - nettoyer le slug cache et le fichier plan séparément
   - supprimer la rangée SQLite

3. **Chemins hardcodés éparpillés** — `os.UserHomeDir()` est appelé à 8 endroits différents dans le code ; `~/.config/nexus-cli` apparaît sous plusieurs formes dans `main.go`, `config.go`, `tui.go`, `credentials.go`, `plan.go`, etc.

4. **Pas de log par session** — un seul `cli.log` global mélange toutes les sessions ; déboguer une session précise oblige à filtrer manuellement.

5. **Nom de répertoire ambigu** — `nexus-cli` désignait l'outil CLI, mais l'application est maintenant un TUI complet. `nexus-tui` est plus précis et évite les collisions avec le backend (`~/.config/nexus`).

---

## Proposition

### Nouveau répertoire racine

| Plateforme | Chemin par défaut            |
|------------|------------------------------|
| Linux      | `~/.config/nexus-tui/`       |
| macOS      | `~/.config/nexus-tui/`       |
| Windows    | `%APPDATA%\nexus-tui\`       |

La variable d'environnement `NEXUS_RUNTIME_ROOT` continue de prendre la priorité pour les usages avancés.

### Arborescence cible

```
~/.config/nexus-tui/
├── config.yaml               # configuration utilisateur
├── secret.key                # clé AES-256 (mode 0600)
├── nexus.db                  # SQLite : metadata sessions, credentials, transcripts
├── logs/
│   └── app.log               # log applicatif global (démarrage, erreurs critiques)
└── sessions/
    └── {session_id}/
        ├── images/           # screenshots browser, images générées
        ├── plans/            # fichiers de plan mode ({slug}.md ou plan.md)
        ├── tools/            # fichiers téléchargés, outputs d'outils, metadata non-DB
        └── session.log       # log spécifique à cette session
        └── permissions.json  # Save permissions per tools during the session
```

### Principes

- **Tout ce qui est propre à une session vit dans `sessions/{id}/`** — un seul `os.RemoveAll` suffit pour supprimer toutes les données physiques d'une session.
- **La DB reste la source de vérité pour les métadonnées** — les chemins physiques en sont déduits via les fonctions du package `runtimepath`, jamais hardcodés.
- **Le package `runtimepath` fournit les fonctions, l'application gère l'initialisation** — les packages internes prennent les chemins en entrée, ils ne font pas de découverte de répertoire eux-mêmes.
- **La DB SQLite passe à la racine** (`nexus.db` au lieu de `data/nexus.db`) — simplification sans impact fonctionnel.

---

## Changements par couche

### 1. `pkg/runtimepath` — nouvelles fonctions

Ajouter les accesseurs pour les répertoires session-scoped :

```go
// Répertoires globaux
func DBPath(root string) string         { return Join(root, "nexus.db") }
func EncryptionKeyPath(root string) string { return Join(root, "secret.key") }
func AppLogPath(root string) string     { return Join(root, "logs", "app.log") }

// Répertoires par session
func SessionsDir(root string) string    { return Join(root, "sessions") }
func SessionDir(root, sessionID string) string {
    return filepath.Join(SessionsDir(root), sessionID)
}
func SessionImagesDir(root, sessionID string) string {
    return filepath.Join(SessionDir(root, sessionID), "images")
}
func SessionPlansDir(root, sessionID string) string {
    return filepath.Join(SessionDir(root, sessionID), "plans")
}
func SessionToolsDir(root, sessionID string) string {
    return filepath.Join(SessionDir(root, sessionID), "tools")
}
func SessionLogPath(root, sessionID string) string {
    return filepath.Join(SessionDir(root, sessionID), "session.log")
}
```

Les fonctions existantes (`PlansDir`, `StorageDir`, `BackendDBPath`) restent présentes pendant la période de migration puis sont dépréciées.

### 2. `cmd/cli/appdir/appdir.go` — nouveau package (côté applicatif)

Ce package est la **seule source de vérité côté application** pour les chemins. Il ne doit pas être importé par les packages internes.

```go
package appdir

// Root retourne le répertoire racine de l'application, résolu via NEXUS_RUNTIME_ROOT
// ou la convention plateforme (Linux/macOS : ~/.config/nexus-tui, Windows : %APPDATA%\nexus-tui).
func Root() string

// EnsureAppDirs crée tous les répertoires applicatifs nécessaires au démarrage.
// Idempotent. À appeler une seule fois dans main().
func EnsureAppDirs() error

// EnsureSessionDir crée sessions/{id}/ et ses sous-répertoires (images, plans, tools).
// À appeler quand une nouvelle session démarre.
func EnsureSessionDir(sessionID string) error

// DeleteSessionDir supprime récursivement sessions/{id}/.
// Utilisé par DeleteSession — un seul appel couvre tous les fichiers physiques.
func DeleteSessionDir(sessionID string) error

// Accesseurs directs (délèguent à runtimepath)
func DBPath() string
func EncryptionKeyPath() string
func AppLogPath() string
func SessionDir(sessionID string) string
func SessionImagesDir(sessionID string) string
func SessionPlansDir(sessionID string) string
func SessionToolsDir(sessionID string) string
func SessionLogPath(sessionID string) string
```

### 3. Stockage des artifacts browser

**Avant :** `storage/artifacts/browser/screenshots/{session_id}/{page_id}/{date}/{ts}-screenshot.png`

**Après :** `sessions/{session_id}/images/{page_id}/{date}/{ts}-screenshot.png`

La fonction `ScreenshotKey` dans `storage/keys.go` est mise à jour pour utiliser `appdir.SessionImagesDir(sessionID)` comme base. De même pour les downloads → `SessionToolsDir`.

### 4. Fichiers de plan

**Avant :** `plans/{random-slug}.md` (lien slug↔session_id uniquement en mémoire)

**Après :** `sessions/{session_id}/plans/plan.md` (ou `sessions/{session_id}/plans/{slug}.md` si plusieurs plans par session)

`GetPlanFilePath` dans `internal/modes/execution/plan.go` utilise `appdir.SessionPlansDir(sessionID)` au lieu de `planCache.GetDirectory()`. Le slug reste utile pour nommer les fichiers quand plusieurs plans coexistent dans une session, mais la session ID est désormais directement dans le chemin.

### 5. Suppression d'une session

**Avant :**
```go
// 1. Lister les artifacts par préfixe
store.List(ctx, {Prefix: "artifacts/browser/screenshots/" + id})
// 2. Supprimer chaque fichier
for _, ref := range refs { store.Delete(ctx, ref.Key) }
// 3. Nettoyer les plans
os.Remove(GetPlanFilePath(sessionID, nil))
execution.ClearState(sessionID)
execution.ClearPlanSlug(sessionID)
// 4. Supprimer la rangée DB
store.DeleteSession(sessionID)
```

**Après :**
```go
// 1. Supprimer tous les fichiers physiques d'un coup
appdir.DeleteSessionDir(string(sessionID))   // os.RemoveAll(sessions/{id}/)
// 2. Supprimer la rangée DB (cascade : transcripts, checkpoints, session_files)
store.DeleteSession(sessionID)
```

### 6. Logs par session

À chaque démarrage de session, un `log.Logger` est créé pointant vers `sessions/{id}/session.log`. Les erreurs spécifiques à la session (tool failures, context errors, provider errors) y sont écrites en plus du log global.

---

## Migration des données existantes

Les installations existantes gardent leur répertoire `~/.config/nexus-cli/` intact. La migration n'est pas destructive :

1. **Première utilisation** : si `~/.config/nexus-tui/` n'existe pas mais `~/.config/nexus-cli/` existe, afficher un message proposant la migration.
2. **Migration opt-in** : `nexus migrate` (ou un flag au démarrage) copie `nexus.db`, `secret.key`, `config.yaml` vers le nouveau répertoire. Les anciens artifacts restent dans l'ancien emplacement (on ne les déplace pas — trop risqué, trop lent).
3. **Période de cohabitation** : `NEXUS_RUNTIME_ROOT=~/.config/nexus-cli` permet de rester sur l'ancien chemin sans changement.

---

## Phases d'implémentation

| Phase | Contenu | Impact |
|-------|---------|--------|
| 1 | Ajouter les fonctions session-scoped à `pkg/runtimepath` | aucun — nouvelles fonctions |
| 2 | Créer `cmd/cli/appdir/appdir.go` avec `Root()`, `EnsureAppDirs()`, `EnsureSessionDir()`, `DeleteSessionDir()` | aucun — nouveau package |
| 3 | Renommer le répertoire racine : `nexus-cli` → `nexus-tui` dans `main.go` + valeur par défaut Windows | breaking pour les users existants → faire en dernier avec migration |
| 4 | Migrer le stockage des artifacts browser vers `sessions/{id}/images/` et `sessions/{id}/tools/` | modifier `storage/keys.go` + `storage/artifacts.go` |
| 5 | Migrer les plans vers `sessions/{id}/plans/` | modifier `internal/modes/execution/plan.go` + `cache.go` |
| 6 | Simplifier `DeleteSession` : remplacer par `appdir.DeleteSessionDir` + `store.DeleteSession` | remplace le code de nettoyage actuel |
| 7 | Ajouter le log par session | nouveau : `sessionLogger` dans `engine/session.go` ou `agent/runner.go` |
| 8 | Déprécier et supprimer les anciennes fonctions `runtimepath` (PlansDir, StorageDir, BackendDBPath legacy) | nettoyage |

Les phases 1 et 2 peuvent être faites immédiatement. Les phases 3–5 sont le gros du travail mais sont indépendantes entre elles. Les phases 6–8 découlent naturellement.

---

## Ce qui ne change pas

- La structure de la base SQLite (`nexus.db`) — seul le chemin change.
- Le format des sessions, transcripts, metadata en DB.
- Le système de stockage S3 (aucun impact — le S3 store reste avec ses propres clés).
- L'interface `ArtifactStore` et la logique de GC.
- Le `runtimepath.EnvRuntimeRoot` comme mécanisme d'override.
