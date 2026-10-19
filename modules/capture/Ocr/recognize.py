# SPDX-License-Identifier: GPL-3.0-only
"""Offline OCR with an optional, parent-owned stream of framed image requests.

Engine choices follow HushSnap ppocr.py at 2d43f0e1f. The parent owns
cancellation and residency. A zero-length frame prepares both lazy sessions;
all other frames contain one image. Stdout is one JSON response per frame.
"""
import contextlib
import functools
import io
import json
import os
import pathlib
import struct
import sys

# HushSnap sets this before importing NumPy. Screenshot preprocessing does
# not need a second full CPU thread pool alongside ONNX Runtime.
os.environ.setdefault('OPENBLAS_NUM_THREADS', '1')
MAX_IMAGE_BYTES = 32 * 1024 * 1024


@functools.lru_cache(maxsize=1)
def _engine(model_root: pathlib.Path):
    # A Host-owned worker reuses only model sessions. Captured images and
    # recognized text are request-local and never cached or written to disk.
    from rapidocr import RapidOCR, OCRVersion, ModelType
    det = model_root / 'PP-OCRv6_det_small.onnx'
    rec = model_root / 'PP-OCRv6_rec_small.onnx'
    if not det.is_file() or not rec.is_file():
        raise FileNotFoundError('OCR models are missing')
    params = {
        'Det.ocr_version': OCRVersion.PPOCRV6,
        'Det.model_type': ModelType.SMALL,
        'Det.model_path': str(det),
        'Rec.ocr_version': OCRVersion.PPOCRV6,
        'Rec.model_type': ModelType.SMALL,
        'Rec.model_path': str(rec),
        'Global.use_cls': False,
        'Global.use_preprocess_img': False,
        'Global.use_vertical_padding': False,
        'Global.log_level': 'error',
        'Rec.rec_batch_num': 1,
        'Det.mean': [0.485, 0.456, 0.406],
        'Det.std': [0.229, 0.224, 0.225],
        'Det.limit_side_len': 32,
        'Det.use_dilation': False,
        'EngineConfig.onnxruntime.intra_op_num_threads': min(8, max(1, (os.cpu_count() or 2) // 2)),
        'EngineConfig.onnxruntime.inter_op_num_threads': 1,
        'EngineConfig.onnxruntime.enable_cpu_mem_arena': False,
    }
    return RapidOCR(params=params)


def recognize(data: bytes, model_root: pathlib.Path) -> str:
    import numpy as np
    from PIL import Image
    from hushsnap_layout import compose_ppocr_structures, _is_vertical_json

    with Image.open(io.BytesIO(data)) as image:
        if image.width * image.height > 40_000_000:
            raise ValueError('OCR image is too large')
        # PIL is RGB; the HushSnap inference path passes contiguous BGR pixels.
        pixels = np.asarray(image.convert('RGB'))[:, :, ::-1].copy()
    engine = _engine(model_root)
    # RapidOCR retains per-call switches. Reset detection after a previous
    # rec-only fallback, otherwise a blank selection breaks the next request.
    result = engine(pixels, use_det=True, use_cls=False, use_rec=True)
    blocks = result.to_json() or []
    if not blocks:
        # HushSnap's rec-only fallback recovers short single-line selections.
        fallback = engine(pixels, use_det=False, use_cls=False)
        return '\n'.join(text for text, score in zip(fallback.txts or [], fallback.scores or []) if score >= 0.5).strip()
    lines = compose_ppocr_structures(
        [{'text': block['txt'], 'box': block['box']} for block in blocks],
        is_vertical=_is_vertical_json(blocks),
    )
    return '\n'.join(line.text for line in lines).rstrip()


def prepare(model_root: pathlib.Path) -> None:
    # RapidOCR 3.10 creates inference sessions on first use, not construction.
    # Exercise the real detection/recognition path with generated text only;
    # a model constructor or an all-white detection pass would leave rec cold.
    from PIL import Image, ImageDraw
    with Image.new('RGB', (192, 48), 'white') as image:
        ImageDraw.Draw(image).text((8, 8), 'WinOne OCR 123', fill='black')
        with io.BytesIO() as png:
            image.save(png, 'PNG')
            recognize(png.getvalue(), model_root)


def _respond(data: bytes, model_root: pathlib.Path) -> bool:
    try:
        with contextlib.redirect_stdout(sys.stderr):
            if not data:
                prepare(model_root)
                result = {'prepared': True}
            else:
                result = {'text': recognize(data, model_root)}
        print(json.dumps(result, ensure_ascii=False), flush=True)
        return True
    except Exception as error:
        # Never echo image bytes or recognition text into diagnostics.
        print(json.dumps({'error': type(error).__name__}), flush=True)
        return False


def main() -> int:
    sys.stdout.reconfigure(encoding='utf-8')
    sys.stderr.reconfigure(encoding='utf-8')
    try:
        model_root = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else pathlib.Path(__file__).parent / 'models'
        if '--serve' not in sys.argv[2:]:
            data = sys.stdin.buffer.read(MAX_IMAGE_BYTES + 1)
            if not data or len(data) > MAX_IMAGE_BYTES:
                raise ValueError('Invalid OCR image size')
            return 0 if _respond(data, model_root) else 1
        while True:
            header = sys.stdin.buffer.read(4)
            if not header:
                return 0
            if len(header) != 4:
                raise ValueError('Incomplete OCR frame')
            length, = struct.unpack('<I', header)
            if length > MAX_IMAGE_BYTES:
                raise ValueError('Invalid OCR image size')
            data = sys.stdin.buffer.read(length)
            if len(data) != length:
                raise ValueError('Incomplete OCR frame')
            _respond(data, model_root)
            del data  # Do not retain a screenshot while waiting for another one.
    except Exception as error:
        print(json.dumps({'error': type(error).__name__}), flush=True)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
