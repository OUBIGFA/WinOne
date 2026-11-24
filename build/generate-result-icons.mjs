// Writes the shared result windows' icons (apps/result-ui/assets) and the
// shared model menu's (modules/nativeui/modelmenu/icons) from the locked
// Solar Linear set, as mapped in third_party/solar-icons.json.
//
// The floating selection toolbar keeps its own toolbar-<id>.svg files for the
// actions a user can place on it; they draw the same artwork, except where the
// mapping's toolbar table names another icon.
//
//   node build/generate-result-icons.mjs          # write the files
//   node build/generate-result-icons.mjs --check  # fail when they are stale
import {readFileSync, writeFileSync, readdirSync, mkdirSync, rmSync, existsSync} from 'node:fs';
import {join, relative} from 'node:path';
import {root, catalog, solarSVG, licenseText} from './solar-icons.mjs';

const check = process.argv.includes('--check');
const results = catalog.surfaces.results;
const toolbarIcons = ['translate', 'explain', 'summarize', 'polish', 'code', 'conversation', 'service', 'copy', 'book', 'idea', 'search', 'document', 'check', 'write', 'mail', 'globe', 'learn', 'calculate', 'terminal', 'braces', 'tasks', 'text', 'case', 'wand', 'astroid', 'quick-assistant'];

const outputs = new Map();
const resultDir = join(root, 'apps/result-ui/assets');
for (const [id, name] of Object.entries(results.icons)) {
  outputs.set(join(resultDir, id + '.svg'), solarSVG(name) + '\n');
}
for (const id of toolbarIcons) {
  const name = results.toolbar?.[id] ?? results.icons[id];
  if (!name) throw Error('Toolbar icon has no mapping: ' + id);
  outputs.set(join(resultDir, 'toolbar-' + id + '.svg'), solarSVG(name) + '\n');
}
outputs.set(join(resultDir, 'LICENSE.txt'), licenseText());
outputs.set(join(resultDir, 'logo.png'), readFileSync(join(root, 'internal/branding/logo.png')));

const menuDir = join(root, 'modules/nativeui/modelmenu/icons');
for (const [id, name] of Object.entries(catalog.surfaces.modelMenu.icons)) {
  outputs.set(join(menuDir, id + '.svg'), solarSVG(name) + '\n');
}
outputs.set(join(menuDir, 'LICENSE.txt'), licenseText());

// Only this generator's files: top-level SVGs, never providers/ or bitmaps.
const owned = dir => existsSync(dir) ? readdirSync(dir, {withFileTypes: true}).filter(e => e.isFile() && (e.name.endsWith('.svg') || e.name === 'LICENSE.txt')).map(e => join(dir, e.name)) : [];
const present = [...owned(resultDir), ...owned(menuDir)];
const same = (path, data) => existsSync(path) && Buffer.compare(readFileSync(path), Buffer.from(data)) === 0;

if (check) {
  const stale = [];
  for (const [path, data] of outputs) if (!same(path, data)) stale.push(relative(root, path));
  for (const path of present) if (!outputs.has(path)) stale.push(relative(root, path) + ' (unused)');
  if (stale.length) {
    console.error('Result icons are stale; run node build/generate-result-icons.mjs:\n' + stale.join('\n'));
    process.exit(1);
  }
  console.log(`result icons current: ${outputs.size} files`);
} else {
  mkdirSync(resultDir, {recursive: true});
  mkdirSync(menuDir, {recursive: true});
  for (const path of present) if (!outputs.has(path)) rmSync(path);
  for (const [path, data] of outputs) if (!same(path, data)) writeFileSync(path, data);
  console.log(`result icons written: ${outputs.size} files`);
}
