# Pathfinding Demo

Une démonstration en Python des algorithmes de pathfinding BFS et A* sur une grille avec obstacles.

## Objectif

Ce projet implémente et compare deux algorithmes de recherche de chemin :
- **BFS (Breadth-First Search)** : Algorithme de recherche en largeur
- **A*** : Algorithme A* avec heuristique de distance de Manhattan

L'objectif est de trouver le plus court chemin entre un point de départ et une arrivée dans une grille 20x20 contenant des obstacles aléatoires.

## Fonctionnalités

- Génération d'une grille aléatoire 20x20 avec obstacles (30% de probabilité)
- Implémentation de l'algorithme BFS
- Implémentation de l'algorithme A* avec distance de Manhattan
- Comparaison des performances :
  - Longueur du chemin trouvé
  - Nombre de cellules explorées
  - Temps d'exécution
- Affichage visuel de la grille et des chemins dans le terminal
- Tests unitaires pour valider le fonctionnement

## Structure du projet

```
examples/
└── pathfinding_demo/
    ├── grid.py              # Gestion de la grille et des obstacles
    ├── algorithms.py        # Implémentation des algorithmes BFS et A*
    ├── main.py              # Point d'entrée principal
    ├── test_pathfinding.py  # Tests unitaires
    └── README.md            # Cette documentation
```

## Installation et exécution

### Prérequis

- Python 3.6 ou supérieur
- Aucune dépendance externe (utilisation uniquement de la bibliothèque standard)

### Exécution

1. Naviguez dans le dossier du projet :
   ```bash
   cd examples/pathfinding_demo
   ```

2. Exécutez le programme principal :
   ```bash
   python main.py
   ```

3. Exécutez les tests unitaires :
   ```bash
   python -m unittest test_pathfinding.py
   ```

## Exemple de sortie

```
==================================================
Démonstration de Pathfinding - BFS vs A*
==================================================
Génération d'une grille aléatoire 20x20...
Position de départ (S): (0, 0)
Position d'arrivée (G): (19, 19)

Grille originale:
+--------------------+
|S#   #   #   #      |
|  #   #   #   #  #  |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   # |
|    #   #   #   #   |
|  #   #   #   #   #G|
+--------------------+

==================================================
Résultats de l'algorithme BFS
==================================================
Chemin trouvé ! Longueur: 38
Cellules explorées: 156
Temps d'exécution: 0.001234 secondes

==================================================
Résultats de l'algorithme A*
==================================================
Chemin trouvé ! Longueur: 38
Cellules explorées: 89
Temps d'exécution: 0.000876 secondes

==================================================
COMPARAISON DES ALGORITHMES
==================================================
Les deux algorithmes ont trouvé un chemin !
Longueur du chemin BFS: 38
Longueur du chemin A*:  38
✓ Les deux algorithmes ont trouvé un chemin de même longueur (optimal)

Cellules explorées BFS: 156
Cellules explorées A*:  89
Efficacité A* vs BFS: 42.9% de cellules explorées en moins

Temps d'exécution BFS: 0.001234 secondes
Temps d'exécution A*:  0.000876 secondes
A* est 29.0% plus rapide que BFS

==================================================
Démonstration terminée !
==================================================
```

## Légende de la grille

- `S` : Point de départ (Start)
- `G` : Point d'arrivée (Goal)
- `#` : Obstacle
- ` ` : Cellule vide
- `.` : Chemin trouvé

## Algorithmes implémentés

### BFS (Breadth-First Search)

- **Principe** : Explore la grille couche par couche à partir du point de départ
- **Avantages** :
  - Garantit de trouver le chemin le plus court (en nombre de mouvements)
  - Simple à implémenter
- **Inconvénients** :
  - Explore beaucoup de cellules inutilement
  - Peut être lent sur de grandes grilles

### A* (A-star)

- **Principe** : Utilise une heuristique (distance de Manhattan) pour guider la recherche
- **Fonction d'évaluation** : `f(n) = g(n) + h(n)`
  - `g(n)` : coût réel du départ au noeud n
  - `h(n)` : estimation heuristique du coût de n à l'arrivée
- **Avantages** :
  - Plus efficace que BFS (explore moins de cellules)
  - Garantit aussi de trouver le chemin optimal avec une heuristique admissible
- **Inconvénients** :
  - Plus complexe à implémenter
  - Nécessite une structure de données de file à priorités

## Tests unitaires

Le projet inclut des tests unitaires qui valident :

- La génération de la grille
- La validation des positions
- Le fonctionnement des algorithmes BFS et A*
- La reconstruction des chemins
- Le calcul de la distance de Manhattan
- La gestion des cas où aucun chemin n'existe

Pour exécuter les tests :
```bash
python -m unittest test_pathfinding.py -v
```

## Personnalisation

Vous pouvez facilement personnaliser le projet :

### Modifier la taille de la grille

Dans `main.py`, modifiez les paramètres lors de la création de la grille :
```python
grid = Grid(width=30, height=30, obstacle_probability=0.3)
```

### Modifier la probabilité d'obstacles

```python
grid = Grid(width=20, height=20, obstacle_probability=0.2)  # 20% d'obstacles
```

### Ajouter d'autres algorithmes

1. Créez une nouvelle classe héritant de `PathfindingAlgorithm` dans `algorithms.py`
2. Implémentez la méthode `find_path()`
3. Ajoutez votre algorithme dans `main.py` pour le tester

## Performance

En général, sur une grille avec obstacles :

- **A*** est plus efficace que BFS en termes de cellules explorées
- **A*** est généralement plus rapide que BFS
- Les deux algorithmes trouvent des chemins de même longueur (optimaux)
- L'avantage de A* est plus marqué sur les grandes grilles

## Limites

- La grille est statique (pas de déplacement d'obstacles)
- Les mouvements sont limités à 4 directions (haut, bas, gauche, droite)
- Les coûts de mouvement sont uniformes (tous les mouvements coûtent 1)
- Pas de prise en charge de poids différents sur les cellules

## Contributions

Ce projet est une démonstration éducative des algorithmes de pathfinding. N'hésitez pas à l'étudier, le modifier et l'améliorer !