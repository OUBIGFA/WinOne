"""Explicit integration tests for the offline OCR byte protocol.

Set WINONE_OCR_RUNTIME to the prepared runtime. Fixtures contain generated text,
never screenshots or files from the user's desktop.
"""
import io
import json
import os
import pathlib
import struct
import subprocess
import unittest


class InteractiveSpeedTests(unittest.TestCase):
    # Windows can run a background process with EcoQoS (efficiency cores, low
    # clock). A user's recognition runs at full speed, then the prior policy
    # returns so idle maintenance stays power-efficient.
    def test_recognition_requests_full_speed_and_restores_the_policy(self):
        import ctypes
        import importlib.util
        source = pathlib.Path(__file__).resolve().parents[2] / 'modules/capture/Ocr/recognize.py'
        spec = importlib.util.spec_from_file_location('winone_recognize', source)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        before = module._power_throttling()
        with module._interactive():
            control, state = module._power_throttling()
            self.assertTrue(control & 1)
            self.assertFalse(state & 1)
        self.assertEqual(module._power_throttling(), before)


@unittest.skipUnless(os.environ.get('WINONE_OCR_RUNTIME'), 'explicit packaged OCR test')
class RecognitionProtocolTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.runtime = pathlib.Path(os.environ['WINONE_OCR_RUNTIME']).resolve()
        cls.source = pathlib.Path(__file__).resolve().parents[2] / 'modules/capture/Ocr/recognize.py'

    def image(self, text='Hello world 123'):
        from PIL import Image, ImageDraw, ImageFont
        image = Image.new('RGB', (500, 85), 'white')
        font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 28)
        ImageDraw.Draw(image).text((12, 8), text, font=font, fill='black')
        data = io.BytesIO()
        image.save(data, 'PNG')
        return data.getvalue()

    def run_worker(self, data, serve=True, device=None):
        command = [str(self.runtime / 'python/python.exe'), '-B', str(self.source), str(self.runtime / 'models')]
        if serve:
            command.append('--serve')
        environment = dict(os.environ)
        environment.pop('WINONE_OCR_DEVICE', None)
        if device:
            environment['WINONE_OCR_DEVICE'] = device
        result = subprocess.run(command, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=60, creationflags=0x08000000, env=environment)
        replies = [json.loads(line) for line in result.stdout.decode('utf-8').splitlines()]
        # Every successful reply names the inference device; compare it apart.
        devices = {reply.pop('device') for reply in replies if 'error' not in reply}
        self.assertLessEqual(devices, {'gpu', 'cpu'})
        self.devices = devices
        return result.returncode, replies

    @staticmethod
    def frame(data):
        return struct.pack('<I', len(data)) + data

    def test_prepare_then_text_and_resume_share_the_protocol(self):
        warm = struct.pack('<I', 0)
        code, replies = self.run_worker(warm + self.frame(self.image()) + warm + self.frame(self.image()))
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'prepared': True}, {'text': 'Hello world 123'},
                                  {'prepared': True}, {'text': 'Hello world 123'}])

    def test_blank_then_text_keeps_detection_enabled(self):
        # RapidOCR's rec-only fallback mutates its flags. The next screenshot
        # must reset them when the same model sessions are reused.
        code, replies = self.run_worker(self.frame(self.image('')) + self.frame(self.image()))
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'text': ''}, {'text': 'Hello world 123'}])

    def test_invalid_image_does_not_poison_the_next_request(self):
        code, replies = self.run_worker(self.frame(b'invalid image') + self.frame(self.image()))
        self.assertEqual(code, 0)
        self.assertEqual(len(replies), 2)
        self.assertIn('error', replies[0])
        self.assertEqual(replies[1], {'text': 'Hello world 123'})

    def test_oversized_frame_is_rejected_before_reading_its_payload(self):
        code, replies = self.run_worker(struct.pack('<I', 32 * 1024 * 1024 + 1))
        self.assertNotEqual(code, 0)
        self.assertEqual(replies, [{'error': 'ValueError'}])

    def test_incomplete_frame_reports_failure(self):
        code, replies = self.run_worker(struct.pack('<I', 20) + b'incomplete')
        self.assertNotEqual(code, 0)
        self.assertEqual(replies, [{'error': 'ValueError'}])

    def page(self, lines):
        from PIL import Image, ImageDraw, ImageFont
        image = Image.new('RGB', (1600, 900), 'white')
        font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 32)
        draw = ImageDraw.Draw(image)
        for index, line in enumerate(lines):
            draw.text((24, 24 + index * 70), line, font=font, fill='black')
        data = io.BytesIO()
        image.save(data, 'PNG')
        return data.getvalue()

    def test_small_selections_stay_on_the_resident_cpu_engine(self):
        # Creating GPU sessions costs more than recognizing a few lines.
        code, replies = self.run_worker(struct.pack('<I', 0) + self.frame(self.image()))
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'prepared': True}, {'text': 'Hello world 123'}])
        self.assertEqual(self.devices, {'cpu'})

    def test_large_page_uses_a_one_request_gpu_engine_or_cpu(self):
        lines = ['Screenshot OCR latency check 2026', '截图文字识别 第二行 测试', 'The quick brown fox jumps over the lazy dog']
        page = self.page(lines)
        # Two pages in one worker: the GPU engine is created per request, and
        # the forced CPU path must read the same lines.
        code, replies = self.run_worker(self.frame(page) + self.frame(page))
        self.assertEqual(code, 0)
        devices = self.devices
        code, forced = self.run_worker(self.frame(page), device='cpu')
        self.assertEqual(code, 0)
        self.assertEqual(self.devices, {'cpu'})
        # The layout joins Chinese words without spaces; compare characters.
        compact = lambda values: [''.join(value.split()) for value in values]
        for reply in replies + forced:
            self.assertEqual(compact(reply['text'].splitlines()), compact(lines))
        expected = os.environ.get('WINONE_OCR_EXPECT_DEVICE')
        if expected:
            self.assertEqual(devices, {expected})

    def test_standalone_capture_keeps_the_single_request_protocol(self):
        code, replies = self.run_worker(self.image(), serve=False)
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'text': 'Hello world 123'}])


if __name__ == '__main__':
    unittest.main()
