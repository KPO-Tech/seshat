# Seshat TUI — Test Prompts

Chaque section cible un groupe de tools précis.
Toujours exécuter les appels dans l'ordre indiqué, sans commenter entre les étapes.

---

## 1. FILE SYSTEM TOOLS

Valide : `write_file`, `edit_file`, `apply_patch`, `create_directory`, `get_file_metadata`, `read_file`, `remove_file`.

```
Exécute ces tools dans cet ordre exact :

1. create_directory — crée /tmp/tui_test
   → attendu : header seul, aucun texte sous la bulle

2. write_file — écris /tmp/tui_test/hello.go avec ce contenu :
   package main

   import "fmt"

   func main() {
       fmt.Println("hello world")
   }
   → attendu : permission panel s'ouvre avec diff (vide → contenu)
   → attendu après approbation : bulle chat affiche le diff coloré

3. read_file — lis /tmp/tui_test/hello.go
   → attendu : header seul, aucun contenu affiché

4. get_file_metadata — inspecte /tmp/tui_test/hello.go
   → attendu : header seul, aucun JSON affiché

5. edit_file — dans /tmp/tui_test/hello.go, remplace :
       fmt.Println("hello world")
   par :
       fmt.Println("hello, nexus!")
   → attendu : permission panel avec extrait old → new coloré
   → attendu après approbation : bulle chat avec diff rouge/vert

6. write_file — remplace /tmp/tui_test/hello.go avec ce contenu :
   package main

   import "fmt"

   func add(a, b int) int { return a + b }

   func main() {
       fmt.Println("hello, nexus!")
       fmt.Println(add(1, 2))
   }
   → attendu : permission panel montre le diff complet

7. apply_patch — applique un patch qui :
   - modifie /tmp/tui_test/hello.go (change "hello, nexus!" en "hello, world!")
   - crée /tmp/tui_test/readme.txt avec le contenu "test file"
   → attendu : bulle chat affiche fichiers modifiés avec indicateurs +/~

8. remove_file — supprime /tmp/tui_test/readme.txt
   → attendu : header seul, aucun texte sous la bulle

9. remove_file (récursif) — supprime /tmp/tui_test avec recursive: true
   → attendu : header seul, aucun texte sous la bulle

Ne fais rien d'autre après l'étape 9.
```

---

## 2. SEARCH TOOLS

Valide : `glob`, `grep`, `list_directory`.

```
Exécute ces tools dans cet ordre exact :

1. glob — pattern: "**/*.go"
   Puis glob — pattern: "**/*.md"
   → attendu : header = pattern + "N files"
   → attendu : body = liste de chemins (max 8, puis +N more)

2. grep — pattern: "func New" dans le répertoire courant
   Puis grep — pattern: "TODO" avec include: "*.go"
   → attendu : header = pattern + "N files · M matches"
   → attendu : body = lignes "file:numéro:extrait" (max 6, puis +N more)

3. list_directory — path: "."
   Puis list_directory — path: "internal"
   → attendu : header = path + "N items"
   → attendu : body = liste dirs/ + fichiers avec taille

Vérifie aussi expand/collapse (espace ou clic) sur chaque bulle.
Ne fais rien d'autre après l'étape 3.
```

---

## 3. WEB TOOLS

Valide : `web_fetch`, `web_search`.

```
Exécute ces tools dans cet ordre exact :

1. web_fetch — url: "https://go.dev/doc/effective_go"
   → attendu : header = URL complète
   → attendu : body markdown rendu (headings, listes, liens)
   → attendu : body tronqué avec "+N more lines" et expand/collapse

2. web_fetch — url: "https://github.com/charmbracelet/bubbletea/blob/main/README.md"
   → attendu : même comportement, URL différente

3. web_search — query: "bubbletea go tui framework tutorial 2024"
   → attendu : header = la query
   → attendu : body avec les résultats de recherche

4. web_search — query: "ripgrep performance vs grep benchmark"
   → attendu : même comportement, query différente

Ne fais rien d'autre après l'étape 4.
```

---

## 4. BASH

Valide : header multi-ligne, body numéroté, JSON highlighting, `(no output)`, troncature.

