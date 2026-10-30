// Reproducible vendoring of Cindy (Apache-2.0) source into WinOne.
//
// A spec under third_party/cindy/*.sync.json lists upstream groups and the
// WinOne seams that replace Cindy host modules. Files are read from the pinned
// revision with `git show`, so working-tree line endings never leak in. Only
// mechanical edits are applied:
//   1. import specifiers are re-pointed to the vendored copy or to a seam;
//   2. configured literal replacements (e.g. window.electronAPI -> electronAPI)
//      add the matching seam import;
//   3. reviewed literal patches (spec.patches) for host facts such as WinOne's
//      single Pi harness. Each patch states its file, exact text and the
//      number of matches, so an upstream change fails the sync loudly.
// Any other change must live in a seam module. `--check` fails when vendored
// files differ from what the spec reproduces, so hand edits cannot drift.
//
// Usage:
//   node build/sync-cindy.mjs --spec third_party/cindy/scheduler.sync.json --cindy <repo> [--write|--check]
import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';

const sha = (bytes) => createHash('sha256').update(bytes).digest('hex');
const posix = (value) => value.split(path.sep).join('/');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function argument(name, fallback) {
  const index = process.argv.indexOf(name);
  return index > 0 && index + 1 < process.argv.length ? process.argv[index + 1] : fallback;
}

function git(repo, args, encoding = 'utf8') {
  return execFileSync('git', ['-C', repo, ...args], {encoding, maxBuffer: 256 * 1024 * 1024});
}

function safeTarget(relative) {
  if (typeof relative !== 'string' || !relative || relative.includes('\\') || path.posix.isAbsolute(relative) ||
      path.posix.normalize(relative) !== relative || relative.split('/').includes('..')) {
    throw Error('Invalid repository path in sync spec: ' + relative);
  }
  return path.join(root, relative);
}

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

