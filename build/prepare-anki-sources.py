"""Pack all locked source tarballs into one deterministic solid XZ archive.

The original gzip streams defeat the installer's solid compression. Removing
only that wrapping preserves their complete tar bytes, including source,
licenses, file modes and links. SOURCES.json records both input and tar hashes.
"""
import argparse
import gzip
import hashlib
import io
import json
import lzma
import tarfile
import zipfile
from pathlib import Path


def prepare_sources(inputs, locked, output):
    needed = {Path(name).name for name in locked}
    candidates = {}
    # Reuse the reviewed old ZIP directly when individual downloads have been
    # archived. Every member still has to match its original locked checksum.
    for item in inputs:
        item = Path(item)
        if item.is_file() and zipfile.is_zipfile(item):
            with zipfile.ZipFile(item) as archive:
                for name in archive.namelist():
                    if Path(name).name in needed:
                        data = archive.read(name)
                        candidates[(Path(name).name, hashlib.sha256(data).hexdigest())] = data
        else:
            for path in item.rglob("*"):
                if path.is_file() and path.name in needed:
                    data = path.read_bytes()
                    candidates[(path.name, hashlib.sha256(data).hexdigest())] = data
    verified = {}
    for name, digest in locked.items():
        data = candidates.get((Path(name).name, digest))
        if data is None:
            raise ValueError("Missing exact corresponding source: " + name)
        verified[name] = data

    output = Path(output)
    output.parent.mkdir(parents=True, exist_ok=True)
    index = {"schemaVersion": 1, "files": {}}
    names = set()
    # This is an offline build cost only. A larger dictionary can share source
    # text across the several locked Windows/Rust dependency versions.
    with lzma.open(output, "wb", preset=9) as compressed:
        with tarfile.open(fileobj=compressed, mode="w|", format=tarfile.PAX_FORMAT) as archive:
            def add(name, data):
                info = tarfile.TarInfo(name)
                info.size = len(data)
                info.mode = 0o644
                archive.addfile(info, io.BytesIO(data))

            for name, data in sorted(verified.items()):
                if name.endswith(".tar.gz"):
                    stored = name[:-3]
                elif name.endswith((".tgz", ".crate")):
                    stored = name.rsplit(".", 1)[0] + ".tar"
                else:
                    raise ValueError("Expected a gzip source archive: " + name)
                if stored in names:
                    raise ValueError("Duplicate source archive: " + stored)
                names.add(stored)
                raw = gzip.decompress(data)
                add(stored, raw)
                index["files"][name] = {"path": stored, "sourceSHA256": locked[name],
                                        "tarSHA256": hashlib.sha256(raw).hexdigest()}
            add("SOURCES.json", (json.dumps(index, indent=2, sort_keys=True) + "\n").encode("utf-8"))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--inputs", nargs="+", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--update", action="store_true")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    spec_path = root / "third_party/anki/source.json"
    spec = json.loads(spec_path.read_text(encoding="utf-8"))
    manifest = spec["correspondingSources"]
    output = Path(args.output).resolve()
    if not output.is_relative_to(root / "_temp") and not output.is_relative_to(root / "dist"):
        raise ValueError("Output must remain within repository artifacts")
    prepare_sources(args.inputs, manifest["files"], output)
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    if args.update:
        manifest["file"] = output.name
        manifest["format"] = "tar.xz"
        manifest["sha256"] = digest
        spec_path.write_text(json.dumps(spec, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    elif output.name != manifest["file"] or digest != manifest["sha256"]:
        raise ValueError("Source archive differs from its reviewed name or digest")
    print(f"Corresponding sources: {len(manifest['files'])} inputs, {output.stat().st_size} bytes, SHA-256 {digest}")


if __name__ == "__main__":
    main()
