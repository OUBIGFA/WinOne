# SPDX-License-Identifier: GPL-3.0-only
# Adapted from HushSnap hushsnap/ocr/text.py at 2d43f0e1fd125484bd132ee6e9cad1db65fb7a8c.
import re
from typing import Callable

URL_REGEX = re.compile(r"https?://?[^\s<>\]\)\}]+", re.IGNORECASE)


def apply_outside_urls(text: str, transform: Callable[[str], str]) -> str:
    """Keep URL content intact when inserting spaces around CJK text."""
    if not text:
        return text
    parts = []
    last_end = 0
    for match in URL_REGEX.finditer(text):
        if match.start() > last_end:
            parts.append(transform(text[last_end:match.start()]))
        parts.append(match.group(0))
        last_end = match.end()
    parts.append(transform(text[last_end:]))
    return ''.join(parts)
