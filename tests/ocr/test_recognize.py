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
        cls.source = pathlib.Path(__file__).resolve().parents[2] / 'modules/capture/Ocr/recognize.py'

    def image(self, text='Hello world 123'):
        from PIL import Image, ImageDraw, ImageFont
        image = Image.new('RGB', (500, 85), 'white')
        font = ImageFont.truetype(str(pathlib.Path(os.environ['WINDIR']) / 'Fonts/msyh.ttc'), 28)
        ImageDraw.Draw(image).text((12, 8), text, font=font, fill='black')
        data = io.BytesIO()
        image.save(data, 'PNG')
        return data.getvalue()

    def run_worker(self, data, serve=True):
        command = [str(self.runtime / 'python/python.exe'), '-B', str(self.source), str(self.runtime / 'models')]
        if serve:
            command.append('--serve')
        result = subprocess.run(command, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=30, creationflags=0x08000000)
        return result.returncode, [json.loads(line) for line in result.stdout.decode('utf-8').splitlines()]

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

    def test_standalone_capture_keeps_the_single_request_protocol(self):
        code, replies = self.run_worker(self.image(), serve=False)
        self.assertEqual(code, 0)
        self.assertEqual(replies, [{'text': 'Hello world 123'}])


if __name__ == '__main__':
    unittest.main()
