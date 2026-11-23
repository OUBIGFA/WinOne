# SPDX-License-Identifier: GPL-3.0-only
"""Offline OCR with an optional, parent-owned stream of framed image requests.

Engine choices follow HushSnap ppocr.py at 2d43f0e1f. The parent owns
cancellation and residency. A zero-length frame prepares both lazy sessions;
all other frames contain one image. Stdout is one JSON response per frame,
naming the inference device: "cpu" for the resident engine, "gpu" when a
large selection used a one-request DirectML engine.
"""
import contextlib
import ctypes
import functools
import gc
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


class _PowerThrottlingState(ctypes.Structure):
    _fields_ = [('Version', ctypes.c_ulong), ('ControlMask', ctypes.c_ulong), ('StateMask', ctypes.c_ulong)]


_PROCESS_POWER_THROTTLING = 4  # ProcessPowerThrottling information class
_EXECUTION_SPEED = 1           # PROCESS_POWER_THROTTLING_EXECUTION_SPEED
_kernel32 = ctypes.WinDLL('kernel32', use_last_error=True)
_kernel32.GetCurrentProcess.restype = ctypes.c_void_p


def _power_throttling() -> tuple[int, int]:
    state = _PowerThrottlingState(1, 0, 0)
    if not _kernel32.GetProcessInformation(ctypes.c_void_p(_kernel32.GetCurrentProcess()), _PROCESS_POWER_THROTTLING,
                                           ctypes.byref(state), ctypes.sizeof(state)):
        raise ctypes.WinError(ctypes.get_last_error())
    return state.ControlMask, state.StateMask


def _set_power_throttling(control: int, state: int) -> None:
    value = _PowerThrottlingState(1, control, state)
    if not _kernel32.SetProcessInformation(ctypes.c_void_p(_kernel32.GetCurrentProcess()), _PROCESS_POWER_THROTTLING,
                                           ctypes.byref(value), ctypes.sizeof(value)):
        raise ctypes.WinError(ctypes.get_last_error())


@contextlib.contextmanager
def _interactive():
    # This worker runs in the background, so Windows may give it EcoQoS
    # (efficiency cores, low clock). Measured on an i7-12700K, a one-line
    # selection took about 230 ms throttled and 16 ms at full speed. A user's
    # recognition opts out, as the capture worker does, and the prior policy
    # returns afterwards so idle maintenance stays power-efficient.
    # https://learn.microsoft.com/windows/win32/procthread/quality-of-service
    try:
        control, state = _power_throttling()
        _set_power_throttling(control | _EXECUTION_SPEED, state & ~_EXECUTION_SPEED)
    except OSError as error:
        # Speed only; recognition is still correct at the system's policy.
        print(f'OCR interactive priority unavailable: {error.winerror}', file=sys.stderr, flush=True)
        yield
        return
    try:
        yield
    finally:
        _set_power_throttling(control, state)


# Selections at least this large use the GPU. Below it the resident CPU
# engine wins: creating DirectML sessions takes about 0.4 s. Measured on
# generated pages (i7-12700K, RTX 3080 Ti): 1600x1000 with 30 lines took
# 2.0-2.5 s on CPU and about 1.2 s with GPU creation included.
GPU_MIN_PIXELS = 1_000_000


