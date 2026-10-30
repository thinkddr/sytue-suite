#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Entrada compatible para los stubs del controlador de llavero privado actual."""
from pathlib import Path
import runpy


if __name__ == "__main__":
    runpy.run_path(str(Path(__file__).with_name("apple_llavero-test.py")), run_name="__main__")