```
Exécute ces commandes bash dans cet ordre exact :

1. bash — echo "hello from nexus"
   → attendu : header = commande sur une ligne, body = "1  hello from nexus"

2. bash — commande multi-ligne :
   for i in 1 2 3; do
     echo "item $i"
   done
   → attendu : header = "for i in 1 2 3; do (+2 lines)"
   → attendu : body = lignes numérotées "1  item 1", "2  item 2", "3  item 3"

3. bash — echo '{"name": "nexus", "version": 1, "active": true}'
   → attendu : body avec coloration syntaxique JSON

4. bash — true
   → attendu : body = "(no output)" en style discret

5. bash — seq 1 30
   → attendu : body = 10 premières lignes + "… (20 lines hidden) [click or space to expand]"
   → après expand : toutes les 30 lignes visibles

Ne fais rien d'autre après l'étape 5.
```

---

## 5. NOTEBOOK TOOLS

Valide : `notebook_edit`, `notebook_create`, `notebook_write`.

### 5a. notebook_create — header seul au succès, erreur si fichier existe

```
Travaille uniquement dans /tmp. Exécute ces appels dans cet ordre exact :

1. notebook_create — créer un notebook vide :
     notebook_path: "/tmp/test_nb.ipynb"
     kernel: "python3"
     language: "python"
   → attendu : header "✓ Create Notebook  ~/test_nb.ipynb" (icône succès, aucun body)

2. notebook_create — retenter sur le même fichier :
     notebook_path: "/tmp/test_nb.ipynb"
   → attendu : header avec icône erreur + body = "file already exists — use notebook_write to overwrite"

3. notebook_create — avec cellules initiales :
     notebook_path: "/tmp/test_nb2.ipynb"
     kernel: "python3"
     cells:
       - cell_type: "markdown"
         source: "# Mon notebook de test"
       - cell_type: "code"
         source: "print('hello')"
   → attendu : header "✓ Create Notebook  ~/test_nb2.ipynb" (header seul, aucun body)

Ne fais rien d'autre après l'étape 3.
```

### 5b. notebook_write — create vs overwrite, preview code

```
Travaille uniquement dans /tmp. Exécute ces appels dans cet ordre exact :

1. notebook_write — créer un nouveau notebook (fichier absent) :
     notebook_path: "/tmp/test_write.ipynb"
     kernel: "python3"
     cells:
       - cell_type: "code"
         source: |
           import numpy as np
           arr = np.array([1, 2, 3, 4, 5])
           print(arr.mean())
       - cell_type: "markdown"
         source: "## Résultats"
       - cell_type: "code"
         source: "print('done')"
   → attendu : header "✓ Write Notebook  ~/test_write.ipynb · create · 3 cells"
   → attendu : body = premier cell code Python colorisé (import numpy...)

2. notebook_write — écraser le même fichier (overwrite) :
     notebook_path: "/tmp/test_write.ipynb"
     kernel: "python3"
     cells:
       - cell_type: "code"
         source: |
           import pandas as pd
           df = pd.read_csv('data.csv')
           df.describe()
   → attendu : header "✓ Write Notebook  ~/test_write.ipynb · overwrite · 1 cells"
   → attendu : body = code Python de la nouvelle cellule

Ne fais rien d'autre après l'étape 2.
```

### 5c. notebook_edit — replace, insert, delete

```
Travaille uniquement dans /tmp. Utilise /tmp/test_nb.ipynb créé à la section 5a.
Exécute ces appels dans cet ordre exact :

1. notebook_edit — insérer une cellule code (insert) :
     notebook_path: "/tmp/test_nb.ipynb"
     cell_type: "code"
     edit_mode: "insert"
     new_source: |
       import pandas as pd
       df = pd.DataFrame({"x": [1, 2, 3], "y": [4, 5, 6]})
       print(df.describe())
   → attendu : header "Notebook Edit  ~/test_nb.ipynb  insert"
   → attendu : body = code Python colorisé

2. notebook_edit — remplacer une cellule markdown (replace) :
     notebook_path: "/tmp/test_nb.ipynb"
     cell_id: "cell-0"
     cell_type: "markdown"
     edit_mode: "replace"
     new_source: |
       # Analyse exploratoire
       Ce notebook explore un jeu de données simple.
   → attendu : header "Notebook Edit  ~/test_nb.ipynb  cell cell-0 · replace"
   → attendu : body = markdown colorisé

3. notebook_edit — supprimer une cellule (delete) :
     notebook_path: "/tmp/test_nb.ipynb"
     cell_id: "cell-1"
     edit_mode: "delete"
   → attendu : header "Notebook Edit  ~/test_nb.ipynb  cell cell-1 · delete"
   → attendu : aucun body (delete = header seul)

Ne fais rien d'autre après l'étape 3.
```

