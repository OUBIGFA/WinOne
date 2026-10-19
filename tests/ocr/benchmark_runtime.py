"""Offline comparison of the production adapter using generated screenshots."""
import argparse
import importlib.util
import io
import json
import os
import pathlib
import statistics
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument('--runtime', required=True)
parser.add_argument('--source', required=True)
parser.add_argument('--package')
parser.add_argument('--models')
parser.add_argument('--tier', default='small')
parser.add_argument('--threads', type=int, default=8)
parser.add_argument('--batch', type=int, default=1)
parser.add_argument('--reuse', action='store_true')
parser.add_argument('--output', required=True)
args = parser.parse_args()
os.environ['OPENBLAS_NUM_THREADS'] = '1'
if args.package:
    sys.path.insert(0, str(pathlib.Path(args.package).resolve()))
sys.path.insert(0, str(pathlib.Path(args.source).resolve()))
started = time.perf_counter()
import rapidocr
from PIL import Image, ImageDraw, ImageFont
import_ms = (time.perf_counter() - started) * 1000
import recognize as adapter
recognize = adapter.recognize
real_engine = rapidocr.RapidOCR
engine = None
build_ms = []


class ResetRequestFlags:
    def __init__(self, value):
        self.value = value

    def __call__(self, image, **kwargs):
        # RapidOCR retains call overrides; a blank-image rec-only fallback must
        # not turn detection off for the next independent screenshot.
        return self.value(image, **({'use_det': True, 'use_cls': False, 'use_rec': True} | kwargs))


def construct(params):
    global engine
    if args.reuse and engine is not None:
        return engine
    params = dict(params)
    params['EngineConfig.onnxruntime.intra_op_num_threads'] = args.threads
    params['Rec.rec_batch_num'] = args.batch
    if args.tier != 'small':
        for stage in ['Det', 'Rec']:
            params[f'{stage}.model_type'] = rapidocr.ModelType(args.tier)
            params[f'{stage}.model_path'] = str(pathlib.Path(args.models) / f'PP-OCRv6_{stage.lower()}_{args.tier}.onnx')
    start = time.perf_counter()
    engine = ResetRequestFlags(real_engine(params=params))
    build_ms.append((time.perf_counter() - start) * 1000)
    return engine


rapidocr.RapidOCR = construct
font_root = pathlib.Path(os.environ['WINDIR']) / 'Fonts'
fixtures = [
    ('blank', '', (400, 120), 24, False),
    ('english', 'Hello world 123', (420, 70), 28, False),
    ('chinese', 'WinOne 文字识别\n第二行测试\nHello world', (500, 180), 24, False),
    ('small_ui', '设置  文件  编辑  帮助\n自动识别并复制到剪贴板\n版本 2026.10.15', (420, 100), 14, False),
    ('dark_ui', '快速截图与文字识别\n保存成功 · Ctrl + Shift + A\nCindy Chat 2026', (550, 150), 22, True),
    ('punctuation', '订单号：202610150018\n金额 ¥1,234.56 (含税)\nEmail: test@example.com', (650, 170), 25, False),
    ('code', 'function add(a, b) {\n    return a + b;\n}\nconst value = add(12, 34);', (640, 190), 24, False),
    ('long_line', '这是一个较长的中英文混合句子，用来检查截图文字是否完整。Open source OCR 1234567890', (1500, 85), 24, False),
    ('document', '\n'.join(f'第 {n:02d} 行：连续识别与内存复用测试，Hello world 2026。' for n in range(1, 19)), (1100, 800), 28, False),
]


def distance(a, b):
    row = list(range(len(b) + 1))
    for i, ac in enumerate(a, 1):
        new = [i]
        for j, bc in enumerate(b, 1):
            new.append(min(new[-1]+1, row[j]+1, row[j-1]+(ac != bc)))
        row = new
    return row[-1]


results = []
model_root = pathlib.Path(args.runtime).resolve() / 'models'
for label, expected, size, font_size, dark in fixtures:
    font = ImageFont.truetype(str(font_root / ('consola.ttf' if label == 'code' else 'msyh.ttc')), font_size)
    image = Image.new('RGB', size, '#202124' if dark else 'white')
    ImageDraw.Draw(image).multiline_text((10, 4), expected, font=font, spacing=10, fill='white' if dark else 'black')
    png = io.BytesIO()
    image.save(png, 'PNG')
    elapsed = []
    for _ in range(4):
        if not args.reuse and hasattr(adapter, '_engine'):
            adapter._engine.cache_clear()
        start = time.perf_counter()
        actual = recognize(png.getvalue(), model_root)
        elapsed.append((time.perf_counter() - start) * 1000)
    clean_expected, clean_actual = ''.join(expected.split()), ''.join(actual.split())
    results.append({'case': label, 'firstMs': elapsed[0], 'medianMs': statistics.median(elapsed[1:]),
                    'expected': expected, 'actual': actual, 'characters': len(clean_expected),
                    'errors': distance(clean_expected, clean_actual)})
    print(f'{label}: {statistics.median(elapsed[1:]):.1f} ms, {results[-1]["errors"]} character errors', flush=True)
summary = {'configuration': vars(args), 'importMs': import_ms, 'buildMs': build_ms,
           'cases': results, 'totalErrors': sum(case['errors'] for case in results),
           'totalCharacters': sum(case['characters'] for case in results)}
pathlib.Path(args.output).write_text(json.dumps(summary, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
print(json.dumps({key: value for key, value in summary.items() if key not in ['cases', 'buildMs']}, ensure_ascii=False))
print(json.dumps([{key: value for key, value in case.items() if key not in ['expected', 'actual']} for case in results], ensure_ascii=False))
