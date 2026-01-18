"""
Sample Python module for testing.
"""

from typing import List, Optional


class DataProcessor:
    """Processes data records."""

    def __init__(self, name: str):
        """
        Initialize the processor.

        Args:
            name: The processor name
        """
        self.name = name
        self._records: List[dict] = []

    def add_record(self, record: dict) -> None:
        """
        Add a record to the processor.

        Args:
            record: The record to add
        """
        self._records.append(record)

    def process(self) -> List[dict]:
        """
        Process all records.

        Returns:
            The processed records
        """
        return [self._transform(r) for r in self._records]

    def _transform(self, record: dict) -> dict:
        """Transform a single record."""
        return {k.upper(): v for k, v in record.items()}


def calculate_sum(numbers: List[int]) -> int:
    """
    Calculate the sum of numbers.

    Args:
        numbers: List of integers to sum

    Returns:
        The sum of all numbers
    """
    return sum(numbers)


def find_maximum(values: List[int]) -> Optional[int]:
    """Find the maximum value in a list."""
    return max(values) if values else None


DEFAULT_BATCH_SIZE = 100
