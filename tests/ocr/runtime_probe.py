"""Explicit offline integration/resource probe; writes only to the supplied output.

Run with the packaged Python and pass its Ocr directory and an ignored output
directory. All images and text are generated fixtures, never desktop content.
"""
import ctypes
from ctypes import wintypes
import io
import json
import os
import pathlib
import subprocess
import sys
import time
from PIL import Image, ImageDraw, ImageFont


class Counters(ctypes.Structure):
    _fields_ = [('cb', wintypes.DWORD), ('faults', wintypes.DWORD)] + [(name, ctypes.c_size_t) for name in
        ('peak_working_set', 'working_set', 'peak_paged', 'paged', 'peak_nonpaged', 'nonpaged', 'pagefile', 'peak_pagefile', 'private_bytes')]


memory = ctypes.WinDLL('psapi', use_last_error=True).GetProcessMemoryInfo
memory.argtypes = [wintypes.HANDLE, ctypes.POINTER(Counters), wintypes.DWORD]
memory.restype = wintypes.BOOL
handles = ctypes.WinDLL('kernel32', use_last_error=True).GetProcessHandleCount
handles.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
handles.restype = wintypes.BOOL


def main():
    runtime = pathlib.Path(sys.argv[1]).resolve()
    output = pathlib.Path(sys.argv[2]).resolve()
    output.mkdir(parents=True, exist_ok=True)
    font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 24)
    results = []
    for label, text, size in [('blank', '', (300,120)), ('small', 'Hello 123', (220,40)),
                              ('multiline', 'WinOne 文字识别\n第二行测试\nHello world', (500,180))]:
        image = Image.new('RGB', size, 'white')
        ImageDraw.Draw(image).multiline_text((10,4), text, font=font, fill='black')
        png = io.BytesIO(); image.save(png, 'PNG')
        started = time.perf_counter()
        child = subprocess.Popen([str(runtime/'python/python.exe'), '-B', str(runtime/'recognize.py')],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, creationflags=0x08000000)
        child.stdin.write(png.getvalue()); child.stdin.close(); child.stdin = None
        peak_ws = peak_private = peak_handles = 0
        while child.poll() is None:
            value = Counters(); value.cb = ctypes.sizeof(value); count = wintypes.DWORD()
            if memory(int(child._handle), ctypes.byref(value), value.cb):
                peak_ws = max(peak_ws, value.peak_working_set)
                peak_private = max(peak_private, value.private_bytes)
            if handles(int(child._handle), ctypes.byref(count)):
                peak_handles = max(peak_handles, count.value)
            if time.perf_counter() - started > 60:
                child.kill(); child.wait(); raise RuntimeError('OCR timed out')
            time.sleep(0.02)
        stdout, stderr = child.communicate()
        actual = json.loads(stdout.decode('utf-8'))
        if child.returncode != 0 or actual.get('text') != text:
            raise RuntimeError(f'{label}: unexpected OCR result {actual}, exit {child.returncode}')
        results.append({'case':label, 'seconds':time.perf_counter()-started, 'peakWorkingSetBytes':peak_ws,
            'peakPrivateBytes':peak_private, 'peakHandles':peak_handles, 'exitCode':child.returncode,
            'diagnosticBytes':len(stderr)})
    (output/'ocr-resources.json').write_text(json.dumps(results, indent=2, ensure_ascii=False)+'\n', encoding='utf-8')
    print(json.dumps(results, ensure_ascii=False))


if __name__ == '__main__':
    main()
