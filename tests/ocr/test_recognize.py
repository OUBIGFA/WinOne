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


@unittest.skipUnless(os.environ.get('WINONE_OCR_RUNTIME'), 'explicit packaged OCR test')
class RecognitionProtocolTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.runtime = pathlib.Path(os.environ['WINONE_OCR_RUNTIME']).resolve()
        cls.worker = cls.runtime.parent / 'WinOne.Capture.exe'

    def image(self, text='Hello world 123'):
        from PIL import Image, ImageDraw, ImageFont
        image = Image.new('RGB', (500, 85), 'white')
        font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 28)
        ImageDraw.Draw(image).text((12, 8), text, font=font, fill='black')
        data = io.BytesIO()
        image.save(data, 'PNG')
        return data.getvalue()

    def run_worker(self, data, serve=True, device=None):
        command = [str(self.worker), '--ocr-worker', str(self.runtime)]
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
        # A rec-only recovery must not disable detection for the next request.
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
        self.assertEqual(replies, [{'error': 'InvalidDataException'}])

    def test_incomplete_frame_reports_failure(self):
        code, replies = self.run_worker(struct.pack('<I', 20) + b'incomplete')
        self.assertNotEqual(code, 0)
        self.assertEqual(replies, [{'error': 'EndOfStreamException'}])

    def test_large_png_header_is_rejected_before_decoding(self):
        data = bytearray(self.image())
        data[16:24] = struct.pack('>II', 8000, 8000)
        code, replies = self.run_worker(self.frame(data) + self.frame(self.image()))
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'error': 'InvalidDataException'}, {'text': 'Hello world 123'}])

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
        for reply in replies + forced:
            self.assertEqual(''.join(reply['text'].split()), ''.join(''.join(lines).split()))
        expected = os.environ.get('WINONE_OCR_EXPECT_DEVICE')
        if expected:
            self.assertEqual(devices, {expected})

    def test_standalone_capture_keeps_the_single_request_protocol(self):
        code, replies = self.run_worker(self.image(), serve=False)
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'text': 'Hello world 123'}])

    def test_wrapped_copy_text_joins_while_inplace_keeps_line_boxes(self):
        from PIL import Image, ImageDraw, ImageFont
        image = Image.new('RGB', (800, 180), 'white')
        draw = ImageDraw.Draw(image)
        font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 28)
        lines = ['This paragraph wraps at the edge', 'and continues on the second line', 'with a short ending.']
        for index, line in enumerate(lines):
            draw.text((12, 8 + index * 40), line, font=font, fill='black')
        png = io.BytesIO()
        image.save(png, 'PNG')
        code, replies = self.run_worker(self.frame(png.getvalue()))
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'text': ' '.join(lines)}])
        result = subprocess.run([str(self.worker), '--ocr-worker', str(self.runtime), '--pin-layout'],
                                input=png.getvalue(), stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=60, creationflags=0x08000000)
        self.assertEqual(result.returncode, 0)
        layout = json.loads(result.stdout.decode('utf-8'))['layout']['lines']
        self.assertEqual([line['text'] for line in layout], lines)
        self.assertTrue(all(line['glyphs'] for line in layout))
        self.assertLess(layout[0]['box'][1], layout[1]['box'][1])
        self.assertLess(layout[1]['box'][1], layout[2]['box'][1])


if __name__ == '__main__':
    unittest.main()
