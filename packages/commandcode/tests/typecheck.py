#!/usr/bin/env python3
"""Check browser types using the same selected SDK mapping as the builder."""
import argparse
from pathlib import Path
import sys

PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE / "scripts"))
from _web import compile_web

sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import sdk_source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sdk", type=Path)
    args = parser.parse_args()
    compile_web(PACKAGE, sdk_source(args.sdk))


if __name__ == "__main__":
    main()
