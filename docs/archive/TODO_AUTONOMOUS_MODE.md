# MODE AUTONOME — DOCUMENT ARCHIVÉ / SPÉCULATIF

> **Statut : archivé.** Ce document décrit une vision future qui n'est pas encore dans le scope actif du produit.
> La direction active est le renforcement du runtime mono-run (voir `docs/architecture.md`).
> Ne pas utiliser ce document pour guider l'architecture en cours.

---

# MODE AUTONOME - DOCUMENTATION FUTURE (archivée)

## 📋 CONCEPT

Le mode **Autonomous** est un mode d'exécution spécial pour Nexus Engine qui permet à un agent de travailler de manière entièrement autonome sans validation utilisateur.

## 🎯 CAS D'USAGE

### **Scénarios idéaux :**
- **Refactorisation complète** : "Refactorise toute cette codebase selon ces patterns"
- **Migration de projet** : "Migre ce projet vers la nouvelle version de l'API"
- **Création de module** : "Crée un nouveau module de gestion d'utilisateurs avec ces specs"
- **Tests automatisés** : "Génère des tests pour tous les endpoints de l'API"
- **Documentation** : "Génère la documentation complète de ce projet"

### **Environnements cibles :**
- ✅ **Environnements de développement** isolés
- ✅ **Containers Docker** temporaires
- ✅ **VMs de test**
- ✅ **Systèmes CI/CD** avec rollback automatique
- ❌ **Production** (jamais !)
- ❌ **Données sensibles** (jamais !)

## 🏗️ ARCHITECTURE

### **Constantes à ajouter :**
```go
const (
    ExecutionModeAutonomous ExecutionMode = "autonomous"
)
```

### **Comportement du mode :**

#### **1. Accès illimité**
```go
// Dans internal/execution/pipeline.go
if modes.IsAutonomousModeString(state.toolCtx.ExecutionMode) {
    // Bypass tous les blocages de sécurité
    // Autoriser tous les outils sans restriction
    // Y compris bash, file_edit, etc.
}
```

#### **2. Aucune validation**
- **Pas de demande de confirmation**
- **Pas de AskUserQuestion**
- **Pas de hooks de permission bloquants**
- **Auto-décision continue/stop**

#### **3. Limites de sécurité**
```go
type AutonomousConfig struct {
    MaxDuration        time.Duration `json:"max_duration"`        // Ex: 1 heure max
    MaxTokenUsage      int           `json:"max_token_usage"`      // Ex: 100k tokens
    MaxFileModifications int         `json:"max_file_modifications"` // Ex: 100 fichiers
    AllowedPaths       []string      `json:"allowed_paths"`        // Paths autorisés
    BlockedPaths       []string      `json:"blocked_paths"`        // Paths interdits (ex: /etc, /system)
    EnableSnapshot     bool          `json:"enable_snapshot"`      // Snapshot avant/après
}
```

#### **4. Audit et logging**
```go
// Logging complet de toutes les actions
type AutonomousAudit struct {
    SessionID       string    `json:"session_id"`
    StartTime       time.Time `json:"start_time"`
    EndTime         time.Time `json:"end_time"`
    ToolsCalled     []string  `json:"tools_called"`
    FilesModified   []string  `json:"files_modified"`
    CommandsExecuted []string `json:"commands_executed"`
    TokenUsage      int       `json:"token_usage"`
    Success         bool      `json:"success"`
    Error           string    `json:"error,omitempty"`
    SnapshotBefore  string    `json:"snapshot_before,omitempty"`
    SnapshotAfter   string    `json:"snapshot_after,omitempty"`
}
```

## 🔧 IMPLEMENTATION FUTURE

### **Fichiers à créer :**

#### **1. Modes constants**
```go
// internal/modes/execution.go
const (
    ExecutionModeAutonomous ExecutionMode = "autonomous"
)

func IsAutonomousMode(mode ExecutionMode) bool {
    return mode == ExecutionModeAutonomous
}

func IsAutonomousModeString(mode string) bool {
    return mode == string(ExecutionModeAutonomous)
}
```

#### **2. State management**
```go
// internal/modes/execution/autonomous.go
type AutonomousState struct {
    IsActive           bool
    Config             *AutonomousConfig
    Audit              *AutonomousAudit
    FileModifications  int
    TokenUsage         int
    CommandsExecuted   int
    StartTime          time.Time
    MaxDurationExceeded bool
}

func EnterAutonomousMode(sessionID types.SessionID, config *AutonomousConfig)
func ExitAutonomousMode(sessionID types.SessionID)
func ShouldContinueAutonomous(sessionID types.SessionID) bool
```

#### **3. Tools d'entrée/sortie**
```go
// internal/tools/autonomous/enterAutonomousMode.go
type EnterAutonomousModeTool struct {
    config *AutonomousConfig
}

// internal/tools/autonomous/exitAutonomousMode.go
type ExitAutonomousModeTool struct {
    generateReport bool
}
```

#### **4. Pipeline integration**
```go
// internal/execution/pipeline.go
if modes.IsAutonomousModeString(state.toolCtx.ExecutionMode) {
    // Bypass tous les blocages normaux
    // Logging audit complet
    // Vérification des limites
    if shouldBlock := checkAutonomousLimits(sessionID, toolUse.Name); shouldBlock {
        return blockedResult("Autonomous mode limits exceeded")
    }
    recordAutonomousAction(sessionID, toolUse.Name)
}
```

