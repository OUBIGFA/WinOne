// Select notices from the binaries and source assets actually distributed.
// The repository retains complete dependency/verification records. The package
// keeps every selected original notice byte and a short attribution index.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const sha = bytes => createHash('sha256').update(bytes).digest('hex');

export function renderNotices(root, records) {
  const texts = new Map();
  const components = new Map();
  for (const record of records) {
    if (!record.notices?.length) throw Error('Missing component notices: ' + record.name);
    const references = [];
    for (const notice of record.notices) {
      if (!/^licenses\/[A-Za-z0-9._-]+\.txt$/.test(notice.file)) throw Error('Invalid notice path');
      const bytes = fs.readFileSync(path.join(root, 'third_party', notice.file));
      if (sha(bytes) !== notice.sha256) throw Error('Notice checksum mismatch: ' + notice.file);
      if (!texts.has(notice.sha256)) texts.set(notice.sha256, {bytes, id: 'L' + (texts.size + 1)});
      references.push(texts.get(notice.sha256).id);
    }
    const name = [record.name, record.version ?? record.commit ?? ''].filter(Boolean).join(' ');
    const key = name + '|' + references.join(',');
    components.set(key, `${name} [${references.join(', ')}]`);
  }
  const revision = execFileSync('git', ['rev-parse', 'HEAD'], {cwd: root, encoding: 'utf8', windowsHide: true}).trim();
  const index = Buffer.from('WinOne third-party components\n\nOriginal license and attribution texts are in THIRD-PARTY-LICENSES.txt.\nCorresponding source and build instructions\nhttps://github.com/OUBIGFA/WinOne/tree/' + revision + '\n\n' + [...components.values()].sort().join('\n') + '\n');
  const licenses = Buffer.concat([...texts.values()].flatMap(({bytes, id}) => [Buffer.from(`\n===== ${id} =====\n`), bytes, Buffer.from('\n')]));
  return {index, licenses, components: components.size, uniqueTexts: texts.size};
}

