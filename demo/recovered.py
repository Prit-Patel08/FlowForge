#!/usr/bin/env python3
"""Healthy worker profile used after demo recovery."""

import time


def main() -> None:
    print("healthy worker restarted")
    for i in range(1, 200):
        print(f"heartbeat {i}: service healthy")
        time.sleep(0.2)


if __name__ == "__main__":
    main()
