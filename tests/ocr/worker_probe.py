"""Explicit worker latency/residency probe; generated PNGs and one owned process.

--prepare measures first use after background preparation separately from its
cost. --idle-seconds crosses the old expiry. --trim-working-set simulates OS
paging for this worker ONLY; it is not a claim of real suspend/resume testing.
"""
import argparse
import ctypes
import io
import json
import os
import pathlib
import queue
import statistics
import struct
import subprocess
import sys
import threading
import time
from ctypes import wintypes
from PIL import Image, ImageDraw, ImageFont

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from runtime_probe import Counters, memory, handles

parser = argparse.ArgumentParser()
parser.add_argument('runtime')
parser.add_argument('output')
parser.add_argument('--prepare', action='store_true')
parser.add_argument('--idle-seconds', type=float, default=0)
parser.add_argument('--trim-working-set', action='store_true')
args = parser.parse_args()
runtime, output = map(lambda value: pathlib.Path(value).resolve(), [args.runtime, args.output])
root = pathlib.Path(__file__).resolve().parents[2]
if not output.is_relative_to(root / '_temp') or not 0 <= args.idle_seconds <= 86400:
    raise ValueError('Choose an owned repository _temp output and a bounded idle time')
output.mkdir(parents=True, exist_ok=False)

cpu_times = ctypes.WinDLL('kernel32', use_last_error=True).GetProcessTimes
cpu_times.argtypes = [wintypes.HANDLE] + [ctypes.POINTER(wintypes.FILETIME)] * 4
cpu_times.restype = wintypes.BOOL
empty_ws = ctypes.WinDLL('psapi', use_last_error=True).EmptyWorkingSet
empty_ws.argtypes, empty_ws.restype = [wintypes.HANDLE], wintypes.BOOL

font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 24)
fixtures = []
for name, text, size in [('small', 'Hello 123', (220, 60)),
                         ('multiline', 'WinOne 文字识别\n第二行测试\nHello world', (500, 180))]:
    with Image.new('RGB', size, 'white') as image:
        ImageDraw.Draw(image).multiline_text((10, 4), text, font=font, fill='black')
        png = io.BytesIO()
        image.save(png, 'PNG')
        fixtures.append((name, text, png.getvalue()))

started = time.perf_counter()
child = subprocess.Popen([str(runtime / 'python/python.exe'), '-B', str(runtime / 'recognize.py'),
                          str(runtime / 'models'), '--serve'], stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE, creationflags=0x08000000)
replies = queue.Queue()
diagnostics = []

def receive():
    for line in child.stdout:
        replies.put(line)
    replies.put(None)

def stderr():
    # Only a byte count is reported; no image/text diagnostic payload is kept.
    total = 0
    for data in iter(lambda: child.stderr.read(4096), b''):
        total += len(data)
    diagnostics.append(total)

read_thread = threading.Thread(target=receive, daemon=True)
error_thread = threading.Thread(target=stderr, daemon=True)
read_thread.start()
error_thread.start()

def snapshot():
    value, count = Counters(), wintypes.DWORD()
    value.cb = ctypes.sizeof(value)
    times = [wintypes.FILETIME() for _ in range(4)]
    if not memory(int(child._handle), ctypes.byref(value), value.cb):
        raise ctypes.WinError(ctypes.get_last_error())
    if not handles(int(child._handle), ctypes.byref(count)):
        raise ctypes.WinError(ctypes.get_last_error())
    if not cpu_times(int(child._handle), *[ctypes.byref(item) for item in times]):
        raise ctypes.WinError(ctypes.get_last_error())
    return {'workingSetBytes': value.working_set, 'privateBytes': value.private_bytes,
            'peakWorkingSetBytes': value.peak_working_set, 'handles': count.value,
            'cpuSeconds': sum((item.dwHighDateTime << 32) | item.dwLowDateTime for item in times[2:]) / 1e7}

def request(data, expected):
    child.stdin.write(struct.pack('<I', len(data)) + data)
    child.stdin.flush()
    line = replies.get(timeout=60)
    if line is None:
        raise RuntimeError('OCR worker exited before responding')
    result = json.loads(line.decode('utf-8'))
    if result != expected:
        raise RuntimeError(f'Incorrect OCR protocol result: {result}')

samples, preparation, idle_report, resume_ms = [], None, None, None

def sample(phase, index):
    name, expected, png = fixtures[index % len(fixtures)]
    before = started if phase == 'initial' and index == 0 and not args.prepare else time.perf_counter()
    request(png, {'text': expected})
    samples.append({'phase': phase, 'case': name, 'milliseconds': (time.perf_counter() - before) * 1000,
                    'cold': phase == 'initial' and index == 0 and not args.prepare})

try:
    if args.prepare:
        request(b'', {'prepared': True})
        preparation = (time.perf_counter() - started) * 1000
    for index in range(12):
        sample('initial', index)
    ready = snapshot()
    if args.idle_seconds:
        # Let ONNX's bounded post-inference spinning finish before measuring
        # true idle CPU; this delay is measurement only, never product behavior.
        time.sleep(.5)
        before, idle_start = snapshot(), time.perf_counter()
        time.sleep(args.idle_seconds)
        after = snapshot()
        idle_report = {'seconds': time.perf_counter() - idle_start,
                       'cpuSeconds': after['cpuSeconds'] - before['cpuSeconds'], 'resources': after}
        for index in range(2):
            sample('after-idle', index)
    if args.trim_working_set:
        if not empty_ws(int(child._handle)):
            raise ctypes.WinError(ctypes.get_last_error())
        trimmed = snapshot()
        for index in range(2):
            sample('after-owned-working-set-trim', index)
        # The second trim models a resume notification arriving before input.
        if not empty_ws(int(child._handle)):
            raise ctypes.WinError(ctypes.get_last_error())
        before = time.perf_counter()
        request(b'', {'prepared': True})
        resume_ms = (time.perf_counter() - before) * 1000
        for index in range(2):
            sample('after-resume-preparation', index)
    final_resources = snapshot()
    child.stdin.close()
    child.wait(timeout=10)
    if child.returncode:
        raise RuntimeError('OCR worker failed to exit')
    read_thread.join(timeout=2)
    error_thread.join(timeout=2)
    report = {'scope': 'Generated text; real packaged interpreter/models/pipes. No real OS suspend or cold disk-cache claim.',
              'pid': child.pid, 'prepare': args.prepare, 'preparationMs': preparation,
              'samples': samples, 'warmMedianMs': statistics.median(item['milliseconds'] for item in samples[2:12]),
              'readyResources': ready, 'idle': idle_report, 'finalResources': final_resources,
              'trimmedResources': trimmed if args.trim_working_set else None, 'resumePreparationMs': resume_ms,
              'diagnosticBytes': sum(diagnostics), 'exitCode': child.returncode}
    (output / 'worker.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report))
finally:
    if child.poll() is None:
        child.kill()
        child.wait()
