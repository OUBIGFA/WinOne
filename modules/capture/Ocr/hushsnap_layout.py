# SPDX-License-Identifier: GPL-3.0-only
# Adapted from tcita/HushSnap commit 2d43f0e1fd125484bd132ee6e9cad1db65fb7a8c.
# Pure reading-order and spacing code only; Qt and app lifecycle are excluded.
import re
import logging
from dataclasses import dataclass, field
logger = logging.getLogger(__name__)
_CENTER_RATIO = 0.4
_BOX_H_TO_FS_RATIO = 1.2


@dataclass
class OcrBox:
    x: float = 0.0
    y: float = 0.0
    width: float = 0.0
    height: float = 0.0

@dataclass
class OcrWord:
    text: str = ''
    bounding_box: OcrBox = field(default_factory=OcrBox)

@dataclass
class OcrLine:
    text: str = ''
    words: list[OcrWord] = field(default_factory=list)
    bounding_box: OcrBox = field(default_factory=OcrBox)
    indent_level: int = 0
    'Leading-indent level decided by the layout engine (0 = flush left).\n    Rendered as ``level * 4`` spaces; the decision is made on clean geometry\n    before any text mutation.'
    is_blank: bool = False
    'True when this line is a paragraph-separator blank line inserted by\n    the layout engine (gap >= 1.5x line height).  ``text`` is empty but the\n    line survives compose-time filtering to produce a blank line.'

def ppocr_box_to_bbox(box) -> tuple[float, float, float, float]:
    if not isinstance(box, list) or not box:
        return (0.0, 0.0, 0.0, 0.0)
    points = []
    for point in box:
        if isinstance(point, list | tuple) and len(point) >= 2:
            points.append((float(point[0]), float(point[1])))
    if not points:
        return (0.0, 0.0, 0.0, 0.0)
    xs = [point[0] for point in points]
    ys = [point[1] for point in points]
    return (min(xs), min(ys), max(xs), max(ys))

def bbox_to_ocr_box(left: float, top: float, right: float, bottom: float) -> OcrBox:
    return OcrBox(x=left, y=top, width=max(0.0, right - left), height=max(0.0, bottom - top))

def is_cjk_or_fullwidth(character: str) -> bool:
    if not character:
        return False
    codepoint = ord(character)
    return 12288 <= codepoint <= 12351 or 12352 <= codepoint <= 12543 or 19968 <= codepoint <= 40959 or (44032 <= codepoint <= 55215) or (65280 <= codepoint <= 65519)

def block_separator(left: str, right: str) -> str:
    if not left or not right:
        return ''
    if is_cjk_or_fullwidth(left[-1]) and is_cjk_or_fullwidth(right[0]):
        return ''
    if left[-1] in '-—–…」』':
        return ''
    if right[0] in ',.;:!?)]}，。、；：！？）】》〉〕｝…—–·"\'”’」』':
        return ''
    return ' '

def _normalize_blocks(blocks: list[dict]) -> list[dict]:
    """Convert raw PP-OCR detection blocks to internal representation; filter junk.

    Blocks without a valid bounding box are skipped — without real
    coordinates we cannot place them in reading order, and a fabricated
    box would distort line clustering.
    """
    empty_skipped = 0
    invalid_skipped = 0
    normalized: list[dict] = []
    for block in blocks or []:
        raw_text = str(block.get('text', '') or '')
        if not raw_text.strip():
            empty_skipped += 1
            continue
        raw_text = raw_text.strip()
        left, top, right, bottom = ppocr_box_to_bbox(block.get('box'))
        w = right - left
        h = bottom - top
        if w <= 0 or h <= 0:
            invalid_skipped += 1
            logger.debug('[DET] _normalize_blocks: skip invalid bbox (w=%.1f h=%.1f) %r', w, h, raw_text[:80])
            continue
        normalized.append({'text': raw_text, 'left': left, 'top': top, 'right': right, 'bottom': bottom, 'width': w, 'height': h, 'center_x': (left + right) / 2, 'center_y': (top + bottom) / 2})
    if empty_skipped or invalid_skipped:
        logger.debug('[DET] _normalize_blocks: %d blocks → %d valid (empty=%d invalid=%d)', len(blocks or []), len(normalized), empty_skipped, invalid_skipped)
    return normalized

