// Upstream tracking for Cindy (Apache-2.0) code that WinOne ports by hand.
//
// The MyGo desktop rewrites Cindy's React Chat, automations and OneBot in Go,
// so `sync-cindy.mjs` cannot reproduce it. third_party/cindy/native-ports.json
// records, for every native module, the Cindy files it follows and, per file,
// the Cindy revision the Go code is aligned with and the file's SHA-256 there.
// Files come from React copies pinned at different times, so each keeps its own
// revision; nothing between that revision and --ref goes unreported.
//
// This script:
//   * verifies the manifest: native files exist, every upstream file exists at
//     its recorded revision with the recorded SHA-256;
//   * reports upstream changes up to --ref (default origin/main), per native
//     module, plus files added under its watched directories and the groups of
//     every *.sync.json spec that changed;
//   * --accept <id|all> moves a port to --ref after its changes were ported,
//     refreshing the recorded SHA-256 values. Accepting states that the Go code
//     now matches; review the report first.
//   * --resolve <id> lists Cindy files reachable from the port's `mirrors`
//     (WinOne React files) through the source manifests; with --write it adds
//     missing ones to `upstream` at the revision their React copy was pinned.
//     A port's `ignore` globs drop upstream files it deliberately does not follow.
//
// Usage:
//   node build/check-cindy-ports.mjs --cindy <repo> [--ref origin/main] [--strict]
//   node build/check-cindy-ports.mjs --cindy <repo> --accept <id> [--ref <commit>]
//   node build/check-cindy-ports.mjs --cindy <repo> --resolve <id> [--depth 1] [--write]
// --strict exits 1 when any port or sync spec is behind --ref.
import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const cindyDir = path.join(root, 'third_party/cindy');
// --manifest replaces the tracked manifest (tests use a fixture).
const manifestPath = (() => { const i = process.argv.indexOf('--manifest'); return i > 0 ? path.resolve(process.argv[i + 1]) : path.join(cindyDir, 'native-ports.json'); })();
const sha = (bytes) => createHash('sha256').update(bytes).digest('hex');
const short = (revision) => revision.slice(0, 10);

function argument(name, fallback) {
  const index = process.argv.indexOf(name);
  return index > 0 && index + 1 < process.argv.length ? process.argv[index + 1] : fallback;
}

const cindy = argument('--cindy');
if (!cindy) throw Error('Pass --cindy <path to a makecindy/cindy clone>');
const git = (args, encoding = 'utf8') => execFileSync('git', ['-C', cindy, ...args], {encoding, maxBuffer: 256 * 1024 * 1024});
const commitOf = (ref) => git(['rev-parse', '--verify', ref + '^{commit}']).trim();
function show(revision, file) {
  try { return git(['show', `${revision}:${file}`], 'buffer'); } catch { return null; }
}
function changed(from, to, paths) {
  if (from === to || paths.length === 0) return [];
  const out = git(['diff', '--numstat', '--no-renames', from, to, '--', ...paths]).trim();
  return out ? out.split('\n').map((line) => { const [add, del, file] = line.split('\t'); return {file, add, del}; }) : [];
}
function isAncestor(older, newer) {
  try { git(['merge-base', '--is-ancestor', older, newer]); return true; } catch { return false; }
}
const tree = (revision, dir) => git(['ls-tree', '-r', '--name-only', revision, '--', dir]).split('\n').filter(Boolean);

function globToRegExp(glob) {
  let pattern = '';
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === '*' && glob[i + 1] === '*') { pattern += '.*'; i++; if (glob[i + 1] === '/') i++; }
    else if (c === '*') pattern += '[^/]*';
    else if (c === '?') pattern += '[^/]';
    else pattern += c.replace(/[.+^${}()|[\]\\]/g, '\\$&');
  }
  return new RegExp('^' + pattern + '$');
}

const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
const ref = argument('--ref', 'origin/main');
const target = commitOf(ref);
const save = () => fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2) + '\n');
const findPort = (id) => manifest.ports.find((p) => p.id === id) ?? (() => { throw Error('Unknown port: ' + id); })();