---

## 6. AGENT TOOLS — LIVE TAIL

Valide : `spawn_agent`, `send_agent_message`, `wait_agent`, `close_agent`, `list_agents`.
Vérifie surtout le **live tail** : pendant l'exécution du subagent, la section "dernier tool complété" doit s'afficher en plein rendu sous l'arbre compact.

### 6a. Agent inline (tool `agent`) — live tail visible

```
Utilise explicitement le tool "agent" (sous-agent inline) pour exécuter la tâche suivante :

  Explore le répertoire examples/ du projet courant.
  1. Liste son contenu avec list_directory
  2. Lis le fichier README.md à la racine avec read_file
  3. Lance une recherche grep avec pattern "func New" dans examples/
  4. Retourne un résumé de 3 lignes de ce que tu as trouvé

Le but est de générer au moins 4 tool calls imbriqués pour tester l'affichage live.

→ attendu pendant l'exécution :
  - Arbre compact en haut : chaque tool apparaît ligne par ligne (✓ ou ● icon)
  - Section live SOUS l'arbre : le dernier tool complété affiché en plein rendu
    (ex: si read_file vient de finir → son contenu s'affiche non-compacté)
  - La section live se met à jour à chaque tool qui se termine
  - Spinner ⠋ visible sous la section live

→ attendu à la fin :
  - Arbre compact uniquement (pas de section live)
  - Body = résumé markdown de l'agent
```

### 6b. spawn_agent + wait_agent + send_agent_message

```
Exécute ces tools dans cet ordre exact :

1. spawn_agent — lance un agent en arrière-plan :
     prompt: "Dans /tmp, crée un fichier agent_test.txt avec le contenu 'hello from subagent',
              puis lis-le pour confirmer, puis retourne le nombre de caractères du fichier."
     agent_type: "general-purpose"
     nickname: "writer"
   → attendu : header "● Spawn Agent  writer · general-purpose"
   → attendu : body = "Task  Dans /tmp, crée un fichier..." (prompt tronqué)
   → attendu quand le tool retourne : "→ <agent_id>  (running)"

2. list_agents — vérifie que l'agent "writer" est visible :
   (appel sans paramètre)
   → attendu : header "List Agents  N agents" avec N ≥ 1
   → attendu : body texte listant l'agent spawné

3. send_agent_message — envoie un message à l'agent en cours :
     agent_id: <id retourné à l'étape 1>
     message: "Quand tu as fini le fichier, ajoute aussi la date courante à la fin."
   → attendu : header "Send Message  → <agent_id>"
   → attendu : body = "↳ Quand tu as fini le fichier..."

4. wait_agent — attend la complétion :
     agent_id: <id retourné à l'étape 1>
   → attendu : header "Wait Agent  <agent_id>" avec spinner le temps de l'attente
   → attendu : body = markdown avec l'output final (nombre de caractères)

5. list_agents — vérifie que l'agent est terminé :
     filter_status: "completed"
   → attendu : header "List Agents  completed  N agents"
   → attendu : l'agent "writer" apparaît dans la liste

Ne fais rien d'autre après l'étape 5.
```

---

## 7. TASK TOOLS

Valide : `task_create`, `task_update`, `task_list`, `task_get`, `task_stop`.

```
Résous cette demande en utilisant explicitement les task tools.

Crée d'abord 5 tâches avec des statuts variés :
- 1 tâche déjà completed
- 1 tâche in_progress avec un activeForm clair
- 3 tâches pending

Chaque tâche doit avoir :
- un titre court et distinct
- une description de 8 à 12 lignes pour au moins 2 tâches (pour tester collapse/expand sidebar)
- un owner si pertinent

Ensuite :
1. Mets une tâche pending en in_progress
2. Marque une autre pending comme completed
3. task_list pour voir l'état global
4. task_get sur une tâche spécifique
5. task_stop sur la tâche in_progress si cohérent

Le sujet peut être : refonte du rendu TUI des tools système.

→ attendu : liste triée in_progress / pending / completed
→ attendu : sélection sidebar + panneau Task Details
→ attendu : repli/expansion des descriptions longues
→ attendu : rendus compacts task_list, task_get, task_stop dans le chat
```

---

## 8. ASK USER QUESTION

Valide : bulle interactive, single-select, multi-select, batch, chemin "Other".