def _params(model_root: pathlib.Path) -> dict:
    from rapidocr import OCRVersion, ModelType
    det = model_root / 'PP-OCRv6_det_small.onnx'
    rec = model_root / 'PP-OCRv6_rec_small.onnx'
    if not det.is_file() or not rec.is_file():
        raise FileNotFoundError('OCR models are missing')
    return {
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


@functools.lru_cache(maxsize=1)
def _engine(model_root: pathlib.Path):
    # A Host-owned worker keeps only these CPU model sessions resident.
    # Captured images and recognized text are request-local and never cached
    # or written to disk.
    import onnxruntime
    # Windows builds of ONNX Runtime can emit ETW telemetry events; OCR is a
    # local feature and does not need them.
    onnxruntime.disable_telemetry_events()
    from rapidocr import RapidOCR
    return RapidOCR(params=_params(model_root))


@functools.lru_cache(maxsize=1)
def _gpu_shell(model_root: pathlib.Path):
    # The engine object and its pre/post-processing stay; its model sessions
    # do not. DirectML sessions cache intermediate buffers sized by the
    # largest input: about 0.9 GB committed when created and up to 5.5 GB
    # after one full-screen page, never returned while the session lives.
    # So they are attached for one request and released afterwards.
    from rapidocr import RapidOCR
    params = _params(model_root)
    # GPU dispatch has a fixed cost per call, so lines are batched. On
    # generated pages six per batch was 2-3x faster than one, with equal or
    # lower character error (0.49% vs 0.51%). The CPU engine keeps one.
    params['Rec.rec_batch_num'] = 6
    engine = RapidOCR(params=params)
    # RapidOCR 3.10.0 cannot pass DirectML options through its configuration;
    # these loaders and session attributes are private to that pinned version.
    models = (engine._load_det_model(), engine._load_rec_model())
    for model in models:
        model.session.session = None  # Free the CPU sessions the loaders built.
    return engine, models, (params['Det.model_path'], params['Rec.model_path'])


def _gpu_sessions(paths):
    import onnxruntime as ort
    from concurrent.futures import ThreadPoolExecutor
    options = ort.SessionOptions()
    options.log_severity_level = 3
    options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
    # DirectML supports neither memory patterns nor parallel execution.
    options.enable_mem_pattern = False
    options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
    # Metacommands made these models about 2x slower on the measured NVIDIA
    # driver (12 lines: 355 -> 158 ms; full screen: 3.4 -> 1.8 s). No CPU
    # provider is listed: a missing DirectX 12 device must fail here and use
    # the resident CPU engine, not run slowly on a second CPU session.
    providers = [('DmlExecutionProvider', {'device_id': 0, 'disable_metacommands': 'true'})]
    # Both sessions are built at once; each takes about 0.3 s.
    with ThreadPoolExecutor(2) as pool:
        return list(pool.map(lambda path: ort.InferenceSession(path, sess_options=options, providers=providers), paths))


_gpu_available = os.environ.get('WINONE_OCR_DEVICE') != 'cpu'  # cpu forces the fallback (tests, diagnosis).


def _pin_layout(engine, pixels):
    """Return geometry in source pixels. Nothing from the image is written to disk."""
    import math

    height, width = pixels.shape[:2]

    def bounds(quad):
        xs, ys = zip(*quad)
        x = max(0, min(width, math.floor(min(xs))))
        y = max(0, min(height, math.floor(min(ys))))
        right = max(x, min(width, math.ceil(max(xs))))
        bottom = max(y, min(height, math.ceil(max(ys))))
        return [x, y, right - x, bottom - y]

    result = engine(pixels, use_det=True, use_cls=False, use_rec=True,
                    return_word_box=True, return_single_char_box=True)
    lines = []
    for index, block in enumerate(result.to_json() or []):
        box = bounds(block['box'])
        if not block['txt'].strip() or not box[2] or not box[3]:
            continue
        # Keep genuine character boxes; some OCR backends only return lines.
        glyphs = []
        if result.word_results and index < len(result.word_results):
            for text, _, quad in result.word_results[index]:
                if text and quad is not None:
                    glyphs.append({'text': text, 'box': bounds(quad)})
        lines.append({'text': block['txt'], 'box': box, 'glyphs': glyphs})
    return {'lines': lines}


def _run(engine, pixels, layout=False):
    if layout:
        return _pin_layout(engine, pixels)
    from hushsnap_layout import compose_ppocr_structures, _is_vertical_json
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


def recognize(data: bytes, model_root: pathlib.Path, *, layout=False):
    global _gpu_available
    import numpy as np
    from PIL import Image

    with Image.open(io.BytesIO(data)) as image:
        if image.width * image.height > 40_000_000:
            raise ValueError('OCR image is too large')
        # PIL is RGB; the HushSnap inference path passes contiguous BGR pixels.
        pixels = np.asarray(image.convert('RGB'))[:, :, ::-1].copy()
        large = image.width * image.height >= GPU_MIN_PIXELS
    if large and _gpu_available:
        engine, models, paths = _gpu_shell(model_root)
        try:
            for model, session in zip(models, _gpu_sessions(paths)):
                model.session.session = session
            return _run(engine, pixels, layout), 'gpu'
        except Exception as error:
            # No usable device, a driver reset or device removal: this worker
            # stays on CPU. The Host's next worker tries the GPU again.
            _gpu_available = False
            print(f'OCR GPU unavailable, using CPU: {type(error).__name__}', file=sys.stderr, flush=True)
        finally:
            # Release the DirectML sessions and their cached buffers now.
            for model in models:
                model.session.session = None
            gc.collect()
    return _run(_engine(model_root), pixels, layout), 'cpu'


def prepare(model_root: pathlib.Path) -> str:
    # RapidOCR 3.10 creates inference sessions on first use, not construction.
    # Exercise the real detection/recognition path with generated text only;
    # a model constructor or an all-white detection pass would leave rec cold.
    from PIL import Image, ImageDraw
    with Image.new('RGB', (192, 48), 'white') as image:
        ImageDraw.Draw(image).text((8, 8), 'WinOne OCR 123', fill='black')
        with io.BytesIO() as png:
            image.save(png, 'PNG')
            device = recognize(png.getvalue(), model_root)[1]
    if _gpu_available:
        # Build the GPU engine shell now (about 0.4 s of CPU, no GPU memory),
        # so a large selection pays only for its DirectML sessions.
        _gpu_shell(model_root)
    return device


def _respond(data: bytes, model_root: pathlib.Path, *, layout=False) -> bool:
    try:
        with contextlib.redirect_stdout(sys.stderr):
            if not data:
                result = {'prepared': True, 'device': prepare(model_root)}
            else:
                with _interactive():
                    value, device = recognize(data, model_root, layout=True) if layout else recognize(data, model_root)
                result = {'layout' if layout else 'text': value, 'device': device}
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
            return 0 if _respond(data, model_root, layout='--pin-layout' in sys.argv[2:]) else 1
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
