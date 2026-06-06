"""
Unit tests for the pathfinding demo.
Tests grid generation, position validation, and pathfinding algorithms.
"""

import unittest
from typing import List, Tuple
from grid import Grid
from algorithms import BFS, AStar, PathfindingResult


class TestGrid(unittest.TestCase):
    """Test cases for the Grid class."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.grid = Grid(width=5, height=5, obstacle_probability=0.0)
        self.grid.generate_random_grid()
    
    def test_grid_dimensions(self):
        """Test that grid has correct dimensions."""
        self.assertEqual(self.grid.width, 5)
        self.assertEqual(self.grid.height, 5)
        self.assertEqual(len(self.grid.grid), 5)
        self.assertEqual(len(self.grid.grid[0]), 5)
    
    def test_grid_generation_no_obstacles(self):
        """Test grid generation with no obstacles."""
        # With obstacle_probability=0.0, all cells should be empty
        for row in self.grid.grid:
            for cell in row:
                self.assertEqual(cell, ' ')
    
    def test_grid_generation_with_obstacles(self):
        """Test grid generation with obstacles."""
        # Create a new grid with high obstacle probability
        obstacle_grid = Grid(width=5, height=5, obstacle_probability=1.0)
        obstacle_grid.generate_random_grid()
        
        # With obstacle_probability=1.0, all cells should be obstacles
        for row in obstacle_grid.grid:
            for cell in row:
                self.assertEqual(cell, '#')
    
    def test_valid_position(self):
        """Test position validation."""
        # Valid positions
        self.assertTrue(self.grid.is_valid_position(0, 0))
        self.assertTrue(self.grid.is_valid_position(2, 2))
        self.assertTrue(self.grid.is_valid_position(4, 4))
        
        # Invalid positions (out of bounds)
        self.assertFalse(self.grid.is_valid_position(-1, 0))
        self.assertFalse(self.grid.is_valid_position(5, 0))
        self.assertFalse(self.grid.is_valid_position(0, -1))
        self.assertFalse(self.grid.is_valid_position(0, 5))
    
    def test_set_start_goal(self):
        """Test setting start and goal positions."""
        start = (0, 0)
        goal = (4, 4)
        
        self.grid.set_start_goal(start, goal)
        
        self.assertEqual(self.grid.start, start)
        self.assertEqual(self.grid.goal, goal)
        self.assertEqual(self.grid.get_cell(0, 0), 'S')
        self.assertEqual(self.grid.get_cell(4, 4), 'G')
    
    def test_set_start_goal_invalid_position(self):
        """Test setting start/goal to invalid positions."""
        # Create grid and set obstacles manually
        obstacle_grid = Grid(width=3, height=3, obstacle_probability=0.0)
        obstacle_grid.generate_random_grid()
        
        # Set obstacles to block start and goal positions
        obstacle_grid.grid[0][0] = '#'  # Block start position
        obstacle_grid.grid[2][2] = '#'  # Block goal position
        
        # These positions should be invalid (obstacles)
        with self.assertRaises(ValueError):
            obstacle_grid.set_start_goal((0, 0), (2, 2))
    
    def test_get_set_cell(self):
        """Test getting and setting cell values."""
        # Create a fresh grid without start/goal for this test
        test_grid = Grid(width=5, height=5, obstacle_probability=0.0)
        test_grid.generate_random_grid()
        
        # Get cell value
        self.assertEqual(test_grid.get_cell(0, 0), ' ')
        
        # Set cell value to obstacle
        test_grid.set_cell(0, 0, '#')
        self.assertEqual(test_grid.get_cell(0, 0), '#')
        
        # Reset cell value
        test_grid.set_cell(0, 0, ' ')
        self.assertEqual(test_grid.get_cell(0, 0), ' ')
        
        # Test invalid cell value
        with self.assertRaises(ValueError):
            test_grid.set_cell(0, 0, 'X')


class TestPathfindingAlgorithms(unittest.TestCase):
    """Test cases for pathfinding algorithms."""
    
    def setUp(self):
        """Set up test fixtures."""
        # Create a simple 3x3 grid without obstacles
        self.grid = Grid(width=3, height=3, obstacle_probability=0.0)
        self.grid.generate_random_grid()
        self.grid.set_start_goal((0, 0), (2, 2))
        
        self.bfs = BFS(self.grid)
        self.astar = AStar(self.grid)
    
    def test_bfs_simple_path(self):
        """Test BFS on a simple grid without obstacles."""
        result = self.bfs.find_path((0, 0), (2, 2))
        
        self.assertIsNotNone(result.path)
        self.assertTrue(len(result.path) > 0)
        self.assertEqual(result.path[0], (0, 0))
        self.assertEqual(result.path[-1], (2, 2))
        self.assertGreater(result.explored_cells, 0)
        self.assertGreater(result.execution_time, 0)
    
    def test_astar_simple_path(self):
        """Test A* on a simple grid without obstacles."""
        result = self.astar.find_path((0, 0), (2, 2))
        
        self.assertIsNotNone(result.path)
        self.assertTrue(len(result.path) > 0)
        self.assertEqual(result.path[0], (0, 0))
        self.assertEqual(result.path[-1], (2, 2))
        self.assertGreater(result.explored_cells, 0)
        self.assertGreater(result.execution_time, 0)
    
    def test_bfs_no_path(self):
        """Test BFS when no path exists."""
        # Create a grid and manually set obstacles to block all paths
        blocked_grid = Grid(width=3, height=3, obstacle_probability=0.0)
        blocked_grid.generate_random_grid()
        
        # Block all cells except start and goal
        blocked_grid.grid[0][1] = '#'  # (0,1)
        blocked_grid.grid[1][0] = '#'  # (1,0)
        blocked_grid.grid[1][1] = '#'  # (1,1)
        blocked_grid.grid[1][2] = '#'  # (1,2)
        blocked_grid.grid[2][1] = '#'  # (2,1)
        
        blocked_grid.set_start_goal((0, 0), (2, 2))
        
        bfs = BFS(blocked_grid)
        result = bfs.find_path((0, 0), (2, 2))
        
        self.assertIsNone(result.path)
        self.assertEqual(result.path_length, 0)
        self.assertGreater(result.explored_cells, 0)
    
    def test_astar_no_path(self):
        """Test A* when no path exists."""
        # Create a grid and manually set obstacles to block all paths
        blocked_grid = Grid(width=3, height=3, obstacle_probability=0.0)
        blocked_grid.generate_random_grid()
        
        # Block all cells except start and goal
        blocked_grid.grid[0][1] = '#'  # (0,1)
        blocked_grid.grid[1][0] = '#'  # (1,0)
        blocked_grid.grid[1][1] = '#'  # (1,1)
        blocked_grid.grid[1][2] = '#'  # (1,2)
        blocked_grid.grid[2][1] = '#'  # (2,1)
        
        blocked_grid.set_start_goal((0, 0), (2, 2))
        
        astar = AStar(blocked_grid)
        result = astar.find_path((0, 0), (2, 2))
        
        self.assertIsNone(result.path)
        self.assertEqual(result.path_length, 0)
        self.assertGreater(result.explored_cells, 0)
    
    def test_bfs_same_start_goal(self):
        """Test BFS when start and goal are the same."""
        result = self.bfs.find_path((0, 0), (0, 0))
        
        self.assertIsNotNone(result.path)
        self.assertEqual(result.path, [(0, 0)])
        self.assertEqual(result.path_length, 0)
    
    def test_astar_same_start_goal(self):
        """Test A* when start and goal are the same."""
        result = self.astar.find_path((0, 0), (0, 0))
        
        self.assertIsNotNone(result.path)
        self.assertEqual(result.path, [(0, 0)])
        self.assertEqual(result.path_length, 0)
    
    def test_algorithms_find_same_path_length(self):
        """Test that both algorithms find paths of the same length (optimal)."""
        bfs_result = self.bfs.find_path((0, 0), (2, 2))
        astar_result = self.astar.find_path((0, 0), (2, 2))
        
        self.assertIsNotNone(bfs_result.path)
        self.assertIsNotNone(astar_result.path)
        self.assertEqual(bfs_result.path_length, astar_result.path_length)
    
    def test_manhattan_distance(self):
        """Test Manhattan distance calculation."""
        # Test the Manhattan distance method directly
        astar = AStar(self.grid)
        
        # Distance from (0,0) to (2,2) should be 4
        distance = astar._manhattan_distance((0, 0), (2, 2))
        self.assertEqual(distance, 4)
        
        # Distance from (0,0) to (0,0) should be 0
        distance = astar._manhattan_distance((0, 0), (0, 0))
        self.assertEqual(distance, 0)
        
        # Distance from (1,1) to (3,4) should be 5
        distance = astar._manhattan_distance((1, 1), (3, 4))
        self.assertEqual(distance, 5)
    
    def test_path_reconstruction(self):
        """Test path reconstruction from came_from dictionary."""
        bfs = BFS(self.grid)
        
        # Create a simple came_from dictionary
        came_from = {
            (1, 0): (0, 0),  # (1,0) came from (0,0)
            (1, 1): (1, 0),  # (1,1) came from (1,0)
            (2, 1): (1, 1),  # (2,1) came from (1,1)
            (2, 2): (2, 1)   # (2,2) came from (2,1)
        }
        
        path = bfs._reconstruct_path(came_from, (2, 2))
        expected_path = [(0, 0), (1, 0), (1, 1), (2, 1), (2, 2)]
        
        self.assertEqual(path, expected_path)


class TestPathfindingResult(unittest.TestCase):
    """Test cases for PathfindingResult class."""
    
    def test_pathfinding_result_with_path(self):
        """Test PathfindingResult when a path is found."""
        path = [(0, 0), (1, 0), (1, 1), (2, 1), (2, 2)]
        result = PathfindingResult(path, 10, 0.001)
        
        self.assertEqual(result.path, path)
        self.assertEqual(result.explored_cells, 10)
        self.assertEqual(result.execution_time, 0.001)
        self.assertEqual(result.path_length, 4)  # 5 positions - 1 = 4 moves
    
    def test_pathfinding_result_without_path(self):
        """Test PathfindingResult when no path is found."""
        result = PathfindingResult(None, 15, 0.002)
        
        self.assertIsNone(result.path)
        self.assertEqual(result.explored_cells, 15)
        self.assertEqual(result.execution_time, 0.002)
        self.assertEqual(result.path_length, 0)


if __name__ == '__main__':
    # Run the tests
    unittest.main(verbosity=2)