```
Exécute les appels dans cet ordre exact :

1. ask_user_question — single-select :
     question: "Quel style de code préfères-tu pour les fonctions Go ?"
     header: "Style"
     options:
       - label: "Verbeux avec commentaires détaillés"
         description: "Chaque fonction documentée, exemples inclus"
       - label: "Concis et auto-documenté (Recommended)"
         description: "Noms clairs, logique évidente sans commentaires"
   → attendu : bulle interactive avec ▶ sur première option, ↑↓ pour naviguer, Enter pour confirmer

2. ask_user_question — multi-select :
     question: "Quelles fonctionnalités souhaites-tu activer ?"
     header: "Features"
     multiSelect: true
     options:
       - label: "LSP (autocompletion)"
         description: "Intégration language server"
       - label: "Worktrees"
         description: "Gestion de branches parallèles"
       - label: "Notifications"
         description: "Alertes bureau en fin de run"
   → attendu : cases [ ] / [✓], Space pour cocher, Enter pour confirmer
   → attendu : hint "↑↓ navigate · Space toggle · Enter confirm"

3. ask_user_question — batch (2 questions d'un coup) :
     question 1 :
       question: "Quel est le provider LLM prioritaire ?"
       header: "Provider"
       options:
         - label: "Anthropic Claude"
         - label: "OpenAI GPT-4o"
         - label: "Ollama local"
     question 2 :
       question: "Quelle taille de contexte préfères-tu ?"
       header: "Context"
       options:
         - label: "8k tokens"
         - label: "32k tokens"
         - label: "128k tokens (Recommended)"
   → attendu : question 1 interactive → réponse → question 2 interactive → toutes les réponses visibles

4. ask_user_question — chemin "Other" (free-text) :
     question: "Quel nom de projet veux-tu utiliser ?"
     header: "Nom"
     options:
       - label: "nexus-core"
       - label: "seshat-v2"
   → attendu : option "Other" ajoutée automatiquement en bas
   → attendu : sélection "Other" → focus éditeur texte → réponse custom dans l'historique Q→A

Ne fais rien d'autre après l'étape 4.
```

---

## 9. PLAN MODE

Valide : `enter_plan_mode`, `submit_plan`, dialog review, `exit_plan_mode`.

```
Exécute les étapes dans cet ordre exact :

1. enter_plan_mode
   → attendu : pas de bulle dans le transcript
   → attendu : indicateur "Plan Mode" visible dans le header

2. list_directory — path: "."
   → attendu : bulle visible normalement

3. submit_plan :
     slug: "test-plan-tui"
     content: |
       # Plan de test TUI

       ## Contexte
       Ce plan valide le flow submit_plan dans le TUI Seshat.

       ## Étapes
       1. Vérifier que le dialog de review s'ouvre
       2. Vérifier la navigation ↑↓ sur les lignes
       3. Vérifier que "c" ouvre l'éditeur de commentaire de ligne
       4. Vérifier que "g" ouvre l'éditeur de commentaire global
       5. Vérifier que "f" bascule en plein écran
       6. Vérifier que "a" approuve et envoie "Proceed"

       ## Fichiers touchés
       - Aucun (plan de test uniquement)

       ## Risques
       - Aucun risque — plan fictif à des fins de test
   → attendu : pas de bulle dans le transcript
   → attendu : dialog "Plan Review" s'ouvre avec le contenu markdown
   → attendu : navigation ↑↓ (curseur ›), "c" commentaire ligne, "g" commentaire global,
               "f" plein écran, "a"/ctrl+y pour approuver

4. Après approbation → exit_plan_mode
   → attendu : pas de bulle
   → attendu : indicateur "Plan Mode" disparaît du header

VARIANTE — Request Changes :
Même prompt mais à l'étape 3, appuyer sur "g" pour laisser un commentaire global, puis "r".
→ attendu : demande de changement envoyée au modèle
→ attendu : re-soumission submit_plan avec version incrémentée (v2)
→ attendu : dialog se rouvre avec "v2 (2/2)" et navigation [/] entre versions
```

---

## 10. WORKTREE

Valide : `enter_worktree`, activité dans la worktree, `exit_worktree`.

