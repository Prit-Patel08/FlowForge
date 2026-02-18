#!/usr/bin/env python3
"""Intentionally broken worker used for first-minute product demo."""

import math


def main() -> None:
    print("runaway worker started")
    i = 0
    while True:
        i += 1
        # Keep CPU very high while still producing deterministic output.
        _ = sum(math.sqrt(x) for x in range(120000))
        if i % 200 == 0:
            print("processing request 4242 failed, retrying endlessly")


if __name__ == "__main__":
    main()