## ⚠️ SÉCURITÉ

### **Risques identifiés :**
1. **Destruction de données** : Modifications sans validation
2. **Boucles infinies** : Agent qui ne s'arrête jamais
3. **Resource exhaustion** : CPU, mémoire, disque
4. **Security breaches** : Accès non autorisé
5. **Cost escalation** : Tokens illimités

### **Mitigations :**
1. **Isolation stricte** : Containers séparés, réseaux isolés
2. **Limites strictes** : Durée, tokens, fichiers, commandes
3. **Snapshots automatiques** : Avant/après pour rollback
4. **Kill switch** : Arrêt d'urgence immédiat
5. **Audit complet** : Traçabilité totale
6. **Whitelist paths** : Seuls certains dossiers accessibles

## 🚦 SIGNALS DE DANGER

### **Quand NE PAS utiliser :**
- ❌ **En production**
- ❌ **Sur des données sensibles**
- ❌ **Sur des systèmes critiques**
- ❌ **Sans rollback possible**
- ❌ **Sans monitoring actif**
- ❌ **Sans tests préalables**

### **Signaux d'arrêt d'urgence :**
- 🚨 **Resource limits exceeded**
- 🚨 **Unexpected errors patterns**
- 🚨 **Access to blocked paths**
- 🚨 **Suspicious activity detected**
- 🚨 **User intervention requested**

## 📊 MÉTRIQUES

### **À surveiller :**
- **Taux de succès** : Tasks completed / Total tasks
- **Durée moyenne** : Temps de completion
- **Consommation ressources** : CPU, mémoire, disque
- **Nombre d'erreurs** : Types et fréquence
- **Rollback rate** : Taux de restaurations

## 🔄 WORKFLOW RECOMMANDÉ

### **Processus sécurisé :**
1. **Préparation** : Créer environnement isolé
2. **Configuration** : Définir limites strictes
3. **Snapshot** : Sauvegarder état initial
4. **Exécution** : Lancer mode autonome
5. **Monitoring** : Surveiller en temps réel
6. **Validation** : Vérifier résultats
7. **Rollback** : Si nécessaire, restaurer snapshot
8. **Audit** : Analyser logs et métriques

## 🎓 EXEMPLES D'USAGE

### **Example 1 : Refactorisation**
```bash
# Créer environnement isolé
docker run -it --rm -v $(pwd):/app nexus-autonomous-env

# Configuration autonome
cat > autonomous_config.json << EOF
{
  "max_duration": "30m",
  "max_file_modifications": 50,
  "allowed_paths": ["/app/src"],
  "blocked_paths": ["/app/.git", "/app/node_modules"],
  "enable_snapshot": true
}
EOF

# Lancer le mode autonome
nexus autonomous --config autonomous_config.json \
  "Refactorise le code pour utiliser async/await au lieu de callbacks"
```

### **Example 2 : Migration API**
```bash
# Configuration avec snapshot
cat > autonomous_config.json << EOF
{
  "max_duration": "1h",
  "max_file_modifications": 100,
  "allowed_paths": ["/project"],
  "enable_snapshot": true,
  "snapshot_before": true,
  "snapshot_after": true
}
EOF

# Lancer la migration
nexus autonomous --config autonomous_config.json \
  "Migre toute l'API vers REST v2 avec les nouveaux endpoints"
```

## 🔮 FUTUR

### **Améliorations possibles :**
- **Machine learning** : Prédire les risques avant exécution
- **Sandbox avancé** : Virtualisation complète
- **Multi-agent autonomous** : Plusieurs agents autonomes coordonnés
- **Auto-healing** : Détection et correction automatique d'erreurs
- **Progressive rollout** : Déploiement progressif en production

## ⚡ PRIORITÉ D'IMPLÉMENTATION

### **Phase 1 : Infrastructure (Mois 1-2)**
- [ ] Constantes et types de base
- [ ] Gestion d'état
- [ ] Configuration et limites
- [ ] Système d'audit

### **Phase 2 : Tools (Mois 2-3)**
- [ ] EnterAutonomousMode tool
- [ ] ExitAutonomousMode tool
- [ ] Monitoring et kill switch
- [ ] Snapshot system

### **Phase 3 : Integration (Mois 3-4)**
- [ ] Pipeline integration
- [ ] Security layers
- [ ] Resource monitoring
- [ ] Error handling

### **Phase 4 : Testing (Mois 4-5)**
- [ ] Tests unitaires complets
- [ ] Tests d'intégration
- [ ] Tests de sécurité
- [ ] Tests de performance

### **Phase 5 : Documentation (Mois 5-6)**
- [ ] User documentation
- [ ] Developer documentation
- [ ] Security guidelines
- [ ] Best practices

---

**Note importante** : Ce mode est puissant mais dangereux. Utilisation seulement dans des environnements contrôlés et avec une compréhension claire des risques.

**Status** : 📋 **CONCEPTUAL** - En attente d'approbation et planification détaillée
