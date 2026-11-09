"""Prepare a verified offline Anki library from pinned wheel archives."""
import argparse
import hashlib
import json
import shutil
import zipfile
from pathlib import Path


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--archives", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    archives, output = Path(args.archives).resolve(), Path(args.output).resolve()
    if not output.is_relative_to(root / "_temp") and not output.is_relative_to(root / "dist"):
        raise ValueError("Generated runtime must stay inside repository output directories")
    spec_path = root / "third_party/anki/source.json"
    spec = json.loads(spec_path.read_text(encoding="utf-8"))
    marker = output / "ownership.json"
    if output.exists() and any(output.iterdir()) and not marker.exists():
        raise ValueError("Existing output has no ownership record; preserving it")
    for item in spec["wheels"] + [spec["sourceArchive"], spec["rendererArchive"], spec["correspondingSources"]]:
        if sha(archives / item["file"]) != item["sha256"]:
            raise ValueError("Archive checksum differs: " + item["file"])
    output.mkdir(parents=True, exist_ok=True)
    marker.write_text(json.dumps({"sourceSHA256": sha(spec_path)}), encoding="utf-8")
    library = output / "lib"
    notices = root / "third_party/anki/python-notices"
    notices.mkdir(parents=True, exist_ok=True)
    records = []
    for item in spec["wheels"]:
        entries = []
        with zipfile.ZipFile(archives / item["file"]) as wheel:
            for member in wheel.infolist():
                if member.is_dir():
                    continue
                path = (library / member.filename).resolve()
                if not path.is_relative_to(library):
                    raise ValueError("Wheel path escapes library")
                path.parent.mkdir(parents=True, exist_ok=True)
                data = wheel.read(member)
                path.write_bytes(data)
                basename = Path(member.filename).name.lower()
                if ".dist-info/" in member.filename and basename.startswith(("license", "licence", "copying", "notice", "copyright")):
                    name = item["name"] + "-" + item["version"] + "--" + Path(member.filename).name
                    (notices / name).write_bytes(data)
                    entries.append({"path": "python-notices/" + name, "sha256": hashlib.sha256(data).hexdigest()})
        if not entries and item["name"] != "anki":
            raise ValueError("Original Python license missing: " + item["name"])
        records.append({"name": item["name"], "version": item["version"], "license": item["license"], "files": entries})
    (root / "third_party/anki/python-notices.json").write_text(json.dumps(records, indent=2) + "\n", encoding="utf-8")
    source = output / "source"
    source.mkdir(exist_ok=True)
    shutil.copy2(archives / spec["sourceArchive"]["file"], source / spec["sourceArchive"]["file"])
    shutil.copy2(archives / spec["correspondingSources"]["file"], source / spec["correspondingSources"]["file"])
    with zipfile.ZipFile(archives / spec["rendererArchive"]["file"]) as wheel:
        prefix = "_aqt/data/web/"
        for member in wheel.infolist():
            if member.is_dir() or not member.filename.startswith(prefix):
                continue
            name = member.filename[len(prefix):]
            if name not in spec["rendererFiles"] and not any(name.startswith(p) for p in spec["rendererPrefixes"]):
                continue
            path = output / "renderer" / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(wheel.read(member))
    inventory = {str(p.relative_to(output)).replace("\\", "/"): sha(p) for p in output.rglob("*")
                 if p.is_file() and "__pycache__" not in p.parts and p.name not in ("manifest.json", "ownership.json")}
    manifest = {"version": spec["version"], "sourceSHA256": sha(spec_path), "files": inventory}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"Verified Anki {spec['version']}: {len(inventory)} runtime/source files; {len(records)} Python distributions.")


if __name__ == "__main__":
    main()
