# SPDX-License-Identifier: GPL-3.0-only
"""One offline OCR request per process; image bytes never need a disk file.

Engine choices follow HushSnap ppocr.py at 2d43f0e1f. The parent owns
cancellation and process lifetime. Stdout contains exactly one JSON response.
"""
import contextlib
import io
import json
import os
import pathlib
import sys

# HushSnap sets this before importing NumPy. Screenshot preprocessing does
# not need a second full CPU thread pool alongside ONNX Runtime.
os.environ.setdefault('OPENBLAS_NUM_THREADS', '1')


def recognize(data: bytes, model_root: pathlib.Path) -> str:
    import numpy as np
    from PIL import Image
    from rapidocr import RapidOCR, OCRVersion, ModelType
    from hushsnap_layout import compose_ppocr_structures, _is_vertical_json

    det = model_root / 'PP-OCRv6_det_small.onnx'
    rec = model_root / 'PP-OCRv6_rec_small.onnx'
    if not det.is_file() or not rec.is_file():
        raise FileNotFoundError('OCR models are missing')
    with Image.open(io.BytesIO(data)) as image:
        if image.width * image.height > 40_000_000:
            raise ValueError('OCR image is too large')
        # PIL is RGB; the HushSnap inference path passes contiguous BGR pixels.
        pixels = np.asarray(image.convert('RGB'))[:, :, ::-1].copy()
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
    engine = RapidOCR(params=params)
    result = engine(pixels)
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


def main() -> int:
    sys.stdout.reconfigure(encoding='utf-8')
    sys.stderr.reconfigure(encoding='utf-8')
    try:
        data = sys.stdin.buffer.read(64 * 1024 * 1024 + 1)
        if not data or len(data) > 64 * 1024 * 1024:
            raise ValueError('Invalid OCR image size')
        model_root = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else pathlib.Path(__file__).parent / 'models'
        with contextlib.redirect_stdout(sys.stderr):
            text = recognize(data, model_root)
        print(json.dumps({'text': text}, ensure_ascii=False))
        return 0
    except Exception as error:
        # Never echo image bytes or recognition text into diagnostics.
        print(json.dumps({'error': type(error).__name__}, ensure_ascii=False))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
