import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {renderNotices} from '../../build/bundle-notices.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const catalog = JSON.parse(fs.readFileSync(path.join(root, 'third_party/import-notices.json'), 'utf8'));
const record = catalog.imports.find(item => item.name === 'umi-ocr-paragraph-layout');
assert.ok(record);
const result = renderNotices(root, [record, {...record, name: 'Second attribution'}]);
assert.equal(result.uniqueTexts, 1);
assert.equal(result.components, 2);
const original = fs.readFileSync(path.join(root, 'third_party', record.notices[0].file));
assert.ok(result.licenses.includes(original), 'original notice bytes were changed');
assert.ok(result.index.toString().includes('Second attribution'));
assert.throws(() => renderNotices(root, [{...record, notices: [{...record.notices[0], sha256: '0'.repeat(64)}]}]), /checksum mismatch/);
assert.throws(() => renderNotices(root, [{...record, notices: []}]), /Missing component notices/);
console.log('Bundled license preservation, deduplication, attribution and corruption checks passed');
