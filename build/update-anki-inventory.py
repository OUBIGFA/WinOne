"""Record the complete generated Anki payload; never scan user dist content."""
import argparse
import json
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument("--runtime",required=True)
parser.add_argument("--check",action="store_true")
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
runtime=Path(args.runtime)
manifest=json.loads((runtime/"manifest.json").read_text(encoding="utf-8"))
names=["shared/anki/"+name for name in manifest["files"]]+["shared/anki/manifest.json"]
names.extend("shared/anki/"+p.name for p in (root/"modules/anki").glob("*.py") if not p.name.startswith("test_"))
names.extend("shared/anki/connect/"+p.name for p in (root/"third_party/anki-connect/plugin").iterdir() if p.is_file())
for name in ("anki","anki-connect"):
    base=root/"third_party"/name
    names.extend("shared/anki/licenses/"+name+"/"+p.relative_to(base).as_posix() for p in base.rglob("*")
                 if p.is_file() and "__pycache__" not in p.parts and p.suffix not in (".pyc",".pyo"))
content="\n".join(sorted(set(names)))+"\n"
target=root/"internal/distribution/anki-runtime-files.txt"
if args.check:
    if target.read_text(encoding="utf-8")!=content:raise ValueError("Anki payload inventory is stale")
else:
    target.write_text(content,encoding="utf-8")
print(f"Anki payload inventory: {len(set(names))} files")