// The hand-written source manifests map a vendored WinOne target to its Cindy
// source. Their formats differ; every {source, target} pair is collected. The
// *.sync.json specs map whole directories (groups).
function sourceMap() {
  const files = new Map();
  const groups = [];
  for (const name of fs.readdirSync(cindyDir).filter((f) => f.endsWith('-source.json') || f === 'source.json')) {
    const spec = JSON.parse(fs.readFileSync(path.join(cindyDir, name), 'utf8'));
    const revision = spec.revision ?? spec.commit;
    const entries = [...(spec.files ?? []), ...(spec.sources ?? [])].filter((e) => typeof e === 'object');
    if (spec.source && spec.target) entries.push({source: spec.source, target: spec.target});
    for (const entry of entries) {
      // Keys vary: target or file; source.json gives sourceDirectory + file;
      // title-source.json gives one top-level target fed by sources[].path.
      let source = entry.source;
      let target = entry.target ?? entry.file;
      if (!source && spec.sourceDirectory && entry.file) { source = spec.sourceDirectory + '/' + entry.file; target = 'third_party/cindy/' + entry.file; }
      if (!source && entry.path && typeof spec.target === 'string') { source = entry.path; target = spec.target; }
      if (!source || !target) continue;
      for (const t of [target].flat()) {
        if (!files.has(t)) files.set(t, []);
        files.get(t).push({source, revision, manifest: name});
      }
    }
  }
  for (const name of fs.readdirSync(cindyDir).filter((f) => f.endsWith('.sync.json'))) {
    const spec = JSON.parse(fs.readFileSync(path.join(cindyDir, name), 'utf8'));
    for (const group of spec.groups ?? []) groups.push({group, revision: spec.revision, manifest: name});
  }
  return {files, groups};
}
// Returns every Cindy source a WinOne file was copied from (usually one).
function lookup(map, file) {
  if (map.files.has(file)) return map.files.get(file);
  // The longest matching group wins; its include/exclude globs must admit the file.
  let best = null;
  for (const {group, revision, manifest: name} of map.groups) {
    if (!file.startsWith(group.target + '/') || (best && best.length >= group.target.length)) continue;
    const rel = file.slice(group.target.length + 1);
    if (!(group.include ?? ['**']).some((g) => globToRegExp(g).test(rel)) || (group.exclude ?? []).some((g) => globToRegExp(g).test(rel))) continue;
    best = {length: group.target.length, entry: {source: group.source + '/' + rel, revision, manifest: name}};
  }
  return best ? [best.entry] : [];
}