function selectRecords(root, bundle, go, cargo, desktop) {
  const read = name => JSON.parse(fs.readFileSync(path.join(root, 'third_party', name), 'utf8'));
  const catalogs = ['runtime-notices.json', 'ocr-ai-notices.json', 'result-ui-notices.json', 'desktop-mygo-notices.json', 'interconnect-worker-notices.json'].map(read);
  const available = catalogs.flatMap(catalog => catalog.components);
  const selected = [];
  const add = record => { if (!record) throw Error('A linked component has no recorded notice'); selected.push(record); };
  const binaries = ['WinOne.Host.exe', 'WinOne.WinTray.exe', 'WinOne.WinTray.Recovery.exe',
    'always-on-top/WinOne.AlwaysOnTop.exe', 'shared/ai/WinOne.AI.exe', 'shared/ui/WinOne.Results.exe',
    'shared/interconnect/WinOne.Interconnect.Worker.exe'];
  if (desktop !== 'Rust') binaries.push('WinOne.exe');
  for (const binary of binaries) {
    const info = execFileSync(go, ['version', '-m', path.join(bundle, binary)], {encoding: 'utf8', windowsHide: true});
    const version = info.split('\n')[0].trim().split(/\s+/).at(-1);
    add(available.find(record => record.kind === 'toolchain' && record.name === 'Go' && record.version === version));
    for (const line of info.split('\n')) {
      const fields = line.trim().split(/\s+/);
      if (fields[0] !== 'dep' || fields[1].startsWith('winone')) continue;
      add(available.find(record => ['go', 'magpie-go'].includes(record.kind) && record.name === fields[1] && record.version === fields[2]));
    }
  }
  if (fs.existsSync(path.join(bundle, 'capture/native-runtime.json'))) {
    for (const record of read('capture-native-notices.json').components) add(record);
  } else {
    const runtime = JSON.parse(fs.readFileSync(path.join(bundle, 'capture/WinOne.Capture.runtimeconfig.json'), 'utf8'));
    for (const framework of runtime.runtimeOptions.includedFrameworks) {
      add(available.find(record => record.kind === 'runtime' && record.name === framework.name && record.version === framework.version));
    }
    const dependencies = JSON.parse(fs.readFileSync(path.join(bundle, 'capture/WinOne.Capture.deps.json'), 'utf8'));
    for (const [identity, library] of Object.entries(dependencies.libraries)) {
      if (library.type !== 'package') continue;
      const slash = identity.lastIndexOf('/');
      add(available.find(record => record.kind === 'nuget' && record.name === identity.slice(0, slash) && record.version === identity.slice(slash + 1)));
    }
  }
  // All adapted source/assets listed here still have active product callers.
  for (const record of read('import-notices.json').imports) {
    if (fs.existsSync(path.join(bundle,'capture/native-runtime.json')) &&
        ['Tools3000 pin translation','OpenCV for Tools3000 pin translation'].includes(record.name)) continue;
    add(record);
  }
  for (const record of available) {
    if (['asset', 'native', 'model'].includes(record.kind) && !record.testOnly) add(record);
    if (record.kind === 'upstream' && ['PowerToys', 'Simple Icons (GitHub)', 'HushSnap', 'magpie', 'zencrop'].includes(record.name)) add(record);
  }
  const plugins = read('ai-plugins.json');
  add({name: 'Magpie community account plugins', version: plugins.revision, notices: [plugins.license]});
  add({name: 'Bun', version: plugins.bun.version, notices: [plugins.bun.license]});
  const learning = read('learning-data.json');
  add({name: 'ECDICT', version: learning.commit, notices: [{file: learning.licenseFile, sha256: learning.licenseSHA256}]});
  const rust = read('rust-notices.json');
  add(rust.toolchain);
  const projects = [
    ['diagram', 'apps/diagram-renderer/Cargo.toml'], ['selection', 'modules/selection/native/Cargo.toml'],
    ['recording', 'modules/recording/Cargo.toml'], ['interconnect-lan', 'third_party/file-share/src-tauri/Cargo.toml'],
  ];
  if (desktop === 'Rust') projects.push(['rust', 'apps/desktop-rust/Cargo.toml']);
  for (const [name, manifest] of projects) {
    // Normal target dependencies only. Build and dev-only dependencies do not
    // become distributable components merely by appearing in Cargo.lock.
    const tree = execFileSync(cargo, ['tree', '--locked', '--target', 'x86_64-pc-windows-msvc', '--edges', 'normal', '--prefix', 'none', '--format', '{p}', '--manifest-path', path.join(root, manifest)], {encoding: 'utf8', windowsHide: true});
    const actual = new Set(tree.split('\n').map(line => line.trim().split(/\s+/).slice(0, 2).join(' ')));
    for (const record of read(name + '-notices.json').packages) {
      if (actual.has(record.name + ' v' + record.version)) add(record);
    }
  }
  if (desktop === 'Rust') add(rust.sdk);
  if (desktop !== 'MyGo') {
    for (const record of available.filter(record => ['npm', 'upstream'].includes(record.kind))) add(record);
    for (const [name, notice] of Object.entries(catalogs[0].licenseTerms ?? {})) add({name: name + ' license terms', notices: [notice]});
  }
  return selected;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [bundle, output, go, cargo, desktop = 'MyGo'] = process.argv.slice(2);
  if (!bundle || !output || !go || !cargo) throw Error('Usage: bundle-notices.mjs <bundle> <output> <go> <cargo> <desktop>');
  const root = path.resolve(import.meta.dirname, '..');
  const records = selectRecords(root, path.resolve(bundle), go, cargo, desktop);
  const result = renderNotices(root, records);
  fs.mkdirSync(output, {recursive: true});
  fs.writeFileSync(path.join(output, 'THIRD-PARTY-NOTICES.txt'), result.index);
  fs.writeFileSync(path.join(output, 'THIRD-PARTY-LICENSES.txt'), result.licenses);
  console.log(`Bundled ${result.components} component attributions and ${result.uniqueTexts} original notice texts (${result.index.length + result.licenses.length} bytes)`);
}
