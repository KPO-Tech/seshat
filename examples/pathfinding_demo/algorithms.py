"""
Pathfinding algorithms module.
Contains BFS and A* implementations for grid-based pathfinding.
"""

from typing import List, Tuple, Optional, Dict, Set
from abc import ABC, abstractmethod
import heapq
import time
from grid import Grid


class PathfindingResult:
    """
    Stores the result of a pathfinding algorithm execution.
    """
    
    def __init__(self, path: Optional[List[Tuple[int, int]]], 
                 explored_cells: int, 
                 execution_time: float):
        """
        Initialize pathfinding result.
        
        Args:
            path: Found path as list of (row, col) tuples, or None if no path found
            explored_cells: Number of cells explored during the search
            execution_time: Time taken for the algorithm to execute (in seconds)
        """
        self.path = path
        self.explored_cells = explored_cells
        self.execution_time = execution_time
        self.path_length = len(path) - 1 if path else 0  # -1 because we don't count the start


class PathfindingAlgorithm(ABC):
    """
    Abstract base class for pathfinding algorithms.
    """
    
    def __init__(self, grid: Grid):
        """
        Initialize the algorithm with a grid.
        
        Args:
            grid: Grid instance to find path in
        """
        self.grid = grid
    
    @abstractmethod
    def find_path(self, start: Tuple[int, int], goal: Tuple[int, int]) -> PathfindingResult:
        """
        Find a path from start to goal.
        
        Args:
            start: (row, col) tuple for start position
            goal: (row, col) tuple for goal position
            
        Returns:
            PathfindingResult containing the path and statistics
        """
        pass
    
    def _get_neighbors(self, pos: Tuple[int, int]) -> List[Tuple[int, int]]:
        """
        Get valid neighboring positions (4-directional: up, down, left, right).
        
        Args:
            pos: (row, col) tuple for current position
            
        Returns:
            List of valid neighboring positions
        """
        row, col = pos
        neighbors = []
        
        # 4-directional movement: up, down, left, right
        directions = [(-1, 0), (1, 0), (0, -1), (0, 1)]
        
        for dr, dc in directions:
            new_row, new_col = row + dr, col + dc
            if self.grid.is_valid_position(new_row, new_col):
                neighbors.append((new_row, new_col))
        
        return neighbors
    
    def _reconstruct_path(self, came_from: Dict[Tuple[int, int], Tuple[int, int]], 
                         current: Tuple[int, int]) -> List[Tuple[int, int]]:
        """
        Reconstruct path from the came_from dictionary.
        
        Args:
            came_from: Dictionary mapping each position to its predecessor
            current: Current position (goal)
            
        Returns:
            Reconstructed path as list of positions from start to goal
        """
        path = [current]
        while current in came_from:
            current = came_from[current]
            path.append(current)
        path.reverse()
        return path


class BFS(PathfindingAlgorithm):
    """
    Breadth-First Search pathfinding algorithm.
    """
    
    def find_path(self, start: Tuple[int, int], goal: Tuple[int, int]) -> PathfindingResult:
        """
        Find path using BFS algorithm.
        
        Args:
            start: (row, col) tuple for start position
            goal: (row, col) tuple for goal position
            
        Returns:
            PathfindingResult containing the path and statistics
        """
        start_time = time.time()
        
        # BFS uses a queue (FIFO)
        queue = [start]
        
        # Keep track of visited positions and their predecessors
        visited: Set[Tuple[int, int]] = {start}
        came_from: Dict[Tuple[int, int], Tuple[int, int]] = {}
        
        explored_cells = 0
        
        while queue:
            current = queue.pop(0)  # Dequeue from front
            explored_cells += 1
            
            # Check if we reached the goal
            if current == goal:
                end_time = time.time()
                path = self._reconstruct_path(came_from, current)
                return PathfindingResult(path, explored_cells, end_time - start_time)
            
            # Explore neighbors
            for neighbor in self._get_neighbors(current):
                if neighbor not in visited:
                    visited.add(neighbor)
                    came_from[neighbor] = current
                    queue.append(neighbor)
        
        # No path found
        end_time = time.time()
        return PathfindingResult(None, explored_cells, end_time - start_time)


class AStar(PathfindingAlgorithm):
    """
    A* pathfinding algorithm using Manhattan distance heuristic.
    """
    
    def find_path(self, start: Tuple[int, int], goal: Tuple[int, int]) -> PathfindingResult:
        """
        Find path using A* algorithm with Manhattan distance heuristic.
        
        Args:
            start: (row, col) tuple for start position
            goal: (row, col) tuple for goal position
            
        Returns:
            PathfindingResult containing the path and statistics
        """
        start_time = time.time()
        
        # Priority queue: (f_score, position)
        # f_score = g_score (actual cost) + h_score (heuristic estimate)
        open_set = [(0, start)]
        
        # Keep track of the best g_score for each position
        g_score: Dict[Tuple[int, int], float] = {start: 0}
        
        # Keep track of visited positions and their predecessors
        came_from: Dict[Tuple[int, int], Tuple[int, int]] = {}
        
        explored_cells = 0
        
        while open_set:
            # Get position with lowest f_score
            current_f, current = heapq.heappop(open_set)
            explored_cells += 1
            
            # Check if we reached the goal
            if current == goal:
                end_time = time.time()
                path = self._reconstruct_path(came_from, current)
                return PathfindingResult(path, explored_cells, end_time - start_time)
            
            # Explore neighbors
            for neighbor in self._get_neighbors(current):
                # All moves have cost 1 in our grid
                tentative_g_score = g_score[current] + 1
                
                # If we haven't visited this neighbor or found a better path
                if neighbor not in g_score or tentative_g_score < g_score[neighbor]:
                    came_from[neighbor] = current
                    g_score[neighbor] = tentative_g_score
                    
                    # Calculate f_score = g_score + heuristic
                    f_score = tentative_g_score + self._manhattan_distance(neighbor, goal)
                    
                    # Add to priority queue if not already there
                    if neighbor not in [item[1] for item in open_set]:
                        heapq.heappush(open_set, (f_score, neighbor))
        
        # No path found
        end_time = time.time()
        return PathfindingResult(None, explored_cells, end_time - start_time)
    
    def _manhattan_distance(self, pos1: Tuple[int, int], pos2: Tuple[int, int]) -> int:
        """
        Calculate Manhattan distance between two positions.
        
        Args:
            pos1: First position (row1, col1)
            pos2: Second position (row2, col2)
            
        Returns:
            Manhattan distance between the positions
        """
        return abs(pos1[0] - pos2[0]) + abs(pos1[1] - pos2[1])