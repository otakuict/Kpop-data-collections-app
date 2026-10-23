"""Remove private usage history from an exported, offline seed snapshot."""

import sqlite3
import sys


def strip_usage(path):
    database = sqlite3.connect(path)
    try:
        exists = database.execute(
            "SELECT 1 FROM sqlite_master WHERE type='table' AND name='usage_visits'"
        ).fetchone()
        if not exists:
            return
        with database:
            database.execute("DELETE FROM usage_visits")
        database.execute("VACUUM")
    finally:
        database.close()


if __name__ == "__main__":
    strip_usage(sys.argv[1])
