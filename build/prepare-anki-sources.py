"""Reproduce the corresponding-source ZIP from its exact locked input bytes."""
import argparse
import hashlib
import json
import zipfile
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument("--inputs",nargs="+",required=True)
parser.add_argument("--output",required=True)
parser.add_argument("--update",action="store_true")
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
spec_path=root/"third_party/anki/source.json"
spec=json.loads(spec_path.read_text(encoding="utf-8"))
manifest=spec["correspondingSources"]
output=Path(args.output).resolve()
if not output.is_relative_to(root/"_temp") and not output.is_relative_to(root/"dist"):
    raise ValueError("Output must remain within repository artifacts")
lookup={}
needed={Path(name).name for name in manifest["files"]}
for folder in args.inputs:
    for path in Path(folder).rglob("*"):
        if path.is_file() and path.name in needed:
            lookup.setdefault(path.name,[]).append(path)
verified={}
for name,want in manifest["files"].items():
    for path in lookup.get(Path(name).name,[]):
        if hashlib.sha256(path.read_bytes()).hexdigest()==want:
            verified[name]=path
            break
    else:
        raise ValueError("Missing exact corresponding source: "+name)
output.parent.mkdir(parents=True,exist_ok=True)
with zipfile.ZipFile(output,"w",compression=zipfile.ZIP_DEFLATED,compresslevel=1) as archive:
    for name,path in sorted(verified.items()):
        info=zipfile.ZipInfo(name,date_time=(1980,1,1,0,0,0))
        info.compress_type=zipfile.ZIP_DEFLATED
        info.external_attr=0o100644<<16
        archive.writestr(info,path.read_bytes(),compresslevel=1)
digest=hashlib.sha256(output.read_bytes()).hexdigest()
if args.update:
    manifest["sha256"]=digest
    spec_path.write_text(json.dumps(spec,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
elif digest!=manifest["sha256"]:
    raise ValueError("Source ZIP differs from its reviewed digest")
print(f"Corresponding sources: {len(verified)} inputs, SHA-256 {digest}")