```
Exécute ces étapes dans cet ordre exact :

1. enter_worktree :
     name: "test-ui-worktree"
     branch: "test/worktree-header-display"
   → attendu : enter_worktree n'apparaît PAS comme bulle dans le transcript
   → attendu : header affiche "⎇ test-ui-worktree" dès que la worktree est active

2. list_directory — path: "."
   → attendu : bulle visible, confirme qu'on est dans la worktree

3. read_file — lis n'importe quel fichier Go de la racine
   → attendu : bulle visible normalement

4. exit_worktree :
     action: "remove"
   → attendu : exit_worktree n'apparaît PAS comme bulle dans le transcript
   → attendu : header revient au cwd normal après exit

Ne fais rien d'autre après l'étape 4.
```

---

## 11. MCP & TOOL SEARCH

Valide : `tool_search`, `mcp_list_resources`, `mcp_read_resource`.

```
Exécute les appels dans cet ordre exact :

1. tool_search — query: "file"
   → attendu : header "Tool Search  file  N results"
   → attendu : body = liste "nom_outil · search_hint" (max 6, puis +N more)

2. tool_search — query: "notebook jupyter cell"  max_results: 3
   → attendu : header "Tool Search  notebook jupyter cell  N results"
   → attendu : 1 à 3 lignes (ou "no results" si vide)

3. tool_search — query: "agent spawn"
   → attendu : résultats incluent spawn_agent, wait_agent, send_agent_message

4. mcp_list_resources
   → attendu : header "MCP Resources"
   → attendu : body = liste des ressources (ou vide si aucun serveur MCP actif)

5. mcp_read_resource — uri: première URI retournée à l'étape 4
   (fallback: "file:///tmp/test_nb.ipynb" si aucune ressource disponible)
   → attendu : header "MCP Read Resource  <uri tronquée>"
   → attendu : body = contenu de la ressource

Ne fais rien d'autre après l'étape 5.
```

---

## 12. READ DOCUMENT URL

Valide : `read_document_url` — header URL, prompt hint, body markdown.

```
Exécute les appels dans cet ordre exact :

1. read_document_url — URL simple :
     url: "https://raw.githubusercontent.com/charmbracelet/bubbletea/main/README.md"
   → attendu : header "Read Document  https://raw.githubusercontent.com/…/README.md"
   → attendu : body markdown (titres, listes, code)

2. read_document_url — avec save_path :
     url: "https://raw.githubusercontent.com/charmbracelet/lipgloss/main/README.md"
     save_path: "/tmp/lipgloss_readme.md"
   → attendu : header "Read Document  https://…/README.md  → /tmp/lipgloss_readme.md"

3. read_document_url — avec prompt ciblé :
     url: "https://raw.githubusercontent.com/charmbracelet/bubbletea/main/README.md"
     prompt: "Résume uniquement la section d'installation en 3 lignes"
   → attendu : ligne "↳ Résume uniquement la section d'installation en 3 lignes" avant le body

Ne fais rien d'autre après l'étape 3.
```

---

## 13. AUDIT GLOBAL DE RENDU

Valide l'ensemble des corrections de rendu TUI en une seule session.
Exécute les tools ci-dessous dans l'ordre exact, sans commenter entre les étapes.

```
1. create_directory /tmp/audit_tui

2. write_file /tmp/audit_tui/main.go (contenu: package main / func main() {})
   → permission panel diff + bulle chat avec diff coloré

3. read_file /tmp/audit_tui/main.go
   → header seul, aucun contenu

4. get_file_metadata /tmp/audit_tui/main.go
   → header seul, aucun JSON

5. edit_file /tmp/audit_tui/main.go : ajoute fmt.Println("audit")
   → permission panel old→new + diff rouge/vert dans la bulle

6. bash : grep -n "audit" /tmp/audit_tui/main.go
   → permission panel avec bash colorisé + body numéroté dans la bulle

7. notebook_create /tmp/audit_tui/test.ipynb kernel=python3
   → header seul "✓ Create Notebook", aucun body

8. notebook_write /tmp/audit_tui/test.ipynb avec 2 cells code
   → header "✓ Write Notebook ... · create · 2 cells" + body preview premier cell

9. notebook_edit /tmp/audit_tui/test.ipynb cell-0 replace new_source="x = 42"
   → header "Notebook Edit" + body code colorisé

10. agent inline : explore internal/ du projet (list_directory + grep + read_file)
    → arbre compact en haut + section live (dernier tool non-compact) + spinner

11. remove_file /tmp/audit_tui (recursive: true)
    → header seul, aucun texte

Ne fais rien d'autre après l'étape 11.
```

---

## 14. MEMORY TOOLS

