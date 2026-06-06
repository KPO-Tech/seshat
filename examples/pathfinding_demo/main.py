"""
Main module for pathfinding demo.
Runs BFS and A* algorithms on a random grid and compares their performance.
"""

from typing import Tuple
import random
from grid import Grid
from algorithms import BFS, AStar, PathfindingResult


def print_result_header(algorithm_name: str) -> None:
    """Print a header for algorithm results."""
    print(f"\n{'='*50}")
    print(f"Résultats de l'algorithme {algorithm_name}")
    print(f"{'='*50}")


def print_statistics(result: PathfindingResult) -> None:
    """Print statistics for a pathfinding result."""
    if result.path:
        print(f"Chemin trouvé ! Longueur: {result.path_length}")
        print(f"Cellules explorées: {result.explored_cells}")
        print(f"Temps d'exécution: {result.execution_time:.6f} secondes")
    else:
        print("Aucun chemin trouvé !")
        print(f"Cellules explorées: {result.explored_cells}")
        print(f"Temps d'exécution: {result.execution_time:.6f} secondes")


def compare_results(bfs_result: PathfindingResult, astar_result: PathfindingResult) -> None:
    """Compare and print statistics for BFS and A* results."""
    print(f"\n{'='*50}")
    print("COMPARAISON DES ALGORITHMES")
    print(f"{'='*50}")
    
    if bfs_result.path and astar_result.path:
        print("Les deux algorithmes ont trouvé un chemin !")
        print(f"Longueur du chemin BFS: {bfs_result.path_length}")
        print(f"Longueur du chemin A*:  {astar_result.path_length}")
        
        if bfs_result.path_length == astar_result.path_length:
            print("✓ Les deux algorithmes ont trouvé un chemin de même longueur (optimal)")
        else:
            print("✗ Les algorithmes ont trouvé des chemins de longueurs différentes")
        
        print(f"\nCellules explorées BFS: {bfs_result.explored_cells}")
        print(f"Cellules explorées A*:  {astar_result.explored_cells}")
        
        efficiency_improvement = ((bfs_result.explored_cells - astar_result.explored_cells) 
                                 / bfs_result.explored_cells * 100)
        print(f"Efficacité A* vs BFS: {efficiency_improvement:.1f}% de cellules explorées en moins")
        
        print(f"\nTemps d'exécution BFS: {bfs_result.execution_time:.6f} secondes")
        print(f"Temps d'exécution A*:  {astar_result.execution_time:.6f} secondes")
        
        if astar_result.execution_time < bfs_result.execution_time:
            speed_improvement = ((bfs_result.execution_time - astar_result.execution_time) 
                               / bfs_result.execution_time * 100)
            print(f"A* est {speed_improvement:.1f}% plus rapide que BFS")
        else:
            print("BFS est plus rapide que A* pour cette instance")
            
    elif bfs_result.path:
        print("Seul BFS a trouvé un chemin !")
    elif astar_result.path:
        print("Seul A* a trouvé un chemin !")
    else:
        print("Aucun des deux algorithmes n'a trouvé de chemin !")


def find_valid_start_goal(grid: Grid, max_attempts: int = 100) -> Tuple[Tuple[int, int], Tuple[int, int]]:
    """
    Find valid start and goal positions that are not obstacles.
    
    Args:
        grid: Grid instance
        max_attempts: Maximum number of attempts to find valid positions
        
    Returns:
        Tuple of (start, goal) positions
    """
    attempts = 0
    while attempts < max_attempts:
        # Try to find start and goal positions
        start = (random.randint(0, grid.height - 1), random.randint(0, grid.width - 1))
        goal = (random.randint(0, grid.height - 1), random.randint(0, grid.width - 1))
        
        # Make sure start and goal are different and valid
        if (start != goal and 
            grid.is_valid_position(start[0], start[1]) and 
            grid.is_valid_position(goal[0], goal[1])):
            return start, goal
        
        attempts += 1
    
    # If random attempts fail, use corners
    start = (0, 0)
    goal = (grid.height - 1, grid.width - 1)
    
    # Check if corners are valid
    if not grid.is_valid_position(start[0], start[1]):
        start = (0, 1)
    if not grid.is_valid_position(goal[0], goal[1]):
        goal = (grid.height - 1, grid.width - 2)
    
    return start, goal


def main() -> None:
    """Main function to run the pathfinding demo."""
    print("Démonstration de Pathfinding - BFS vs A*")
    print("=" * 50)
    
    # Create a 20x20 grid with 30% obstacle probability
    grid = Grid(width=20, height=20, obstacle_probability=0.3)
    
    # Generate random grid
    print("Génération d'une grille aléatoire 20x20...")
    grid.generate_random_grid()
    
    # Find valid start and goal positions
    start, goal = find_valid_start_goal(grid)
    grid.set_start_goal(start, goal)
    
    print(f"Position de départ (S): {start}")
    print(f"Position d'arrivée (G): {goal}")
    
    # Display the original grid
    print("\nGrille originale:")
    grid.display_grid()
    
    # Create algorithm instances
    bfs = BFS(grid)
    astar = AStar(grid)
    
    # Run BFS
    print("\nExécution de l'algorithme BFS...")
    bfs_result = bfs.find_path(start, goal)
    
    # Run A*
    print("\nExécution de l'algorithme A*...")
    astar_result = astar.find_path(start, goal)
    
    # Display results
    print_result_header("BFS")
    print_statistics(bfs_result)
    if bfs_result.path:
        print("\nGrille avec le chemin BFS:")
        grid.display_grid(bfs_result.path)
    
    print_result_header("A*")
    print_statistics(astar_result)
    if astar_result.path:
        print("\nGrille avec le chemin A*:")
        grid.display_grid(astar_result.path)
    
    # Compare results
    compare_results(bfs_result, astar_result)
    
    print(f"\n{'='*50}")
    print("Démonstration terminée !")
    print(f"{'='*50}")


if __name__ == "__main__":
    main()