# Nexus TUI — Test Prompts

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

### 6c. Agentic Fetch — live tail avec nested tools web

```
Utilise explicitement le tool "agentic_fetch" pour cette tâche :

  url: "https://go.dev/doc/effective_go"
  prompt: "Extrais uniquement les 5 règles de nommage les plus importantes mentionnées dans ce document."

→ attendu pendant l'exécution :
  - Header "● Agentic Fetch  https://go.dev/doc/effective_go"
  - Tag "Prompt" + texte de la requête sous le header
  - Arbre compact des nested tools (fetch, éventuellement web_search)
  - Section live = dernier tool complété en plein rendu
  - Spinner ⠋

→ attendu à la fin :
  - Header avec icône ✓
  - Arbre compact uniquement
  - Body = les 5 règles en markdown

Ne fais rien d'autre après.
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
       - label: "nexus-engine-v2"
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
       Ce plan valide le flow submit_plan dans le TUI Nexus.

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