Valide : `memory_create_entities`, `memory_add_observations`, `memory_search_nodes`, `memory_open_nodes`.

```
Exécute ces appels dans cet ordre exact, sans commenter entre les étapes.

1. memory_create_entities — crée 3 entités :
     entities:
       - name: "seshat"
         entity_type: "project"
         observations:
           - "TUI terminal écrit en Go avec bubbletea"
           - "Supporte les agents multi-modaux"
       - name: "Alice"
         entity_type: "person"
         observations:
           - "Développeuse principale du projet"
       - name: "bubbletea"
         entity_type: "library"
   → attendu : header "✓ Create Memory  3 entities"
   → attendu : body = liste 3 lignes :
       seshat (project) · 2 obs
       Alice (person) · 1 ob
       bubbletea (library)

2. memory_add_observations — ajoute des observations :
     observations:
       - entity_name: "bubbletea"
         contents:
           - "Framework TUI de Charmbracelet"
           - "Utilisé par seshat pour le rendu terminal"
       - entity_name: "Alice"
         contents:
           - "Préfère les interfaces en ligne de commande"
   → attendu : header "✓ Memory Observe  2 entities"
   → attendu : body = liste 2 lignes :
       bubbletea · +2 observations
       Alice · +1 observation

3. memory_search_nodes — recherche par mot-clé :
     query: "Go"
   → attendu : header "✓ Memory Search  Go  N results" (N ≥ 1)
   → attendu : body = liste des entités correspondantes avec leur type

4. memory_search_nodes — recherche sans résultats :
     query: "rust programming language"
   → attendu : header "✓ Memory Search  rust programming language  no results"
   → attendu : aucun body

5. memory_open_nodes — ouvre des nœuds par nom exact :
     names: ["seshat", "Alice"]
   → attendu : header "✓ Memory Open  2 nodes" (+ relations si présentes)
   → attendu : body = liste 2 lignes avec nom + type + obs count

6. memory_open_nodes — nom inexistant :
     names: ["entite-inconnue-xyz"]
   → attendu : header "✓ Memory Open  1 node  not found"
   → attendu : aucun body

Ne fais rien d'autre après l'étape 6.
```

---

## 15. GOAL TOOLS

Valide : `create_goal`, `get_goal`, `update_goal`.

```
Exécute ces appels dans cet ordre exact, sans commenter entre les étapes.

1. get_goal — avant toute création (aucun goal actif)
   → attendu : header "Goal · not set"
   → attendu : aucun body

2. create_goal — avec objectif et budget de tokens :
     objective: "Refactoriser le système de rendu TUI pour supporter les nouveaux tools de goal, memory et agent"
     token_budget: 8000
   → attendu : header "Create Goal · Refactoriser le système de rendu TUI… · active"
   → attendu : header mentionne "8000 token budget"
   → attendu : body = "Objective: Refactoriser le système de rendu TUI…" + ligne budget

3. get_goal — après création
   → attendu : header "Goal · active · Refactoriser le système de rendu TUI…"
   → attendu : body = lignes Objective + Budget restant

4. update_goal — change le statut en paused :
     status: "paused"
   → attendu : header "Update Goal · paused"
   → attendu : body = nouvelle ligne statut + budget mis à jour

5. update_goal — reprend et modifie l'objectif :
     status: "active"
     objective: "Refactoriser le rendu TUI — phase 2 : tools de goal et memory"
   → attendu : header "Update Goal · active"
   → attendu : body = nouveau résumé goal

6. get_goal — état final
   → attendu : header "Goal · active · Refactoriser le rendu TUI — phase 2…"
   → attendu : body = objective complet + ligne tokens

7. update_goal — marque comme complete :
     status: "complete"
   → attendu : header "Update Goal · complete"
   → attendu : body = "Goal marked complete (status: complete)…"

Ne fais rien d'autre après l'étape 7.
```

---

## 16. DOCX, MONITOR, CODE COMPLETE, LSP

Valide : `docx`, `monitor`, `code_complete`, `lsp`.

### 16a. docx — create, append, replace

