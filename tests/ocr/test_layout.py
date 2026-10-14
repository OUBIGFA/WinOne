"""Reading order must survive extraction from HushSnap's Qt application."""
import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2] / 'modules/capture/Ocr'))


class LayoutTests(unittest.TestCase):
    def test_horizontal_lines_and_cjk(self):
        from hushsnap_layout import compose_ppocr_structures
        blocks = [
            {'text': '第二行', 'box': [[10, 50], [100, 50], [100, 70], [10, 70]]},
            {'text': 'Hello', 'box': [[10, 10], [60, 10], [60, 30], [10, 30]]},
            {'text': 'World', 'box': [[70, 10], [130, 10], [130, 30], [70, 30]]},
        ]
        text = '\n'.join(line.text for line in compose_ppocr_structures(blocks))
        self.assertIn('Hello World', text)
        self.assertTrue(text.endswith('第二行'))

    def test_empty_boxes_are_not_text(self):
        from hushsnap_layout import compose_ppocr_structures
        self.assertEqual(compose_ppocr_structures([{'text': 'invalid', 'box': []}]), [])

    def test_vertical_columns_read_right_to_left(self):
        from hushsnap_layout import compose_ppocr_structures
        blocks = [
            {'text': '左列', 'box': [[10, 10], [30, 10], [30, 100], [10, 100]]},
            {'text': '右列', 'box': [[60, 10], [80, 10], [80, 100], [60, 100]]},
        ]
        self.assertEqual([line.text for line in compose_ppocr_structures(blocks, True) if line.text], ['右列', '左列'])


if __name__ == '__main__':
    unittest.main()
