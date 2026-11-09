"""Verify original Anki/AnkiConnect sources, notices and runtime bytes offline."""
import argparse
import hashlib
import json
from pathlib import Path


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify(root, runtime=None):
    source = root / "third_party/anki"
    spec = json.loads((source / "source.json").read_text(encoding="utf-8"))
    for name in spec["licenseFiles"]:
        if not (source / name).is_file() or not (source / name).stat().st_size:
            raise ValueError("Missing original Anki license/author file: " + name)
    total = 0
    for filename in ("python-notices.json", "rust-notices.json", "rust-notice-supplements.json", "rust-git-notices.json", "renderer-notices.json"):
        for record in json.loads((source / filename).read_text(encoding="utf-8")):
            if not record["files"] and record["name"] != "anki" and not record.get("distribution", "").startswith("not copied:"):
                raise ValueError("Missing original dependency notice: " + record["name"])
            for file in record["files"]:
                if sha(source / file["path"]) != file["sha256"]:
                    raise ValueError("Original notice changed: " + file["path"])
                total += 1
    connect = root / "third_party/anki-connect"
    record = json.loads((connect / "source.json").read_text(encoding="utf-8"))
    for name, digest in record["files"].items():
        if sha(connect / name) != digest:
            raise ValueError("Original AnkiConnect source changed: " + name)
    if not (connect / "GPL-3.0.txt").is_file():
        raise ValueError("Missing AnkiConnect full license")
    if runtime:
        manifest = json.loads((runtime / "manifest.json").read_text(encoding="utf-8"))
        if manifest["sourceSHA256"] != sha(source / "source.json"):
            raise ValueError("Runtime source lock differs")
        for name, digest in manifest["files"].items():
            if sha(runtime / name) != digest:
                raise ValueError("Runtime changed: " + name)
    print(f"Anki original sources and {total} license records verified" + ("; runtime bytes matched" if runtime else ""))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime")
    args = parser.parse_args()
    verify(Path(__file__).resolve().parents[1], Path(args.runtime) if args.runtime else None)