// Locale subset: every key the vendored code can look up, taken from the
// upstream locale JSON at the pinned revision. Keys are found as dotted string
// literals that exist upstream, as static prefixes of template/concatenated
// keys (the whole subtree), plural siblings and `$t(...)` references.
function extractLocales(config, outputs, read) {
  const bundles = Object.fromEntries(Object.entries(config.sources).map(([lang, file]) => [lang, {file, bytes: read(file)}]));
  for (const bundle of Object.values(bundles)) bundle.tree = JSON.parse(bundle.bytes.toString('utf8').replace(/^\uFEFF/, ''));
  const trees = Object.values(bundles).map((bundle) => bundle.tree);
  const lookup = (tree, key) => key.split('.').reduce((node, part) => (node && typeof node === 'object' ? node[part] : undefined), tree);
  const plural = /_(zero|one|two|few|many|other)$/;
  const exists = (key) => trees.some((tree) => lookup(tree, key) !== undefined || ['_one', '_other', '_zero'].some((suffix) => lookup(tree, key + suffix) !== undefined));
  const keys = new Set(config.extraKeys ?? []);
  const prefixes = new Set(config.extraPrefixes ?? []);
  const literal = /(['"`])([A-Za-z][\w-]*(?:\.[\w-]+)+)\1/g;
  const dynamic = [/`([A-Za-z][\w-]*(?:\.[\w-]+)*)\.\$\{/g, /(['"])([A-Za-z][\w-]*(?:\.[\w-]+)*)\.\1\s*\+/g];
  for (const output of outputs) {
    if (!/\.(ts|tsx)$/.test(output.target)) continue;
    const text = output.bytes.toString('utf8');
    for (const match of text.matchAll(literal)) if (exists(match[2])) keys.add(match[2]);
    for (const pattern of dynamic) for (const match of text.matchAll(pattern)) {
      const prefix = match[match.length - 1];
      if (trees.some((tree) => { const node = lookup(tree, prefix); return node && typeof node === 'object'; })) prefixes.add(prefix);
    }
  }
  const excluded = (leaf) => (config.exclude ?? []).some((prefix) => leaf === prefix || leaf.startsWith(prefix + '.'));
  const included = (leaf) => !excluded(leaf) && (keys.has(leaf) || keys.has(leaf.replace(plural, '')) || [...prefixes].some((prefix) => leaf === prefix || leaf.startsWith(prefix + '.')));
  // `$t(other.key)` nesting pulls in the referenced keys too.
  for (let changed = true; changed;) {
    changed = false;
    const visit = (node, at) => {
      if (typeof node === 'string') {
        if (!included(at)) return;
        for (const match of node.matchAll(/\$t\(([^,)]+)/g)) if (!keys.has(match[1].trim())) { keys.add(match[1].trim()); changed = true; }
      } else if (node && typeof node === 'object') for (const [name, child] of Object.entries(node)) visit(child, at ? at + '.' + name : name);
    };
    for (const tree of trees) visit(tree, '');
  }
  const variables = config.variables ?? {};
  const problems = [];
  const results = [];
  for (const [lang, bundle] of Object.entries(bundles)) {
    const subset = (node, at) => {
      if (typeof node === 'string') {
        if (!included(at)) return undefined;
        let value = node;
        for (const [name, replacement] of Object.entries(variables)) value = value.split('{{' + name + '}}').join(replacement);
        const override = config.overrides?.[lang]?.[at];
        if (override !== undefined) value = override;
        if ((config.forbid ?? []).some((pattern) => new RegExp(pattern).test(value))) problems.push(`${lang} ${at}: ${value.slice(0, 80)}`);
        return value;
      }
      if (!node || typeof node !== 'object' || Array.isArray(node)) return included(at) ? node : undefined;
      const out = {};
      for (const [name, child] of Object.entries(node)) {
        const value = subset(child, at ? at + '.' + name : name);
        if (value !== undefined) out[name] = value;
      }
      return Object.keys(out).length ? out : undefined;
    };
    const bytes = Buffer.from(JSON.stringify(subset(bundle.tree, '') ?? {}, null, 1) + '\n', 'utf8');
    const target = config.targets[lang];
    results.push({source: bundle.file, sourceSHA256: sha(bundle.bytes), target, sha256: sha(bytes), rewritten: true, patched: [], adaptation: 'locale subset' + (config.exclude?.length ? '; excluded ' + config.exclude.join(', ') : '') + (config.overrides?.[lang] ? '; WinOne wording for ' + Object.keys(config.overrides[lang]).join(', ') : '') + (Object.keys(variables).length ? '; ' + Object.keys(variables).map((name) => '{{' + name + '}}').join(', ') + ' substituted' : ''), bytes});
  }
  if (problems.length) throw Error('Cindy locale subset contains forbidden text:\n  ' + problems.join('\n  '));
  return results;
}

export function syncCindy({spec, cindy, mode}) {
  if (spec.schemaVersion !== 1 || !/^[a-f0-9]{40}$/.test(spec.revision)) throw Error('Invalid sync spec');
  const revision = spec.revision;
  const resolved = git(cindy, ['rev-parse', revision + '^{commit}']).trim();
  if (resolved !== revision) throw Error('Cindy revision is not available: ' + revision);
  const tree = new Set(git(cindy, ['ls-tree', '-r', '--name-only', revision]).split('\n').filter(Boolean));
  const read = (file) => execFileSync('git', ['-C', cindy, 'show', revision + ':' + file], {maxBuffer: 256 * 1024 * 1024});

  // Package export maps for @cindy/* bare specifiers.
  const exportsCache = new Map();
  function packageExports(name) {
    if (!exportsCache.has(name)) {
      const dir = 'packages/' + name.replace(/^@cindy\//, '');
      const manifest = dir + '/package.json';
      exportsCache.set(name, tree.has(manifest) ? {dir, exports: JSON.parse(read(manifest).toString('utf8')).exports ?? {}} : null);
    }
    return exportsCache.get(name);
  }

  function existing(base) {
    const stripped = base.replace(/\.(js|mjs|jsx)$/, '');
    for (const candidate of [base, stripped + '.ts', stripped + '.tsx', base + '.ts', base + '.tsx', base + '/index.ts', base + '/index.tsx', stripped + '.mjs', stripped + '.d.mts']) {
      if (tree.has(candidate)) return candidate;
    }
    return null;
  }

  // Bare module specifiers that WinOne replaces with a seam (e.g. react-router-dom).
  const modules = spec.modules ?? {};
  for (const target of Object.values(modules)) safeTarget(target);

  // Resolve an import specifier inside an upstream file to an upstream file path.
  // spec.redirects re-point one upstream module for importers under a prefix
  // (e.g. renderer type-only imports of a package index -> its types module).
  function resolveUpstream(fromFile, specifier) {
    const resolved = resolveUpstreamModule(fromFile, specifier);
    if (typeof resolved !== 'string') return resolved;
    const redirect = (spec.redirects ?? []).find((entry) => entry.from === resolved && fromFile.startsWith(entry.importers));
    return redirect ? redirect.to : resolved;
  }
  function resolveUpstreamModule(fromFile, specifier) {
    if (Object.hasOwn(modules, specifier)) return {module: specifier};
    // Upstream also writes `@/../shared/x`: normalize like a relative import.
    if (specifier.startsWith('@/')) return existing(path.posix.normalize('apps/desktop/src/renderer/' + specifier.slice(2)));
    if (specifier.startsWith('.')) return existing(path.posix.normalize(path.posix.join(path.posix.dirname(fromFile), specifier)));
    const match = /^(@cindy\/[^/]+)(?:\/(.+))?$/.exec(specifier);
    if (!match) return undefined; // external npm module
    const info = packageExports(match[1]);
    if (!info) return null;
    const key = match[2] ? './' + match[2] : '.';
    let target = info.exports[key];
    if (target && typeof target === 'object') target = target.import ?? target.default ?? target.types;
    if (typeof target !== 'string') return existing(info.dir + '/src/' + (match[2] ?? 'index'));
    return existing(path.posix.normalize(path.posix.join(info.dir, target)));
  }

  // Collect vendored files and their targets.
  const vendored = new Map(); // upstream file -> target repo path
  for (const group of spec.groups) {
    const include = (group.include ?? ['**/*']).map(globToRegExp);
    const exclude = (group.exclude ?? []).map(globToRegExp);
    const prefix = group.source.replace(/\/$/, '') + '/';
    let matched = 0;
    for (const file of tree) {
      if (!file.startsWith(prefix) && file !== group.source) continue;
      const relative = file === group.source ? path.posix.basename(file) : file.slice(prefix.length);
      if (!include.some((re) => re.test(relative)) || exclude.some((re) => re.test(relative))) continue;
      const target = group.file ? group.target : group.target.replace(/\/$/, '') + '/' + relative;
      safeTarget(target);
      if (vendored.has(file)) throw Error('Upstream file vendored twice: ' + file);
      vendored.set(file, target);
      matched++;
    }
    if (!matched) throw Error('Sync group matched no upstream files: ' + group.source);
  }
  const seams = spec.seams ?? {};
  for (const target of Object.values(seams)) safeTarget(target);

  const specifierPatterns = [
    /(\bfrom\s*)(['"])([^'"\n]+)\2/g,
    /(^\s*import\s*)(['"])([^'"\n]+)\2/gm,
    /(\bimport\s*\(\s*)(['"])([^'"\n]+)\2/g,
    /(\bvi\.(?:mock|doMock|unmock|importActual)\s*(?:<[^>]*>)?\s*\(\s*)(['"])([^'"\n]+)\2/g,
  ];
  const problems = [];
  const outputs = [];

  function relativeSpecifier(fromTarget, toTarget, original) {
    let relative = path.posix.relative(path.posix.dirname(fromTarget), toTarget);
    if (!relative.startsWith('.')) relative = './' + relative;
    const keepJs = /\.js$/.test(original);
    relative = relative.replace(/\.(tsx?|mts)$/, keepJs ? '.js' : '').replace(/\/index$/, keepJs ? '/index.js' : '');
    return relative;
  }

  for (const [file, target] of vendored) {
    const original = read(file);
    // Images/fonts/WASM and package documents are opaque original assets.
    // UTF-8 round-tripping corrupts their bytes and invalidates package hashes.
    if (!/\.(?:tsx?|[cm]?js|mts|cts)$/.test(file)) {
      outputs.push({source:file,sourceSHA256:sha(original),target,sha256:sha(original),rewritten:false,patched:[],bytes:original});
      continue;
    }
    let text = original.toString('utf8');
    const extraction = spec.extract?.find(entry => entry.file === file);
    if (extraction) {
      // Reuse intact upstream declarations when a file also contains unrelated
      // host transactions. Parse boundaries; never trim function bodies by regex.
      const ts = createRequire(import.meta.url)(path.join(root, 'apps/desktop/frontend/node_modules/typescript/lib/typescript.js'));
      const source = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true);
      const wanted = new Set(extraction.declarations), found = new Set();
      const selected = [];
      for (const node of source.statements) {
        if (ts.isImportDeclaration(node)) {
          if (extraction.imports?.includes(node.moduleSpecifier.text)) selected.push(node.getFullText(source));
        } else if (ts.isVariableStatement(node)) {
          // `const A = …` statements: take the whole statement when it declares
          // only wanted names, so no unrelated binding is carried along.
          const names = node.declarationList.declarations.map(declaration => declaration.name.text);
          if (names.length && names.every(name => wanted.has(name))) {
            for (const name of names) found.add(name);
            selected.push(node.getFullText(source));
          }
        } else if (node.name && wanted.has(node.name.text)) {
          found.add(node.name.text); selected.push(node.getFullText(source));
        }
      }
      for (const name of wanted) if (!found.has(name)) throw Error(`${file}: missing upstream declaration ${name}`);
      for (const name of extraction.exports ?? []) if (!found.has(name)) throw Error(`${file}: cannot export omitted declaration ${name}`);
      text = selected.join('\n') + '\nexport { ' + (extraction.exports ?? []).join(', ') + ' };\n';
    }
    for (const pattern of specifierPatterns) {
      text = text.replace(pattern, (whole, lead, quote, specifier) => {
        // Vite's plain JavaScript AudioWorklet is a file asset when imported
        // with ?url, unlike ordinary .js imports that resolve to TS sources.
        if ((/\.(css|json|svg|png|webp|jpg|jpeg|gif|wasm|woff2?)(\?.*)?$/.test(specifier) || /\.js\?.+$/.test(specifier)) && specifier.startsWith('.')) {
          const upstream = path.posix.normalize(path.posix.join(path.posix.dirname(file), specifier.replace(/\?.*$/, '')));
          const mapped = vendored.get(upstream) ?? seams[upstream];
          if (!mapped) { problems.push(`${file}: asset ${specifier} (${upstream}) is not vendored`); return whole; }
          const query = /\?.*$/.exec(specifier)?.[0] ?? '';
          let relative = path.posix.relative(path.posix.dirname(target), mapped);
          if (!relative.startsWith('.')) relative = './' + relative;
          return lead + quote + relative + query + quote;
        }
        const upstream = resolveUpstream(file, specifier);
        if (upstream === undefined) return whole; // external package
        if (upstream === null) { problems.push(`${file}: cannot resolve ${specifier}`); return whole; }
        if (typeof upstream === 'object') return lead + quote + relativeSpecifier(target, modules[upstream.module], specifier) + quote;
        const mapped = vendored.get(upstream) ?? seams[upstream];
        if (!mapped) { problems.push(`${file}: no vendored copy or seam for ${upstream} (imported as ${specifier})`); return whole; }
        return lead + quote + relativeSpecifier(target, mapped, specifier) + quote;
      });
    }
    for (const replacement of spec.replacements ?? []) {
      if (!text.includes(replacement.pattern)) continue;
      text = text.split(replacement.pattern).join(replacement.with);
      if (replacement.import) {
        const specifier = relativeSpecifier(target, replacement.import, '');
        const statement = `import { ${replacement.importName} } from '${specifier}';\n`;
        const lines = text.split('\n');
        const index = lines.findIndex((line) => /^import\s/.test(line));
        lines.splice(index < 0 ? 0 : index, 0, statement.trimEnd());
        text = lines.join('\n');
      }
    }
    const patched = extraction ? ['intact declarations: ' + extraction.declarations.join(', ')] : [];
    for (const patch of (spec.patches ?? []).filter((entry) => entry.file === file)) {
      const count = text.split(patch.find).length - 1;
      if (count !== (patch.count ?? 1)) { problems.push(`${file}: patch "${patch.reason}" matched ${count} times`); continue; }
      text = text.split(patch.find).join(patch.replace);
      patched.push(patch.reason);
    }
    const bytes = Buffer.from(text, 'utf8');
    outputs.push({source: file, sourceSHA256: sha(original), target, sha256: sha(bytes), rewritten: !bytes.equals(original), patched, bytes});
  }
  for (const patch of spec.patches ?? []) if (!vendored.has(patch.file)) problems.push(`patch "${patch.reason}" targets a file that is not vendored: ${patch.file}`);
  if (problems.length) throw Error('Cindy sync has unresolved imports:\n  ' + [...new Set(problems)].sort().join('\n  '));
  if (spec.locales) outputs.push(...extractLocales(spec.locales, outputs, read));

  const mismatched = [];
  for (const output of outputs) {
    const destination = safeTarget(output.target);
    if (mode === 'write') {
      fs.mkdirSync(path.dirname(destination), {recursive: true});
      fs.writeFileSync(destination, output.bytes);
    } else if (!fs.existsSync(destination) || sha(fs.readFileSync(destination)) !== output.sha256) {
      mismatched.push(output.target);
    }
  }
  // Files previously vendored by this spec but no longer produced. They are
  // reported, never deleted here: removal goes through the recycle bin after review.
  const produced = new Set(outputs.map((output) => output.target));
  const stale = (spec.files ?? []).map((entry) => entry.target).filter((target) => !produced.has(target) && fs.existsSync(safeTarget(target)));
  if (mode !== 'write' && stale.length) mismatched.push(...stale.map((target) => target + ' (stale)'));
  const files = outputs.map(({source, sourceSHA256, target, sha256, rewritten, patched, adaptation}) => ({source, sourceSHA256, target, sha256, ...(rewritten ? {adaptation: adaptation ?? (patched.length ? 'import-paths; ' + patched.join('; ') : 'import-paths')} : {})}));
  return {files, mismatched, stale};
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const specPath = path.resolve(root, argument('--spec', ''));
    const cindy = path.resolve(argument('--cindy', process.env.WINONE_CINDY_SOURCE ?? path.join(root, 'dist/resources/sources/cindy')));
    const mode = process.argv.includes('--write') ? 'write' : 'check';
    const spec = JSON.parse(fs.readFileSync(specPath, 'utf8'));
    if (mode === 'check' && !fs.existsSync(path.join(cindy, '.git'))) {
      // Clean checkouts have no Cindy clone: verify the recorded digests so a
      // hand edit to a vendored file still fails the build.
      const changed = (spec.files ?? []).filter((entry) => !fs.existsSync(safeTarget(entry.target)) || sha(fs.readFileSync(safeTarget(entry.target))) !== entry.sha256);
      if (changed.length) throw Error('Vendored Cindy files differ from their recorded digests:\n  ' + changed.map((entry) => entry.target).join('\n  '));
      console.log(`Cindy vendored digests verified: ${spec.files.length} files (upstream clone not present)`);
      process.exit(0);
    }
    const result = syncCindy({spec, cindy, mode});
    if (mode === 'write') {
      fs.writeFileSync(specPath, JSON.stringify({...spec, files: result.files}, null, 2) + '\n');
      console.log(`Cindy sync wrote ${result.files.length} files from ${spec.revision.slice(0, 9)}`);
      if (result.stale.length) console.warn('No longer vendored (move to the recycle bin after review):\n  ' + result.stale.join('\n  '));
    } else {
      const recorded = new Map((spec.files ?? []).map((entry) => [entry.target, entry]));
      for (const entry of result.files) {
        const prior = recorded.get(entry.target);
        if (!prior || prior.sha256 !== entry.sha256 || prior.sourceSHA256 !== entry.sourceSHA256) result.mismatched.push(entry.target + ' (record)');
      }
      if (result.mismatched.length) throw Error('Vendored Cindy files differ from the pinned sync:\n  ' + [...new Set(result.mismatched)].join('\n  '));
      console.log(`Cindy sync verified: ${result.files.length} files at ${spec.revision.slice(0, 9)}`);
    }
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
