"""Make a read-only snapshot and a disposable working copy for preview.sh."""
from pathlib import Path
import shutil
import sqlite3
import sys
import time

source, run = map(Path, sys.argv[1:])
snapshot = run / "snapshot"
snapshot.mkdir()
db = "tasks.sqlite3"
names = (db, db + "-wal", db + "-journal")


def file_versions():
    versions = {}
    for name in names:
        try:
            info = (source / name).stat()
        except FileNotFoundError:
            if name == db:
                raise
            continue
        versions[name] = (info.st_dev, info.st_ino, info.st_size, info.st_ctime_ns)
    return versions


# SQLite only opens the private copy. Capture the DB and transactional sidecars
# across an interval with no writes; retry if a writer commits or checkpoints.
for attempt in range(5):
    before = file_versions()
    try:
        for name in names:
            if name in before:
                shutil.copyfile(source / name, snapshot / name)
            else:
                (snapshot / name).unlink(missing_ok=True)
        if before == file_versions():
            break
    except FileNotFoundError:
        pass
    time.sleep(0.1)
else:
    sys.exit("Production data changed during all snapshot attempts. Please retry.")

# Recover/checkpoint any copied journals before freezing the snapshot.
with sqlite3.connect(snapshot / db) as conn:
    conn.execute("PRAGMA journal_mode=DELETE").fetchone()
    if conn.execute("PRAGMA quick_check").fetchall() != [("ok",)]:
        sys.exit("The isolated SQLite snapshot failed its integrity check.")
conn.close()
if (source / "icons").is_dir():
    shutil.copytree(source / "icons", snapshot / "icons", copy_function=shutil.copyfile)
shutil.copytree(snapshot, run / "data")
for item in snapshot.rglob("*"):
    item.chmod(0o500 if item.is_dir() else 0o400)
snapshot.chmod(0o500)
