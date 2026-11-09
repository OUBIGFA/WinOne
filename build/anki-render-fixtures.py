"""Create real Anki card output for offline browser renderer verification."""
import argparse
import json
import struct
import sys
import wave
import zlib
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument("--library",required=True)
parser.add_argument("--output",required=True)
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
output=Path(args.output).resolve()
if not output.is_relative_to(root/"_temp"):raise ValueError("Use task temporary output")
output.mkdir(parents=True,exist_ok=True)
sys.path[:0]=[str(root/"modules/anki"),str(Path(args.library).resolve())]
from engine import Engine
engine=Engine(output/"profile")
fixtures=[]
try:
    engine.legacy("createDeck",{"deck":"Fixture"})
    engine.legacy("createModel",{"modelName":"Fixture","inOrderFields":["Front","Back"],"cardTemplates":[{"Name":"Card","Front":"{{Front}}","Back":"{{FrontSide}}<hr>{{Back}}"}]})
    engine.legacy("addNote",{"note":{"deckName":"Fixture","modelName":"Fixture","fields":{"Front":r"<h2>Formula</h2>\(x^2 + y^2 = z^2\)","Back":"A right triangle"}}})
    engine.legacy("guiDeckReview",{"name":"Fixture"})
    value=engine.invoke("winone.review")
    fixtures.append({"name":"math","body":value["rendered"]["questionHtml"],"css":value["rendered"]["css"],"media":value["mediaDirectory"]})
    def chunk(kind,data):return struct.pack(">I",len(data))+kind+data+struct.pack(">I",zlib.crc32(kind+data))
    width,height=480,240
    rows=[]
    for y in range(height):
        row=bytearray([0])
        for x in range(width):row.extend((38,38,38) if x in range(80,240) and y in range(60,150) else (220,220,220))
        rows.append(bytes(row))
    image=output/"diagram.png"
    image.write_bytes(b"\x89PNG\r\n\x1a\n"+chunk(b"IHDR",struct.pack(">IIBBBBB",width,height,8,2,0,0,0))+chunk(b"IDAT",zlib.compress(b"".join(rows)))+chunk(b"IEND",b""))
    engine.legacy("createDeck",{"deck":"Images"})
    did=engine.legacy("deckNamesAndIds")["Images"]
    engine.invoke("winone.image.save",{"path":str(image),"deckId":str(did),"header":"Image occlusion","occlusions":[{"ordinal":1,"shapes":[{"shape":"rect","properties":[{"name":"left","value":".16"},{"name":"top","value":".24"},{"name":"width","value":".36"},{"name":"height","value":".4"}]}]}]})
    engine.legacy("guiDeckReview",{"name":"Images"})
    value=engine.invoke("winone.review")
    fixtures.append({"name":"occlusion","body":value["rendered"]["questionHtml"],"css":value["rendered"]["css"],"media":value["mediaDirectory"]})
    engine.legacy("createDeck",{"deck":"Typing"})
    engine.legacy("createModel",{"modelName":"Typing","inOrderFields":["Word","Meaning"],"cardTemplates":[{"Name":"Type","Front":"{{Meaning}}<br>{{type:Word}}","Back":"{{FrontSide}}<hr>{{type:Word}}"}]})
    engine.legacy("addNote",{"note":{"deckName":"Typing","modelName":"Typing","fields":{"Word":"hello","Meaning":"greeting"}}})
    engine.legacy("guiDeckReview",{"name":"Typing"})
    value=engine.invoke("winone.review",{"typed":"hello"})
    fixtures.append({"name":"typing","body":value["rendered"]["answerHtml"],"css":value["rendered"]["css"],"media":value["mediaDirectory"]})
    (output/"fixtures.json").write_text(json.dumps(fixtures,indent=2),encoding="utf-8")
finally:
    engine.close()
print("Real Anki renderer fixtures prepared")
