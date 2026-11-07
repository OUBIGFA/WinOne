"""Exercise the production builder with a tiny, readable ECDICT-shaped input."""
import csv
from contextlib import closing
import importlib.util
import pathlib
import sqlite3
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("learning_dictionary", ROOT / "build/learning_dictionary.py")
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)

class DictionaryTests(unittest.TestCase):
    def test_retains_inflections_lemmas_and_decodes_lines(self):
        with tempfile.TemporaryDirectory() as folder:
            source, target = pathlib.Path(folder)/"input.csv", pathlib.Path(folder)/"dict.db"
            with source.open("w", encoding="utf-8", newline="") as stream:
                writer = csv.DictWriter(stream, fieldnames=["word", "translation", "tag", "exchange"])
                writer.writeheader()
                writer.writerows([
                    dict(word="go", translation=r"v. 去\n行走", tag="cet4", exchange="p:went/d:gone/i:going/3:goes"),
                    dict(word="went", translation="go 的过去式", tag="", exchange="0:go"),
                    dict(word="gone", translation="go 的过去分词", tag="", exchange="0:go"),
                    dict(word="unrelated", translation="无关", tag="", exchange=""),
                ])
            builder.build(source, target)
            with closing(sqlite3.connect(target)) as db:
                self.assertEqual(db.execute("select word from entries order by word").fetchall(), [("go",),("gone",),("went",)])
                self.assertIn("\n",db.execute("select translation from entries where word='go'").fetchone()[0])
                self.assertEqual(db.execute("select lemma from forms where form='went'").fetchall(),[("go",)])

if __name__ == "__main__": unittest.main()
