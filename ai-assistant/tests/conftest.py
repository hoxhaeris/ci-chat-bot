"""Pytest configuration for the ai-assistant test suite.

Ensures the ai-assistant project root is importable (so ``import tools.*``
works regardless of how pytest is invoked).
"""

import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parent.parent
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
