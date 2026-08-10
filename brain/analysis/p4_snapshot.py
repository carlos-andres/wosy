#!/usr/bin/env python3
"""P4 snapshot — the additive-only safety net.

Records sha256 + size + mtime for every file under a root (the "before" state),
so we can PROVE the migration touched no source (.devwork is git-untracked, so a
snapshot is the only net — master §6 P4). Two modes:

  snapshot:  p4_snapshot.py snap   <root> <manifest.json>
  verify:    p4_snapshot.py verify <root> <manifest.json>   # nonzero exit on any change

Read-only. Never writes into <root>.
"""
import hashlib, json, os, sys
from pathlib import Path


def sha256(p: Path) -> str:
    h = hashlib.sha256()
    with open(p, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def scan(root: Path) -> dict:
    out = {}
    for dirpath, dirnames, filenames in os.walk(root):
        # skip VCS noise; never descend into our own staged store output
        dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules", "vendor")]
        for fn in filenames:
            p = Path(dirpath) / fn
            try:
                st = p.stat()
                rel = str(p.relative_to(root))
                out[rel] = {"sha256": sha256(p), "size": st.st_size, "mtime": int(st.st_mtime)}
            except (OSError, ValueError):
                continue
    return out


def main(argv):
    if len(argv) != 4 or argv[1] not in ("snap", "verify"):
        print(__doc__)
        return 2
    mode, root, manifest = argv[1], Path(argv[2]), Path(argv[3])
    if mode == "snap":
        data = {"root": str(root), "files": scan(root)}
        manifest.write_text(json.dumps(data, indent=2))
        print(f"snapshot: {len(data['files'])} files, "
              f"{sum(f['size'] for f in data['files'].values())} bytes -> {manifest}")
        return 0

    before = json.loads(manifest.read_text())["files"]
    after = scan(root)
    changed = [k for k in before if k not in after or after[k]["sha256"] != before[k]["sha256"]]
    added = [k for k in after if k not in before]
    if changed or added:
        for k in changed:
            print(f"CHANGED/REMOVED: {k}")
        for k in added:
            print(f"ADDED: {k}")
        print(f"\nSOURCE MUTATED — {len(changed)} changed/removed, {len(added)} added")
        return 1
    print(f"VERIFY OK: all {len(before)} files unchanged (read-only honored)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
