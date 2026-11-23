"""Exercise the shipped Tools3000 ABI with generated pixels and offline translations."""
import ctypes as C
import pathlib
import sys
import unittest

class Rect(C.Structure):
    _fields_ = [(field, C.c_int) for field in ('x', 'y', 'width', 'height')]

class PinEngineTest(unittest.TestCase):
    def test_segment_render_and_restore_source(self):
        engine = C.CDLL(self.library_path)
        engine.pin_create.argtypes = [C.c_void_p, C.c_int, C.c_int, C.c_int]
        engine.pin_create.restype = C.c_void_p
        engine.pin_add.argtypes = [C.c_void_p, C.c_char_p, Rect, C.POINTER(Rect), C.c_int, C.c_int, C.c_int]
        engine.pin_prepare.argtypes = [C.c_void_p]
        engine.pin_text.argtypes = [C.c_void_p, C.c_int]
        engine.pin_text.restype = C.c_char_p
        engine.pin_translated.argtypes = [C.c_void_p, C.c_int, C.c_char_p]
        engine.pin_render.argtypes = [C.c_void_p, C.c_char_p, C.c_void_p, C.c_int]
        engine.pin_destroy.argtypes = [C.c_void_p]
        width, height = 360, 100
        pixels = (C.c_ubyte * (width * height * 4))(*([255] * (width * height * 4)))
        pixels[3] = 75  # Preserve the alpha of the original pinned PNG.
        before = bytes(pixels)
        handle = engine.pin_create(pixels, width, height, width * 4)
        self.assertTrue(handle)
        try:
            for value, box in [(b'Welcome to the garden.', Rect(10, 10, 200, 25)), (b'https://example.org', Rect(10, 65, 160, 20))]:
                self.assertEqual(1, engine.pin_add(handle, value, box, None, 0, 0, -1))
            self.assertEqual(1, engine.pin_prepare(handle))
            self.assertEqual(b'Welcome to the garden.', engine.pin_text(handle, 0))
            self.assertEqual(1, engine.pin_translated(handle, 0, '欢迎来到花园。'.encode()))
            out = type(pixels)()
            self.assertEqual(1, engine.pin_render(handle, b'zh-CN', out, width * 4))
            self.assertNotEqual(bytes(pixels), bytes(out))
            self.assertEqual(bytes(pixels)[60*width*4:], bytes(out)[60*width*4:])
            self.assertEqual(75, out[3])
            self.assertEqual(before, bytes(pixels))
        finally:
            engine.pin_destroy(handle)

if __name__ == '__main__':
    PinEngineTest.library_path = str(pathlib.Path(sys.argv.pop(1)).resolve())
    unittest.main()