def _greedy_line_cluster(blocks: list[dict]) -> list[list[dict]]:
    """Greedy line clustering for horizontal LTR text.

    1. Sort boxes by y_top (top → bottom)
    2. Greedy: if box centre is close to the line's median centre,
       add to current line.
    3. Within each line: sort by x_left (left → right)
    4. Between lines: sort by average y_center (top → bottom)

    Single condition::

        abs(box.center_y - median_center) < 0.4 × median_height

    This centre-distance check alone is functionally equivalent to the
    previous two-condition (overlap + centre) approach: at the 0.4
    threshold the overlap check is mathematically implied and therefore
    redundant.
    """
    if not blocks:
        return []
    sorted_blocks = sorted(blocks, key=lambda b: b['top'])
    lines: list[list[dict]] = []
    current: list[dict] = [sorted_blocks[0]]
    line_centers: list[float] = [sorted_blocks[0]['center_y']]
    line_heights: list[float] = [sorted_blocks[0]['height']]
    for box in sorted_blocks[1:]:
        sorted_centers = sorted(line_centers)
        n = len(sorted_centers)
        median_center = sorted_centers[n // 2]
        sorted_h = sorted(line_heights)
        median_h = sorted_h[n // 2]
        if abs(box['center_y'] - median_center) < _CENTER_RATIO * median_h / _BOX_H_TO_FS_RATIO:
            current.append(box)
            line_centers.append(box['center_y'])
            line_heights.append(box['height'])
        else:
            lines.append(current)
            current = [box]
            line_centers = [box['center_y']]
            line_heights = [box['height']]
    lines.append(current)
    for line in lines:
        line.sort(key=lambda b: b['left'])
    lines.sort(key=lambda ln: sum((b['center_y'] for b in ln)) / len(ln))
    logger.debug('[DET] _greedy_line_cluster: %d blocks → %d lines', len(blocks), len(lines))
    for i, ln in enumerate(lines):
        ys = sorted({b['top'] for b in ln})
        texts = [b['text'][:30] for b in ln]
        logger.debug('[DET]   line[%d]: %d boxes  y_range=%s  texts=%s', i, len(ln), ys[:6], texts)
    return lines

def _greedy_column_cluster(blocks: list[dict]) -> list[list[dict]]:
    """Greedy column clustering for vertical RTL text (CJK).

    Mirror of :func:`_greedy_line_cluster` with x/y roles swapped and
    sort directions reversed:

    1. Sort boxes by x_right descending (right → left)
    2. Greedy: if box centre is close to the column's median centre,
       add to current column.
    3. Within each column: sort by y_top (top → bottom)
    4. Between columns: sort by average x_center descending (right → left)

    Same single centre-distance condition as the line variant."""
    if not blocks:
        return []
    sorted_blocks = sorted(blocks, key=lambda b: -b['right'])
    columns: list[list[dict]] = []
    current = [sorted_blocks[0]]
    col_centers: list[float] = [sorted_blocks[0]['center_x']]
    col_widths: list[float] = [sorted_blocks[0]['width']]
    for box in sorted_blocks[1:]:
        sorted_centers = sorted(col_centers)
        n = len(sorted_centers)
        median_center = sorted_centers[n // 2]
        sorted_w = sorted(col_widths)
        median_w = sorted_w[n // 2]
        if abs(box['center_x'] - median_center) < _CENTER_RATIO * median_w / _BOX_H_TO_FS_RATIO:
            current.append(box)
            col_centers.append(box['center_x'])
            col_widths.append(box['width'])
        else:
            columns.append(current)
            current = [box]
            col_centers = [box['center_x']]
            col_widths = [box['width']]
    columns.append(current)
    for col in columns:
        col.sort(key=lambda b: b['top'])
    columns.sort(key=lambda col: -sum((b['center_x'] for b in col)) / len(col))
    logger.debug('[DET] _greedy_column_cluster: %d blocks → %d columns', len(blocks), len(columns))
    for i, col in enumerate(columns):
        xs = sorted({b['left'] for b in col})
        texts = [b['text'][:30] for b in col]
        logger.debug('[DET]   column[%d]: %d boxes  x_range=%s  texts=%s', i, len(col), xs[:6], texts)
    return columns

def _build_lines_from_clusters(clusters: list[list[dict]], is_vertical: bool=False) -> list[OcrLine]:
    """Convert clustered blocks into OcrLine objects.

    Each cluster (a line for horizontal text, or a column for vertical
    CJK) becomes one OcrLine.  Blocks within a cluster are already sorted
    in reading order (left→right for horizontal, top→bottom for vertical).

    Completes the full within-line text rendering in one pass: block
    boundaries (block_separator), inline-gap geometry, and intra-block
    CJK<->Latin spacing (_apply_cjk_spacing).  Downstream stages see line
    text already spacing-final at the within-line level.
    """
    result: list[OcrLine] = []
    for cluster in clusters:
        text_parts: list[str] = []
        words: list[OcrWord] = []
        prev_block = None
        min_l = min((b['left'] for b in cluster))
        min_t = min((b['top'] for b in cluster))
        max_r = max((b['right'] for b in cluster))
        max_b = max((b['bottom'] for b in cluster))
        for block in cluster:
            if prev_block:
                sep = block_separator(prev_block['text'], block['text'])
                if is_vertical:
                    gap = block['top'] - prev_block['bottom']
                    est = min(prev_block['width'], block['width']) / _BOX_H_TO_FS_RATIO
                else:
                    gap = block['left'] - prev_block['right']
                    est = min(prev_block['height'], block['height']) / _BOX_H_TO_FS_RATIO
                if est > 0:
                    gap_ratio = gap / est
                    if gap_ratio > 2.0:
                        n = max(1, round(gap_ratio))
                        sep = (sep or '') + ' ' * n
                if sep:
                    text_parts.append(sep)
            text_parts.append(block['text'])
            words.append(OcrWord(text=block['text'], bounding_box=bbox_to_ocr_box(block['left'], block['top'], block['right'], block['bottom'])))
            prev_block = block
        result.append(OcrLine(text=_apply_cjk_spacing(''.join(text_parts)), words=words, bounding_box=bbox_to_ocr_box(min_l, min_t, max_r, max_b)))
    return result

def _word_upper_median(line: OcrLine, *, axis: str) -> float:
    """High median (``sorted[n // 2]``) of the line's word-box geometry.

    Used wherever a line's *scale* (height / centre) is needed as a ruler
    for thresholds - the same drift-robust role the greedy clusterer's
    running median plays, but computed over the *complete* line (the
    clusterer's median is a partial sample missing the last box).

    The union bbox (``max(bottom) - min(top)``) is *not* the ruler here -
    but not for the reason of the absolute box-height inflation documented
    in comment §4 (every detection box runs larger than the font-size,
    ratio ~1.2-1.7× per scripts/measure_box_inflation.py; that inflation
    is shared by all boxes, so it alone favours neither the median nor
    the union).  The ruler-relevant failure is *relative*: when
    a line yields several boxes their heights diverge (mixed font sizes,
    descenders vs caps, CJK vs Latin on one baseline) and the union takes
    the extreme, amplifying that divergence.  On realistic mixed-content
    rendered lines (``scripts/measure_within_line_drift.py``) 65 % of
    multi-box lines show >=2 px of such union inflation (median +7.7 %,
    p95 +19 %); the high median anchors to the majority of boxes and is
    robust to it.

    Scope - do not over-read those figures: most desktop screenshots carry
    continuous text, so the detector emits few boxes per line (typically
    <=3).  With so few boxes the relative divergence has little room to
    act and the median's practical gain over the union is small.  The
    median is a correctness backstop for the multi-box edge case (drop-caps,
    inline smaller runs, mixed-script lines), not a high-frequency win.

    For *extent / position* queries (leftmost edge, the area a line
    occupies) use ``line.bounding_box`` directly - the union is correct
    there because taking the extreme is the point.

    axis:
        ``'h'``  -> upper-median word-box height
        ``'cy'`` -> upper-median word-box centre_y
        ``'cx'`` -> upper-median word-box centre_x (reserved, unused now)

    Falls back to the union-bbox value when the line has no usable words
    (e.g. tests construct ``OcrLine`` with only ``bounding_box`` set).
    """
    bbox = line.bounding_box
    if axis == 'h':
        vals = [w.bounding_box.height for w in line.words if w.bounding_box.height > 0]
        fallback = bbox.height
    elif axis == 'cy':
        vals = [w.bounding_box.y + w.bounding_box.height / 2 for w in line.words if w.bounding_box.height > 0]
        fallback = bbox.y + bbox.height / 2
    else:
        raise ValueError(f'unsupported axis {axis!r}')
    if not vals:
        return fallback
    s = sorted(vals)
    return s[len(s) // 2]

def _decide_paragraph_breaks(lines: list[OcrLine]) -> set[int]:
    """Return indices after which a blank separator line should be inserted.

    Deliberately conservative: this does NOT try to detect typographic
    paragraph spacing, which varies wildly (Markdown's blank line, Word's
    8pt space-after, a letter's signature gap, a poem's stanza break all
    differ, and a ci's upper/lower folio is often barely wider than a normal
    line).  It only separates lines that are obviously not one continuous
    block - the gap is large enough to fit a whole other line of text, so no
    reader would consider them adjacent.

    The decision is local rather than based on the page's average line height.
    Both the line height (threshold) and the line centre (gap) are taken as
    the high-median word-box value via :func:`_word_upper_median` (axis
    ``'h'`` and ``'cy'``).  The union bbox is *not* used as a ruler here:
    with several boxes on a line their heights diverge and
    ``max(bottom) - min(top)`` takes the extreme, amplifying that
    divergence.  The break rule's effective working region is the narrow
    band where the gap just fits one line - exactly where that union
    inflation would cause missed breaks.

    The remaining centre distance must exceed the taller adjacent line by
    half again.  In centre coordinates::

        next_center_y - current_center_y
            > current_h / 2 + next_h / 2 + 1.5 * max(current_h, next_h)

    i.e. the whitespace gap between the two boxes must exceed 1.5x the taller
    adjacent line's font size.  Symmetric for mixed font sizes and
    deliberately conservative: a smaller fraction (e.g. 0.6) would guess at
    paragraph semantics and fragment tight multi-line text; requiring room
    for one and a half taller lines only splits clearly-disconnected blocks.

    Returns a set of indices *i* such that a blank line is inserted after
    ``lines[i]`` during render.  Gap magnitude beyond the threshold does not
    produce additional blank lines - the separator is binary (one blank
    line, never several).  Horizontal-only; vertical is handled upstream.
    """
    if len(lines) <= 1:
        return set()
    medians = [_word_upper_median(line, axis='h') for line in lines]
    break_after: set[int] = set()
    for i in range(len(lines) - 1):
        line = lines[i]
        next_line = lines[i + 1]
        current_h = medians[i] / _BOX_H_TO_FS_RATIO
        next_h = medians[i + 1] / _BOX_H_TO_FS_RATIO
        if current_h <= 0 or next_h <= 0:
            continue
        current_center_y = _word_upper_median(line, axis='cy')
        next_center_y = _word_upper_median(next_line, axis='cy')
        threshold = current_h / 2 + next_h / 2 + 1.5 * max(current_h, next_h)
        if next_center_y - current_center_y > threshold:
            break_after.add(i)
    if break_after:
        logger.debug('[DET] _decide_paragraph_breaks: local centre-distance rule  %d blank lines (%d lines)', len(break_after), len(lines))
    return break_after
_CJK_RANGES = '\\u2E80-\\u2EFF\\u2F00-\\u2FDF\\u3040-\\u309F\\u30A0-\\u30FF\\u30FB-\\u30FF\\u3100-\\u312F\\u3200-\\u32FF\\u3400-\\u4DBF\\u4E00-\\u9FFF\\uF900-\\uFAFF'
_ANS_CLASS = 'A-Za-z\\u0370-\\u03FF0-9@$%^&*\\-+\\\\=\\|/\\u00A1-\\u00FF\\u2150-\\u218F\\u2700-\\u27BF'
_CJK_ANS_RE = re.compile(f'([{_CJK_RANGES}])([{_ANS_CLASS}])')
_ANS_CJK_RE = re.compile(f'([{_ANS_CLASS}~!;:,./?])([{_CJK_RANGES}])')

def _apply_cjk_spacing(text: str) -> str:
    """Ensure a single space between CJK and Latin characters.

    Idempotent: will not double-space text that already has correct spacing.

    URLs are exempted: the pangu-style CJK↔Latin spacers would otherwise
    insert a space at every CJK/Latin boundary *inside* a URL (e.g.
    ``kw=测试页面&fr=pb`` → ``kw= 测试页面 &fr=pb``), and the link highlighter —
    which stops at whitespace — would then only colour the fragment up to the
    first inserted space.  Spacing is applied only to the non-URL runs.
    """
    from hushsnap_text import apply_outside_urls
    if not text:
        return text

    def _space_runs(s: str) -> str:
        s = _CJK_ANS_RE.sub('\\1 \\2', s)
        s = _ANS_CJK_RE.sub('\\1 \\2', s)
        return s
    return apply_outside_urls(text, _space_runs)

def compose_ppocr_structures(blocks: list[dict], is_vertical: bool=False) -> list[OcrLine]:
    """Convert PP-OCR detection blocks into ordered OcrLines.

    Three-stage pipeline - geometry, decide, render - so that layout
    decisions (indent level, where blank lines go) are made on clean
    geometry *before* any text mutation, and text is rewritten once at
    the end:

      Stage 1 - geometry + text assembly (no decisions):
        1. Normalize raw blocks (filter empty / zero-size)
        2. Greedy overlap-based clustering -> reading order
        3. Build OcrLine objects from clusters (text = rec + block_separator
           + inline-gap spaces; CJK<->Latin spacing applied)
      Stage 2 - decide (compute, don't mutate text):
        4. Indentation: indent_level per line, baseline on clean lines
        5. Paragraph breaks: which line indices get a blank line after
      Stage 3 - render (mutate text once):
        6. Prepend indent spaces, insert is_blank separator lines

    Stages 4-6 are horizontal-only by design (see note below).  When
    *is_vertical* is True the image contains predominantly vertical CJK
    text (tall boxes, h > w * 1.3); column clustering detects vertical
    columns, reading order is right->left, top->bottom.

    Why decide-then-render: previously paragraph-break *sentinel* OcrLines
    (bounding_box=OcrBox() -> x=0) were inserted before indentation ran, so
    the indent baseline calculation had to exclude them by flag
    (``if not line.paragraph_break``).  Splitting decide from render lets
    the baseline see only real lines - no sentinel exclusion needed.
    """
    normalized = _normalize_blocks(blocks)
    if not normalized:
        return []
    if is_vertical:
        clusters = _greedy_column_cluster(normalized)
    else:
        clusters = _greedy_line_cluster(normalized)
    lines = _build_lines_from_clusters(clusters, is_vertical=is_vertical)
    if not lines:
        return []
    if not is_vertical:
        _decide_indentation(lines)
        break_after = _decide_paragraph_breaks(lines)
        lines = _render_layout(lines, break_after)
    return lines

def _decide_indentation(lines: list[OcrLine]) -> None:
    """Decide each line's ``indent_level`` from left-edge clustering.

    Mutates only ``line.indent_level`` - text is left untouched (rendered
    later by :func:`_render_layout`).  Runs on clean geometry: no blank-line
    separators are present yet, so the baseline needs no sentinel exclusion.

    Baseline is the leftmost box edge (a *position* query - union bbox is
    correct there).  Each line's height denominator is the high-median
    word-box height via :func:`_word_upper_median` (axis ``'h'``), matching
    the paragraph-break ruler: with several boxes on a line their heights
    diverge and the union takes the extreme, which would suppress the
    ratio and miss indents.

    ``indent_ratio = (x - baseline) / line_height``.  When the ratio
    exceeds 1.0 the line is considered intentionally indented; smaller
    offsets are treated as detection jitter.  1.0 deliberately gives up
    1-character indents (ratio ~0.79-1.10x font size): the calibrated
    divisor _BOX_H_TO_FS_RATIO leaves ``h`` ~9 % above real font size, so
    a 1-char offset lands at ~0.72-1.01 -> mostly below the threshold.
    Only multi-character indentation (code blocks, nested lists) is
    detected.  Per-line thresholds handle mixed-height text correctly - a
    tall title with a small absolute offset is not falsely flagged, and a
    short body line with the same offset is properly recognised.

    The indent *unit* is the smallest non-jitter offset from baseline.
    ``level = round(offset / unit)``; rendering applies ``level * 4``
    spaces.  Multi-level indentation (code, nested quotes, outlines) is
    handled without any per-language constants.
    """
    if len(lines) <= 1:
        return
    baseline = min((line.bounding_box.x for line in lines))

    def _is_indented(line: OcrLine) -> bool:
        """Offset exceeds jitter threshold relative to this line's own height."""
        h = _word_upper_median(line, axis='h') / _BOX_H_TO_FS_RATIO
        if h <= 0:
            return False
        indent_ratio = (line.bounding_box.x - baseline) / h
        return indent_ratio > 1.0
    offsets = sorted(set((round(line.bounding_box.x) - baseline for line in lines if _is_indented(line))))
    if not offsets:
        logger.debug('[DET] _decide_indentation: no offsets > per-line threshold -> no indent  (baseline=%.1f  n_lines=%d)', baseline, len(lines))
        return
    unit = offsets[0]
    logger.debug('[DET] _decide_indentation: baseline=%.1f  indent_unit=%d px  offsets=%s  n_lines=%d', baseline, unit, offsets, len(lines))
    indented = 0
    for line in lines:
        if not _is_indented(line):
            continue
        offset = round(line.bounding_box.x) - baseline
        line.indent_level = max(round(offset / unit), 1)
        indented += 1
        logger.debug('[DET]   indent L%d: offset=%d px -> level=%d  %r', indented, offset, line.indent_level, line.text[:60])
    if indented:
        logger.debug('[DET] _decide_indentation: %d/%d lines indented', indented, len(lines))

def _render_layout(lines: list[OcrLine], break_after: set[int]) -> list[OcrLine]:
    """Render layout decisions into text: prepend indent spaces and insert
    blank-line separators.  Single mutation pass - text was untouched during
    the decide stage.

    *break_after* holds indices (into the input *lines* list) after which a
    blank separator line should be inserted.
    """
    result: list[OcrLine] = []
    for i, line in enumerate(lines):
        if line.indent_level > 0:
            line.text = '    ' * line.indent_level + line.text
        result.append(line)
        if i in break_after:
            result.append(OcrLine(text='', bounding_box=OcrBox(), is_blank=True))
    return result

def compose_ppocr_text(blocks: list[dict]) -> str:
    """Compatibility wrapper that returns plain text string."""
    lines = compose_ppocr_structures(blocks)
    return '\n'.join((line.text for line in lines)).rstrip()
_VERTICAL_BOX_RATIO = 1.5
_VERTICAL_WEIGHTED_THRESHOLD = 0.5

def _is_vertical_json(json_data: list[dict]) -> bool:
    """Return True if text is predominantly vertical (tall boxes by area).

    Uses area-weighted voting: each box votes for "vertical" or "horizontal"
    in proportion to its pixel area.  Small fragments (stray chars, labels)
    contribute negligible weight (< 1 % of a real text column), so no
    explicit noise-filter threshold is needed — the weighting is inherently
    robust to outliers.

    This replaces the earlier ad-hoc approach of filtering by a percentage
    of the largest box's area (5 %) with an absolute floor (500 px²), which
    required tuning per image resolution.
    """
    if not json_data:
        return False
    total_area = 0.0
    tall_area = 0.0
    for item in json_data:
        box = item.get('box', [])
        if not box:
            continue
        xs = [p[0] for p in box]
        ys = [p[1] for p in box]
        w = max(xs) - min(xs)
        h = max(ys) - min(ys)
        if w <= 0:
            continue
        area = w * h
        total_area += area
        if h > w * _VERTICAL_BOX_RATIO:
            tall_area += area
    if total_area == 0.0:
        return False
    ratio = tall_area / total_area
    is_vert = ratio >= _VERTICAL_WEIGHTED_THRESHOLD
    logger.debug('[DET] _is_vertical_json: tall_area_ratio=%.2f (threshold=%.2f) total_area=%d → vertical=%s', ratio, _VERTICAL_WEIGHTED_THRESHOLD, int(total_area), is_vert)
    return is_vert
