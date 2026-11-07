"""Reproduce the NewCM copyright evidence from the locked npm font files.

Run deliberately after reviewing an upgrade: python build/record-math-font-notices.py
Requires fontTools[woff] (intake used 4.63.0), not needed by normal/offline builds.
The original name records, not npm's package-level license label, identify the
underlying font's GUST license. No font or package files are modified.
"""
from collections import defaultdict
from hashlib import sha256
import json
from pathlib import Path

from fontTools.ttLib import TTFont

root = Path(__file__).resolve().parent.parent
package_dir = root / 'modules/chat-pi/node_modules/@mathjax/mathjax-newcm-font'
package = json.loads((package_dir / 'package.json').read_text(encoding='utf-8'))
groups = defaultdict(list)
evidence = []
for font_file in sorted((package_dir / 'chtml/woff2').glob('*.woff2')):
    with TTFont(font_file) as font:
        records = sorted({record.toUnicode().strip() for record in font['name'].names
                          if record.nameID in (0, 13, 14) and record.toUnicode().strip()})
    if not records:
        raise ValueError(f'Missing copyright: {font_file.name}')
    groups[tuple(records)].append(font_file.name)
    evidence.append({'sourceFile': font_file.relative_to(package_dir).as_posix(),
                     'sha256': sha256(font_file.read_bytes()).hexdigest(), 'notices': records})
if not evidence:
    raise ValueError('No font evidence found')
for name in ('package.json', 'def/mathjax-newcm.ts'):
    evidence.append({'sourceFile': name, 'sha256': sha256((package_dir / name).read_bytes()).hexdigest()})
record = {'package': package['name'], 'version': package['version'],
          'generator': 'build/record-math-font-notices.py', 'files': evidence}
(root / 'third_party/chat-math-font-evidence.json').write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8', newline='\n')
text = [f"{package['name']} {package['version']} — original embedded font notices",
        'Extracted verbatim from nonempty OpenType name IDs 0, 13 and 14.',
        'File hashes and original records: chat-math-font-evidence.json.',
        'Generator: build/record-math-font-notices.py (fontTools[woff]).', '']
for records, files in groups.items():
    text.extend(['Fonts: ' + ', '.join(files), *records, ''])
(root / 'third_party/licenses/npm-chat-mathjax-newcm-4.1.3-COPYRIGHT.txt').write_text('\n'.join(text), encoding='utf-8', newline='\n')
print(f'Recorded original notices from {len(evidence) - 2} fonts')
