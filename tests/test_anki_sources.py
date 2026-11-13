"""Corresponding sources keep every upstream byte after removing gzip wrapping."""
import gzip
import hashlib
import importlib.util
import io
import json
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class CorrespondingSourcesTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location("anki_sources", ROOT / "build/prepare-anki-sources.py")
        cls.builder = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.builder)
        spec = importlib.util.spec_from_file_location("verify_anki", ROOT / "build/verify-anki.py")
        cls.verifier = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.verifier)

    def test_repack_preserves_all_source_archives_and_provenance(self):
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            inputs = directory / "original.zip"
            originals = {}
            locked = {}
            with zipfile.ZipFile(inputs, "w") as archive:
                for name in ("rust/example-1.0.crate", "renderer/example-2.0.tgz"):
                    inner = io.BytesIO()
                    with tarfile.open(fileobj=inner, mode="w") as source:
                        for filename, content in (("src/main.txt", b"source content\n" * 20),
                                                  ("LICENSE", b"Original license and copyright\n")):
                            info = tarfile.TarInfo(filename)
                            info.size = len(content)
                            source.addfile(info, io.BytesIO(content))
                    raw = inner.getvalue()
                    compressed = gzip.compress(raw, mtime=0)
                    originals[name] = raw
                    locked[name] = hashlib.sha256(compressed).hexdigest()
                    archive.writestr(name, compressed)
            output = directory / "sources.tar.xz"
            self.builder.prepare_sources([inputs], locked, output)
            self.verifier.verify_corresponding_sources(output, locked)
            with tarfile.open(output, "r:xz") as archive:
                index = json.load(archive.extractfile("SOURCES.json"))
                self.assertEqual(set(index["files"]), set(locked))
                for original, record in index["files"].items():
                    raw = archive.extractfile(record["path"]).read()
                    self.assertEqual(raw, originals[original])
                    self.assertEqual(record["sourceSHA256"], locked[original])
                    self.assertEqual(record["tarSHA256"], hashlib.sha256(raw).hexdigest())
                self.assertEqual(len(archive.getmembers()), len(locked) + 1)
            repeat = directory / "repeat.tar.xz"
            self.builder.prepare_sources([inputs], locked, repeat)
            self.assertEqual(output.read_bytes(), repeat.read_bytes())
            with self.assertRaisesRegex(ValueError, "provenance differs"):
                self.verifier.verify_corresponding_sources(output, dict.fromkeys(locked, "0" * 64))

    def test_verifier_rejects_changed_tar_bytes(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "sources.tar.xz"
            record = {"schemaVersion": 1, "files": {"rust/example.crate": {
                "path": "rust/example.tar", "sourceSHA256": "0" * 64,
                "tarSHA256": hashlib.sha256(b"original source").hexdigest()}}}
            with tarfile.open(output, "w:xz") as archive:
                for name, data in (("rust/example.tar", b"changed source"),
                                   ("SOURCES.json", json.dumps(record).encode("utf-8"))):
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    archive.addfile(info, io.BytesIO(data))
            with self.assertRaisesRegex(ValueError, "tar content differs"):
                self.verifier.verify_corresponding_sources(output, {"rust/example.crate": "0" * 64})

    def test_rejects_a_changed_locked_source_before_creating_output(self):
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            source = directory / "example.crate"
            source.write_bytes(gzip.compress(b"unexpected source"))
            output = directory / "sources.tar.xz"
            with self.assertRaisesRegex(ValueError, "Missing exact corresponding source"):
                self.builder.prepare_sources([directory], {"rust/example.crate": "0" * 64}, output)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
