"""Verify native capture payloads and their actual PE dependency closure."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import struct

parser=argparse.ArgumentParser();parser.add_argument('capture');parser.add_argument('--report');args=parser.parse_args()
root=Path(__file__).resolve().parents[1];capture=Path(args.capture).resolve()
required=(root/'internal/distribution/native_capture_runtime_paths.txt').read_text('utf-8').split()
for item in required:
    if not (capture/Path(item).name).is_file():raise ValueError('Missing native capture runtime: '+item)
for item in ['WinOne.Recording.dll','recording-runtime.json']:
    if not (capture/item).is_file():raise ValueError('Missing existing recording runtime: '+item)
ocr=json.loads((root/'third_party/ocr-runtime.json').read_text('utf-8'))
for item in ocr['files']:
    file=capture/'Ocr'/item['path']
    if hashlib.sha256(file.read_bytes()).hexdigest()!=item['sha256']:raise ValueError('OCR model/runtime changed: '+item['path'])
for prohibited in ['coreclr.dll','clrjit.dll','hostfxr.dll','hostpolicy.dll','System.Private.CoreLib.dll','System.Windows.Forms.dll','WinOne.Capture.dll','WinOne.Capture.runtimeconfig.json']:
    if (capture/prohibited).exists():raise ValueError('Managed screenshot dependency is still bundled: '+prohibited)
def inspect(path):
    data=path.read_bytes();offset=struct.unpack_from('<I',data,0x3c)[0]
    if b'winone_recording_synthetic_test\0' in data:raise ValueError('Generated recording source in product payload: '+path.name)
    if data[offset:offset+4]!=b'PE\0\0':raise ValueError('Not a PE image: '+path.name)
    sections=struct.unpack_from('<H',data,offset+6)[0];optional=offset+24
    size=struct.unpack_from('<H',data,offset+20)[0]
    if struct.unpack_from('<H',data,optional)[0]!=0x20b:raise ValueError('Native capture must be x64: '+path.name)
    directories=optional+112
    if struct.unpack_from('<II',data,directories+14*8)!=(0,0):raise ValueError('Managed CLR header found: '+path.name)
    def file_offset(rva):
        for index in range(sections):
            header=optional+size+index*40
            virtual_size,virtual,raw_size,raw=struct.unpack_from('<IIII',data,header+8)
            if virtual<=rva<virtual+max(virtual_size,raw_size):return raw+rva-virtual
        raise ValueError(f'Invalid PE section address {path.name}: {rva}')
    def string(rva):
        start=file_offset(rva);return data[start:data.index(b'\0',start)].decode('ascii').lower()
    export_rva,_=struct.unpack_from('<II',data,directories)
    if export_rva:
        exports=file_offset(export_rva)
        count,names=struct.unpack_from('<I',data,exports+24)[0],struct.unpack_from('<I',data,exports+32)[0]
        if count>65536:raise ValueError('Invalid native export count')
        names_offset=file_offset(names) if count else 0
        for index in range(count):
            name=string(struct.unpack_from('<I',data,names_offset+index*4)[0])
            if name in ['winonenativecapturefixture','winonenativerenderedframe','winone_recording_synthetic_test']:
                raise ValueError('Private test entrypoint in product payload: '+name)
    imports=[]
    for directory,step,name_index in [(1,20,3),(13,32,1)]:
        rva,length=struct.unpack_from('<II',data,directories+directory*8)
        if not rva:continue
        start=file_offset(rva)
        while any(data[start:start+step]):
            values=struct.unpack_from('<'+'I'*(step//4),data,start)
            if directory==13 and not values[0]&1:raise ValueError('Unsupported absolute delay import')
            imports.append(string(values[name_index]));start+=step
    return sorted(set(imports))
local={p.name.lower():p for p in capture.iterdir() if p.is_file() and p.suffix.lower() in ('.exe','.dll')}
system=Path(os.environ['WINDIR'])/'System32';results=[]
for name,path in sorted(local.items()):
    if 'probe' in name or 'tests' in name:raise ValueError('Test program in product capture payload: '+name)
    dependencies=inspect(path)
    for dependency in dependencies:
        if dependency.startswith(('api-ms-win-','ext-ms-win-')):continue
        if dependency not in local and not (system/dependency).is_file():raise ValueError(f'Missing native dependency: {name} -> {dependency}')
        if dependency.startswith(('msvcp','vcruntime')) and dependency not in local:raise ValueError('C++ runtime must be app-local: '+dependency)
    results.append({'file':name,'bytes':path.stat().st_size,'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'imports':dependencies})
catalog=json.loads((root/'third_party/capture-native-notices.json').read_text('utf-8'))
for component in catalog['components']:
    for filename in component.get('runtimeFiles',[]):
        if not (capture/filename).is_file():raise ValueError('Recorded native runtime is missing: '+filename)
    for notice in component['notices']:
        if hashlib.sha256((root/'third_party'/notice['file']).read_bytes()).hexdigest()!=notice['sha256']:raise ValueError('Native dependency notice changed')
record={'native':True,'externalDotNetRequired':False,'files':results,'payloadBytes':sum(p.stat().st_size for p in capture.rglob('*') if p.is_file()),'noticeComponents':len(catalog['components'])}
if args.report:Path(args.report).write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n','utf-8')
print(f'PASS: {len(results)} x64 native images; complete DLL closure; no external .NET; {len(catalog["components"])} original notice records')
