"""Compare shipped OCR with native-compiled OCR using generated images only."""
import argparse
import ctypes
import hashlib
import io
import json
import os
from pathlib import Path
import struct
import subprocess
from ctypes import wintypes
from PIL import Image, ImageDraw, ImageFont

parser = argparse.ArgumentParser()
parser.add_argument('--baseline', required=True)
parser.add_argument('--candidate', required=True)
parser.add_argument('--runtime', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
output = Path(args.output).resolve()
if not output.is_relative_to(root / '_temp'):
    raise ValueError('Choose a task-owned repository _temp output')
output.mkdir(parents=True, exist_ok=False)
runtime = Path(args.runtime).resolve()
candidate = Path(args.candidate).resolve()
baseline = Path(args.baseline).resolve()
font_path = str(Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc')
models_before = {str(p.relative_to(runtime)): hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in runtime.rglob('*') if p.is_file()}

# A Native AOT executable has no CLR header or external CoreCLR/JIT host.
binary = candidate.read_bytes()
pe = struct.unpack_from('<I', binary, 0x3C)[0]
optional = pe + 24
if struct.unpack_from('<H', binary, optional)[0] != 0x20B:
    raise AssertionError('Expected a Windows x64 native executable')
if struct.unpack_from('<II', binary, optional + 112 + 14 * 8) != (0, 0):
    raise AssertionError('Candidate still has a managed CLR header')

samples = [
    ('short', 'Hello 123', (240, 70), 24),
    ('mixed', 'WinOne 文字识别\n第二行测试\nHello world', (520, 190), 24),
    ('paragraph', '这是一段因为截图宽度而\n换行的文字，复制后应当连续。', (600, 140), 25),
    ('code', 'if (ready) {\n    return value;\n}', (500, 180), 24),
    ('url', '地址 https://example.com/a?q=123\nEnglish 中文 123', (620, 140), 22),
    ('japanese', '日本語の文字認識\n東京 こんにちは', (500, 150), 26),
    ('small-font', 'Small text 小号文字 0123456789', (450, 80), 15),
    ('large', 'WinOne 原生识别测试\nLarge image OCR 123', (1400, 820), 36),
]

def recognize(executable, data, layout, cpu):
    env = dict(os.environ)
    if cpu:
        env['WINONE_OCR_DEVICE'] = 'cpu'
    else:
        env.pop('WINONE_OCR_DEVICE', None)
    command = [str(executable), '--ocr-worker', str(runtime)]
    if layout:
        command.append('--pin-layout')
    result = subprocess.run(command, input=data, capture_output=True, timeout=90,
                            env=env, creationflags=0x08000000)
    if result.returncode:
        raise RuntimeError(f'OCR process failed ({executable.name}, code {result.returncode}): '
                           + result.stdout.decode('utf-8', 'replace'))
    return json.loads(result.stdout)

results = []
for name, text, size, font_size in samples:
    image = Image.new('RGB', size, 'white')
    ImageDraw.Draw(image).multiline_text((10, 8), text, font=ImageFont.truetype(font_path, font_size), fill='black')
    formats = ['PNG', 'BMP', 'JPEG', 'TIFF', 'GIF'] if name == 'mixed' else ['PNG']
    for image_format in formats:
        buffer = io.BytesIO()
        image.save(buffer, image_format)
        for layout in (False, True):
            expected = recognize(baseline, buffer.getvalue(), layout, True)
            actual = recognize(candidate, buffer.getvalue(), layout, True)
            if actual != expected:
                (output / 'mismatch.json').write_text(json.dumps({'name': name, 'format': image_format,
                    'layout': layout, 'baseline': expected, 'candidate': actual}, ensure_ascii=False, indent=2), 'utf-8')
                raise AssertionError(f'Native OCR differs: {name} {image_format} layout={layout}')
            results.append({'name': name, 'format': image_format, 'layout': layout, 'matched': True})
        if name == 'large':
            expected = recognize(baseline, buffer.getvalue(), True, False)
            actual = recognize(candidate, buffer.getvalue(), True, False)
            if actual != expected:
                raise AssertionError('Native OCR differs on automatic GPU path')
            results.append({'name': name, 'format': image_format, 'layout': True, 'device': actual['device'], 'matched': True})

# Keep a native worker alive long enough to inspect its real loaded libraries.
child = subprocess.Popen([str(candidate), '--ocr-worker', str(runtime), '--serve'],
                         stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                         creationflags=0x08000000)
try:
    child.stdin.write(struct.pack('<I', 0)); child.stdin.flush()
    prepared = json.loads(child.stdout.readline())
    if prepared != {'prepared': True, 'device': 'cpu'}:
        raise AssertionError(f'Native preparation differs: {prepared}')
    psapi = ctypes.WinDLL('psapi', use_last_error=True)
    enum = psapi.EnumProcessModulesEx
    enum.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.HMODULE), wintypes.DWORD,
                     ctypes.POINTER(wintypes.DWORD), wintypes.DWORD]
    enum.restype = wintypes.BOOL
    filename = psapi.GetModuleFileNameExW
    filename.argtypes = [wintypes.HANDLE, wintypes.HMODULE, wintypes.LPWSTR, wintypes.DWORD]
    filename.restype = wintypes.DWORD
    modules, needed = (wintypes.HMODULE * 1024)(), wintypes.DWORD()
    if not enum(int(child._handle), modules, ctypes.sizeof(modules), ctypes.byref(needed), 3):
        raise ctypes.WinError(ctypes.get_last_error())
    if needed.value > ctypes.sizeof(modules):
        raise AssertionError('Module inventory was truncated')
    names = []
    for module in modules[:needed.value // ctypes.sizeof(wintypes.HMODULE)]:
        path = ctypes.create_unicode_buffer(32768)
        if not filename(int(child._handle), module, path, len(path)):
            raise ctypes.WinError(ctypes.get_last_error())
        names.append(Path(path.value).name.lower())
    prohibited = {'coreclr.dll', 'clrjit.dll', 'hostfxr.dll', 'hostpolicy.dll', 'system.windows.forms.dll'}
    if prohibited.intersection(names):
        raise AssertionError('Native OCR loaded a managed runtime')
finally:
    child.stdin.close()
    try:
        exit_code = child.wait(timeout=15)
    except subprocess.TimeoutExpired:
        child.kill(); child.wait()
        raise
    if exit_code:
        raise AssertionError('Native OCR worker did not exit normally')

models_after = {str(p.relative_to(runtime)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in runtime.rglob('*') if p.is_file()}
if models_before != models_after:
    raise AssertionError('OCR model/runtime files changed')
report = {'cases': results, 'count': len(results), 'nativePE': True, 'loadedModules': names,
          'externalDotNetLoaded': False, 'modelsUnchanged': True,
          'baselineSHA256': hashlib.sha256(baseline.read_bytes()).hexdigest(),
          'candidateSHA256': hashlib.sha256(candidate.read_bytes()).hexdigest()}
(output / 'equivalence.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), 'utf-8')
print(f'PASS: {len(results)} real-image results match, native PE, no external .NET loaded, model hashes unchanged')
