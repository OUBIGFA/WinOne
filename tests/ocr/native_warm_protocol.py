"""Exercise the shipped NativeAOT worker's alternating text/layout protocol."""
import argparse, io, json, os, struct, subprocess, time
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont

p=argparse.ArgumentParser();p.add_argument('--capture',required=True);p.add_argument('--output',required=True);a=p.parse_args()
root=Path(__file__).resolve().parents[2];out=Path(a.output).resolve()
if not out.is_relative_to(root/'_temp'):raise ValueError('Output must be task-local')
out.mkdir(parents=True,exist_ok=True)
image=Image.new('RGB',(640,180),'white');draw=ImageDraw.Draw(image)
font=ImageFont.truetype(str(Path(os.environ['WINDIR'])/'Fonts'/'arial.ttf'),30)
draw.text((20,20),'WinOne OCR 123',font=font,fill='black');draw.text((20,85),'Warm layout and text',font=font,fill='black')
b=io.BytesIO();image.save(b,format='PNG');png=b.getvalue()
capture=Path(a.capture).resolve();events=[]
env=dict(os.environ,TEMP=str(out),TMP=str(out),WINONE_OCR_DEVICE='cpu')
with (out/'stderr.log').open('wb') as errors:
    proc=subprocess.Popen([str(capture/'WinOne.Ocr.exe'),'--ocr-worker',str(capture/'Ocr'),'--serve'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=errors,env=env,creationflags=0x08000000)
    try:
        def request(data,layout=False):
            started=time.perf_counter();proc.stdin.write(struct.pack('<I',len(data)|(0x80000000 if layout else 0))+data);proc.stdin.flush()
            raw=proc.stdout.readline()
            if not raw:raise AssertionError(f'Worker exited: {proc.poll()}')
            result=json.loads(raw)
            if 'error' in result:raise AssertionError(result)
            events.append({'mode':'prepare' if not data else 'layout' if layout else 'text','ms':round((time.perf_counter()-started)*1000,2),'pid':proc.pid})
            return result
        assert request(b'')['prepared']
        for _ in range(3):
            text=request(png)['text'];assert 'WinOne' in text and '123' in text,text
            lines=request(png,True)['layout']['lines'];assert len(lines)>=2,lines
            assert any('123' in line['text'] for line in lines),lines
            assert all(len(line['box'])==4 and line['box'][2]>0 and line['box'][3]>0 for line in lines),lines
            assert any(line['glyphs'] for line in lines),'Single-character coordinates missing'
        proc.stdin.close();assert proc.wait(timeout=10)==0
        result={'passed':True,'events':events,'sameProcess':len({e['pid'] for e in events})==1}
    finally:
        if proc.poll() is None:proc.kill();proc.wait()
(out/'result.json').write_text(json.dumps(result,indent=2),encoding='utf-8')
print(json.dumps(result))
