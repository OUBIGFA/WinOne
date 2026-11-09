"""Collect original license files from checksum-locked upstream Rust crates.

Local Cargo archives are reused. Missing archives are downloaded without
executing them, through the caller-selected downloader, with SHA-256 checks.
"""
import argparse
import hashlib
import json
import subprocess
import tarfile
import tomllib
from pathlib import Path


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    parser.add_argument("--cargo-cache", required=True)
    parser.add_argument("--work", required=True)
    parser.add_argument("--downloader", required=True)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    source, cache, work = Path(args.source), Path(args.cargo_cache), Path(args.work).resolve()
    if not work.is_relative_to(root / "_temp"):
        raise ValueError("Use the repository task temporary directory")
    work.mkdir(parents=True, exist_ok=True)
    upstream = json.loads((source / "cargo/licenses.json").read_text(encoding="utf-8"))
    names = {r["name"] for r in upstream}
    packages = [p for p in tomllib.loads((source / "Cargo.lock").read_text(encoding="utf-8"))["package"]
                if p["name"] in names and p.get("source", "").startswith("registry+")]
    files, downloads = {}, []
    for p in packages:
        name = f'{p["name"]}-{p["version"]}.crate'
        candidates = [*cache.glob("registry/cache/*/" + name), work / name]
        found = next((path for path in candidates if path.is_file() and digest(path.read_bytes()) == p["checksum"]), None)
        if found:
            files[name] = found
        else:
            url = f'https://static.crates.io/crates/{p["name"]}/{name}'
            downloads.append(f'{url}\n  dir={work}\n  out={name}\n  checksum=sha-256={p["checksum"]}\n')
            files[name] = work / name
    if downloads:
        batch = work / "downloads.txt"
        batch.write_text("\n".join(downloads), encoding="utf-8")
        print(f"Reusing {len(packages)-len(downloads)} archives; downloading {len(downloads)} missing sources.", flush=True)
        # The crate CDN can reject multipart range requests for small archives.
        # One stream also keeps source preparation from occupying extra sockets.
        subprocess.run([args.downloader, "--enable-dht=false", "--bt-enable-lpd=false", "--stream-max-connections=1",
                        "--max-concurrent-downloads=3", "--summary-interval=0", "--show-console-readout=false",
                        "--console-log-level=warn", "--auto-file-renaming=false", "-c", "-i", str(batch)], check=True,
                       stdout=(work / "downloads.log").open("w", encoding="utf-8"), stderr=subprocess.STDOUT)
    target = root / "third_party/anki/rust-notices"
    target.mkdir(parents=True, exist_ok=True)
    records, missing = [], []
    supplement_path = root / "third_party/anki/rust-notice-supplements.json"
    supplements = {(r["name"],r["version"]):r for r in json.loads(supplement_path.read_text(encoding="utf-8"))} if supplement_path.exists() else {}
    for p in packages:
        archive = files[f'{p["name"]}-{p["version"]}.crate']
        if digest(archive.read_bytes()) != p["checksum"]:
            raise ValueError("Checksum mismatch: " + archive.name)
        entries = []
        with tarfile.open(archive) as tar:
            for member in tar.getmembers():
                basename = Path(member.name).name.lower()
                if not member.isfile() or not basename.startswith(("license", "licence", "copying", "notice", "copyright", "unlicense")):
                    continue
                if member.size > 2 * 1024 * 1024:
                    continue
                payload = tar.extractfile(member).read()
                relative = Path(member.name).parts[1:]
                filename = p["name"] + "-" + p["version"] + "--" + "--".join(relative)
                (target / filename).write_bytes(payload)
                entries.append({"path": "rust-notices/" + filename, "sha256": digest(payload), "sourcePath": member.name})
        if not entries:
            supplement = supplements.get((p["name"],p["version"]))
            if supplement:
                for item in supplement["files"]:
                    if digest((root / "third_party/anki" / item["path"]).read_bytes()) != item["sha256"]:
                        raise ValueError("Supplemented license changed: " + item["path"])
                entries=supplement["files"]
            else:
                missing.append(p["name"] + " " + p["version"])
        records.append({"name": p["name"], "version": p["version"], "archiveSHA256": p["checksum"], "files": entries})
    (root / "third_party/anki/rust-notices.json").write_text(json.dumps(records, indent=2) + "\n", encoding="utf-8")
    if missing:
        raise ValueError("Upstream archives without original license files: " + ", ".join(missing))
    print(f"Verified original notices for {len(records)} locked registry packages.")


if __name__ == "__main__":
    main()