```
Travaille uniquement dans /tmp. Exécute ces appels dans cet ordre exact.

1. docx — créer un nouveau document :
     document_path: "/tmp/test_nexus.docx"
     action: "create"
     content: "Seshat — rapport de test TUI"
     bold: true
     font_size: 18
     alignment: "center"
     title: "Rapport TUI"
     author: "Seshat Agent"
   → attendu : header "✓ Docx  create  ~/test_nexus.docx"
   → attendu : body = "Message: Document created successfully"

2. docx — ajouter du contenu :
     document_path: "/tmp/test_nexus.docx"
     action: "append"
     content: "Section 1 : le rendu TUI utilise bubbletea et lipgloss."
     italic: true
   → attendu : header "✓ Docx  append  ~/test_nexus.docx"
   → attendu : body = "Message: Content appended successfully"

3. docx — créer un document avec table :
     document_path: "/tmp/test_table.docx"
     action: "create"
     table_data: "Tool\tStatus\tFile\ndocx\tDone\tdocx.go\nmonitor\tDone\tmonitor.go\nlsp\tDone\tlsp_tools.go"
   → attendu : header "✓ Docx  create  ~/test_table.docx"
   → attendu : body = "Message: Document created successfully"

4. docx — remplacer le premier paragraphe :
     document_path: "/tmp/test_nexus.docx"
     action: "replace"
     content: "Seshat v2 — rapport de test TUI (mis à jour)"
   → attendu : header "✓ Docx  replace  ~/test_nexus.docx"
   → attendu : body = "Message: Content replaced successfully"

Ne fais rien d'autre après l'étape 4.
```

### 16b. monitor — démarrage en arrière-plan

```
Exécute ces appels dans cet ordre exact.

1. monitor — surveiller un log en boucle :
     command: "for i in $(seq 1 5); do echo \"line $i\"; sleep 0.5; done"
     description: "Génère 5 lignes à 0.5s d'intervalle"
   → attendu : header "Monitor  for i in $(seq 1 5)…  Génère 5 lignes…"
   → attendu : body = "Monitor task started with ID: <id>. Output is being streamed to: <file>"

2. monitor — commande plus simple sans description :
     command: "echo hello && sleep 1 && echo world"
   → attendu : header "Monitor  echo hello && sleep 1…"
   → attendu : body = ligne task ID

Ne fais rien d'autre après l'étape 2.
```

### 16c. code_complete — complétion FIM

```
Exécute ces appels dans cet ordre exact.
(Si le tool code_complete n'est pas activé, passe directement à 16d.)

1. code_complete — compléter une fonction Go :
     prompt: |
       package main

       import "fmt"

       func fibonacci(n int) int {
           if n <= 1 {
               return n
           }
   → attendu : header "Code Complete  if n <= 1 {  <provider>/<model>  N tokens"
   → attendu : body = complétion de la fonction en code Go colorisé

2. code_complete — compléter avec suffix :
     prompt: |
       func greet(name string) string {
           return fmt.Sprintf("Hello,
     suffix: |
       ")
       }
   → attendu : header "Code Complete  return fmt.Sprintf(…  <provider>/<model>"
   → attendu : body = fragment de complétion en code colorisé

Ne fais rien d'autre après l'étape 2.
```

### 16d. lsp — opérations code intelligence

```
Utilise un fichier Go existant du projet courant pour chaque opération.
Exécute ces appels dans cet ordre exact.
(Si gopls n'est pas disponible, les résultats peuvent être vides — vérifie quand même le header.)

1. lsp — symboles du document :
     operation: "symbols"
     file_path: "internal/nexustui/ui/chat/tools.go"
   → attendu : header "✓ LSP  symbols  internal/…/tools.go  Found N symbol(s)"
   → attendu : aucun body (summary dans le header suffit)

2. lsp — définition d'un symbole :
     operation: "definition"
     file_path: "internal/nexustui/ui/chat/tools.go"
     line: 99
     column: 6
   → attendu : header "✓ LSP  definition  internal/…/tools.go:99:6  Found N location(s)"
   → attendu : aucun body si summary présent

3. lsp — hover sur un identifiant :
     operation: "hover"
     file_path: "internal/nexustui/ui/chat/tools.go"
     line: 99
     column: 6
   → attendu : header "✓ LSP  hover  internal/…/tools.go:99:6  <texte hover tronqué>"
   → attendu : aucun body si summary présent

4. lsp — recherche workspace :
     operation: "workspace_symbol"
     file_path: "internal/nexustui/ui/chat/tools.go"
     query: "ToolMessageItem"
   → attendu : header "✓ LSP  workspace_symbol  internal/…/tools.go  ToolMessageItem  Found N symbol(s)"

Ne fais rien d'autre après l'étape 4.
```
