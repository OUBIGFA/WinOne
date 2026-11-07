"""Build deterministic, read-only ECDICT data; never download or edit user data."""
import argparse
import csv
from contextlib import closing
import hashlib
import json
import pathlib
import re
import sqlite3

def integer(value):
    return int(value) if (value or "").strip().isdigit() else 0

def decode(value):
    # ECDICT escapes only backslash, newline and carriage return. Unicode-escape
    # decoding would corrupt Chinese; preserve every other literal character.
    return re.sub(r"\\([\\nr])", lambda m: {"\\":"\\", "n":"\n", "r":"\r"}[m[1]], value or "")

def exchanges(value):
    result = {}
    for part in (value or "").split("/"):
        if ":" in part:
            key, val = part.split(":", 1)
            if key in ("0", "p", "d", "i", "3", "r", "t", "s"):
                result[key] = val
    return result

def build(source, target):
    words, links, retained = {}, set(), set()
    with pathlib.Path(source).open(encoding="utf-8-sig", newline="") as stream:
        for row in csv.DictReader(stream):
            word = row["word"].strip().lower()
            if not word:
                continue
            # Match the runtime's case-insensitive lookup deterministically.
            if word in words:
                continue
            words[word] = row
            bnc, coca = integer(row.get("bnc")), integer(row.get("frq"))
            if row.get("tag", "").strip() or integer(row.get("oxford")) or integer(row.get("collins")) or 0 < bnc <= 80000 or 0 < coca <= 80000:
                retained.add(word)
            for key, value in exchanges(row.get("exchange")).items():
                for related in value.split(","):
                    related = related.strip().lower()
                    if related:
                        links.add((word, related) if key == "0" else (related, word))
    # Follow both directions to retain untagged inflections AND their lemmas.
    changed = True
    while changed:
        changed = False
        for form, lemma in links:
            if form in retained or lemma in retained:
                for word in (form, lemma):
                    if word in words and word not in retained:
                        retained.add(word)
                        changed = True
    with closing(sqlite3.connect(target)) as db:
        db.executescript("""
            PRAGMA journal_mode=OFF;
            PRAGMA user_version=1;
            CREATE TABLE entries(word TEXT PRIMARY KEY COLLATE NOCASE, phonetic TEXT, definition TEXT, translation TEXT, tag TEXT, bnc INTEGER, coca INTEGER, collins INTEGER, oxford INTEGER, exchange TEXT) WITHOUT ROWID;
            CREATE TABLE forms(form TEXT COLLATE NOCASE, lemma TEXT COLLATE NOCASE, PRIMARY KEY(form,lemma)) WITHOUT ROWID;
        """)
        for word in sorted(retained):
            row = words[word]
            db.execute("INSERT INTO entries VALUES (?,?,?,?,?,?,?,?,?,?)", (word, decode(row.get("phonetic")), decode(row.get("definition")), decode(row.get("translation")), row.get("tag", ""), integer(row.get("bnc")), integer(row.get("frq")), integer(row.get("collins")), integer(row.get("oxford")), row.get("exchange", "")))
        db.executemany("INSERT INTO forms VALUES (?,?)", sorted((form, lemma) for form, lemma in links if lemma in retained))
        db.commit()
        db.execute("VACUUM")
        if db.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
            raise RuntimeError("Generated dictionary failed integrity check")
    return len(retained)

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=pathlib.Path)
    parser.add_argument("target", type=pathlib.Path)
    parser.add_argument("--source-sha256", required=True)
    args = parser.parse_args()
    if hashlib.sha256(args.source.read_bytes()).hexdigest() != args.source_sha256:
        raise SystemExit("ECDICT source checksum mismatch")
    count = build(args.source, args.target)
    print(json.dumps({"entries": count, "bytes": args.target.stat().st_size, "sha256": hashlib.sha256(args.target.read_bytes()).hexdigest()}))
