"""Compatibility command for the single full OMP distribution."""

import argparse
from build import build as build_distribution


def build(destination, sdk_root=None):
    return build_distribution(destination, sdk_root)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, help="new or empty distribution directory")
    parser.add_argument("--sdk", help="optional build-time SDK source directory")
    options = parser.parse_args()
    build(options.output, options.sdk)