if (process.argv.includes('--resolve')) {
  const port = findPort(argument('--resolve'));
  const map = sourceMap();
  const found = new Map();
  const seen = new Set();
  // Mirrors are WinOne files the Go code reproduces. Their relative imports
  // that are vendored Cindy copies are recorded, followed up to --depth
  // levels (default 1: only what the mirrors import directly).
  const depth = Number(argument('--depth', '1'));
  const ignored = (port.ignore ?? []).map(globToRegExp);
  const queue = (port.mirrors ?? []).map((file) => [file, 0]);
  while (queue.length) {
    const [file, level] = queue.shift();
    if (seen.has(file)) continue;
    seen.add(file);
    // A copy whose source is absent upstream is WinOne's own file in a vendored
    // directory; it is not recorded.
    for (const vendored of lookup(map, file)) {
      if (!show(vendored.revision, vendored.source) || ignored.some((r) => r.test(vendored.source))) continue;
      // A file copied twice keeps the newer revision: changes up to it are in WinOne already.
      const known = found.get(vendored.source);
      if (!known || isAncestor(known.revision, vendored.revision)) found.set(vendored.source, vendored);
    }
    const full = path.join(root, file);
    if (!fs.existsSync(full)) { console.log('missing mirror ' + file); continue; }
    if (level >= depth) continue;
    const text = fs.readFileSync(full, 'utf8');
    for (const match of text.matchAll(/(?:from|import)\s*\(?\s*['"](\.{1,2}\/[^'"]+)['"]/g)) {
      const base = path.posix.normalize(path.posix.join(path.posix.dirname(file), match[1]));
      for (const candidate of [base, base + '.ts', base + '.tsx', base + '/index.ts', base + '/index.tsx']) {
        const abs = path.join(root, candidate);
        if (!fs.existsSync(abs) || !fs.statSync(abs).isFile()) continue;
        if (lookup(map, candidate).length) queue.push([candidate, level + 1]);
        break;
      }
    }
  }
  const have = new Set(port.upstream.map((e) => e.path));
  for (const [source, info] of [...found].sort()) {
    console.log(`${have.has(source) ? '  ' : '+ '}${source}\t(${info.manifest} @ ${short(info.revision)})`);
    if (!process.argv.includes('--write') || have.has(source)) continue;
    const bytes = show(info.revision, source);
    if (!bytes) throw Error(`${source} not found at ${info.revision}`);
    port.upstream.push({path: source, revision: info.revision, sha256: sha(bytes)});
  }
  if (process.argv.includes('--write')) { port.upstream.sort((a, b) => a.path.localeCompare(b.path)); save(); }
  process.exit(0);
}

// 1. Verify the manifest.
const problems = [];
const ids = new Set();
for (const port of manifest.ports) {
  if (ids.has(port.id)) problems.push(`duplicate port id ${port.id}`);
  ids.add(port.id);
  for (const file of port.native) if (!fs.existsSync(path.join(root, file))) problems.push(`${port.id}: native file missing: ${file}`);
  for (const entry of port.upstream) {
    const bytes = show(entry.revision, entry.path);
    if (!bytes) problems.push(`${port.id}: ${entry.path} not found at ${short(entry.revision)}`);
    else if (sha(bytes) !== entry.sha256) problems.push(`${port.id}: ${entry.path} SHA-256 differs from the record at ${short(entry.revision)}`);
  }
}

// 2. Accept: move ports to the target revision after porting.
// A copied baseline is not parity evidence. Pending migration/verification
// items stay explicit until their implementation and checks are completed.
const accept = argument('--accept');
if (accept) {
  for(const port of manifest.ports.filter(p=>accept==='all'||p.id===accept))if(port.pending?.length)problems.push(`${port.id}: finish pending work before accepting: ${port.pending.join('; ')}`);
  if (problems.length) { console.error(problems.join('\n')); process.exit(1); }
  for (const port of manifest.ports.filter((p) => accept === 'all' || p.id === accept)) {
    for (const entry of port.upstream) {
      const bytes = show(target, entry.path);
      if (!bytes) throw Error(`${port.id}: ${entry.path} was removed upstream at ${short(target)}; update the manifest by hand`);
      entry.sha256 = sha(bytes);
      entry.revision = target;
    }
    if (port.watch) port.watchRevision = target;
    console.log(`accepted ${port.id} at ${short(target)}`);
  }
  save();
  process.exit(0);
}

// 3. Report upstream changes.
let behind = 0;
console.log(`Cindy ${ref} = ${short(target)}\n`);
for (const port of manifest.ports) {
  if(port.pending?.length){behind++;console.log(`pending ${port.id}: ${port.pending.join('; ')}`);}
  const files = port.upstream.flatMap((e) => changed(e.revision, target, [e.path]));
  const added = [];
  const since = port.watchRevision ?? port.upstream[0]?.revision ?? target;
  for (const dir of port.watch ?? []) {
    const before = new Set(tree(since, dir));
    for (const file of tree(target, dir)) if (!before.has(file) && !/\.test\.|__tests__/.test(file)) added.push(file);
  }
  if (!files.length && !added.length) { if(!port.pending?.length)console.log(`ok      ${port.id}`); continue; }
  behind++;
  console.log(`behind  ${port.id} → ${short(target)}; port into ${port.native.join(', ')}`);
  for (const f of files) console.log(`          ${f.file}  +${f.add} -${f.del}`);
  for (const f of added) console.log(`          new file in watched directory: ${f}`);
}

// The sync specs belong to the tracked tree; a fixture manifest skips them.
const syncSpecs = process.argv.includes('--manifest') ? [] : fs.readdirSync(cindyDir).filter((f) => f.endsWith('.sync.json')).sort();
if (syncSpecs.length) console.log('');
for (const name of syncSpecs) {
  const spec = JSON.parse(fs.readFileSync(path.join(cindyDir, name), 'utf8'));
  let count = 0;
  for (const group of spec.groups ?? []) {
    const include = (group.include ?? ['**']).map(globToRegExp);
    const exclude = (group.exclude ?? []).map(globToRegExp);
    for (const f of changed(spec.revision, target, [group.source])) {
      const rel = f.file.slice(group.source.length + 1);
      if (include.some((r) => r.test(rel)) && !exclude.some((r) => r.test(rel))) count++;
    }
  }
  if (count) { behind++; console.log(`behind  ${name}: ${count} file(s) changed; move its revision and run sync-cindy.mjs --write`); }
  else console.log(`ok      ${name} (${short(spec.revision)})`);
}

if (problems.length) { console.error('\nManifest problems:\n' + problems.join('\n')); process.exit(1); }
if (behind && process.argv.includes('--strict')) process.exit(1);
