"""
Grid module for pathfinding demo.
Contains the Grid class for managing a 20x20 grid with obstacles.
"""

from typing import List, Tuple, Optional
import random


class Grid:
    """
    Represents a 20x20 grid with obstacles for pathfinding.
    """
    
    def __init__(self, width: int = 20, height: int = 20, obstacle_probability: float = 0.3):
        """
        Initialize a grid with given dimensions and obstacle probability.
        
        Args:
            width: Grid width (default: 20)
            height: Grid height (default: 20)
            obstacle_probability: Probability of a cell being an obstacle (default: 0.3)
        """
        self.width = width
        self.height = height
        self.obstacle_probability = obstacle_probability
        self.grid: List[List[str]] = []
        self.start: Optional[Tuple[int, int]] = None
        self.goal: Optional[Tuple[int, int]] = None
        
    def generate_random_grid(self) -> None:
        """
        Generate a random grid with obstacles.
        Empty cells are represented by ' ' (space).
        """
        self.grid = []
        for _ in range(self.height):
            row = []
            for _ in range(self.width):
                if random.random() < self.obstacle_probability:
                    row.append('#')  # Obstacle
                else:
                    row.append(' ')  # Empty cell
            self.grid.append(row)
    
    def set_start_goal(self, start: Tuple[int, int], goal: Tuple[int, int]) -> None:
        """
        Set the start and goal positions on the grid.
        
        Args:
            start: (row, column) tuple for start position
            goal: (row, column) tuple for goal position
        """
        if not self.is_valid_position(start[0], start[1]):
            raise ValueError(f"Invalid start position: {start}")
        if not self.is_valid_position(goal[0], goal[1]):
            raise ValueError(f"Invalid goal position: {goal}")
            
        self.start = start
        self.goal = goal
        
        # Clear previous start and goal markers if they exist
        for i in range(self.height):
            for j in range(self.width):
                if self.grid[i][j] in ['S', 'G']:
                    self.grid[i][j] = ' '
        
        # Set new start and goal markers
        self.grid[start[0]][start[1]] = 'S'
        self.grid[goal[0]][goal[1]] = 'G'
    
    def is_valid_position(self, row: int, col: int) -> bool:
        """
        Check if a position is valid (within bounds and not an obstacle).
        
        Args:
            row: Row index
            col: Column index
            
        Returns:
            True if position is valid, False otherwise
        """
        if row < 0 or row >= self.height or col < 0 or col >= self.width:
            return False
        
        return self.grid[row][col] != '#'
    
    def display_grid(self, path: Optional[List[Tuple[int, int]]] = None) -> None:
        """
        Display the grid in the terminal.
        
        Args:
            path: Optional path to display on the grid
        """
        # Create a copy of the grid for display
        display_grid = [row[:] for row in self.grid]
        
        # Mark the path if provided
        if path:
            for row, col in path:
                if display_grid[row][col] == ' ':
                    display_grid[row][col] = '.'
        
        # Print the grid with borders
        print('+' + '-' * self.width + '+')
        for row in display_grid:
            print('|' + ''.join(row) + '|')
        print('+' + '-' * self.width + '+')
    
    def get_cell(self, row: int, col: int) -> str:
        """
        Get the value of a cell at given position.
        
        Args:
            row: Row index
            col: Column index
            
        Returns:
            Cell value (' ' for empty, '#' for obstacle, 'S' for start, 'G' for goal)
        """
        if row < 0 or row >= self.height or col < 0 or col >= self.width:
            raise ValueError(f"Invalid position: ({row}, {col})")
        return self.grid[row][col]
    
    def set_cell(self, row: int, col: int, value: str) -> None:
        """
        Set the value of a cell at given position.
        
        Args:
            row: Row index
            col: Column index
            value: Value to set (' ', '#', 'S', 'G', or '.')
        """
        if row < 0 or row >= self.height or col < 0 or col >= self.width:
            raise ValueError(f"Invalid position: ({row}, {col})")
        if value not in [' ', '#', 'S', 'G', '.']:
            raise ValueError(f"Invalid cell value: {value}")
        self.grid[row][col] = value