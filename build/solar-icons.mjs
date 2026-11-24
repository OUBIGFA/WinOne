// Shared access to the locked Solar Linear icons (MIT + CC BY 4.0).
//
// third_party/solar-icons.json records the pinned source and, per surface,
// which Solar icon each WinOne icon name uses. The SVGs are read from the
// prepared source cache (build/prepare-solar-icons.ps1); the generators write
// only the icons a surface uses, with currentColor instead of Solar's black.
import {readFileSync, readdirSync, existsSync} from 'node:fs';
import {join, resolve, dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';

export const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
export const catalog = JSON.parse(readFileSync(join(root, 'third_party/solar-icons.json'), 'utf8'));
export const sourceDirectory = resolve(root, process.env.WINONE_SOLAR_ICONS_DIRECTORY || catalog.source.cacheDirectory);

const sha = bytes => createHash('sha256').update(bytes).digest('hex');

let index;
function icons() {
  if (index) return index;
  const svgs = join(sourceDirectory, 'packages/core/svgs');
  if (!existsSync(svgs)) throw Error(`Solar icon source missing: ${svgs}\nRun build/prepare-solar-icons.ps1 -Archive <solar-icons-${catalog.source.commit}.tar.gz>`);
  for (const [file, digest] of Object.entries(catalog.source.licenseFiles)) {
    if (sha(readFileSync(join(sourceDirectory, file))) !== digest) throw Error('Solar icon license changed: ' + file);
  }
  index = new Map();
  for (const group of readdirSync(svgs)) {
    const linear = join(svgs, group, 'Linear');
    if (!existsSync(linear)) continue;
    for (const file of readdirSync(linear)) if (file.endsWith('.svg')) index.set(file.slice(0, -4), join(linear, file));
  }
  return index;
}

// The drawing inside an icon, colored by currentColor.
export function solarBody(name) {
  const path = icons().get(name);
  if (!path) throw Error('Not a Solar Linear icon: ' + name);
  const svg = readFileSync(path, 'utf8');
  const body = svg.replace(/^[\s\S]*?<svg[^>]*>/, '').replace(/<\/svg>\s*$/, '');
  if (/\bstroke="(?!black")[^"]*"|\bfill="(?!none"|white")[^"]*"/.test(body)) throw Error('Unexpected Solar colors: ' + name);
  return body.replace(/\bstroke="black"/g, 'stroke="currentColor"').replace(/\r?\n/g, '').replace(/>\s+</g, '><').trim();
}

// A whole icon. color sets currentColor for rasterizers; size the pixels.
export function solarSVG(name, {size = 24, color} = {}) {
  const colorAttribute = color ? ` color="${color}"` : '';
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none"${colorAttribute}>${solarBody(name)}</svg>`;
}

// Solar's MIT license followed by its CC BY 4.0 attribution, as shipped
// beside every generated set.
export function licenseText() {
  return Object.keys(catalog.source.licenseFiles).map(file => readFileSync(join(sourceDirectory, file), 'utf8').split(/\r?\n/).map(line => line.trimEnd()).join('\n').trimEnd()).join('\n\n----\n\n') + '\n';
}
