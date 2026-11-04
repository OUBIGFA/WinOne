// build/check-cindy-ports.mjs against a throwaway upstream repository: the
// manifest is verified, upstream changes are reported per port, --strict
// fails while a port is behind, and --accept moves it forward.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {execFileSync, spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const script = path.join(root, 'build/check-cindy-ports.mjs');
const sha = (text) => createHash('sha256').update(text).digest('hex');

function upstream() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'cindy-ports-'));
  const git = (...args) => execFileSync('git', ['-C', dir, ...args], {encoding: 'utf8'}).trim();
  git('init', '-q');
  git('config', 'user.email', 'test@example.invalid');
  git('config', 'user.name', 'test');
  git('config', 'core.autocrlf', 'false');
  const commit = (files) => {
    for (const [file, text] of Object.entries(files)) {
      fs.mkdirSync(path.dirname(path.join(dir, file)), {recursive: true});
      fs.writeFileSync(path.join(dir, file), text);
    }
    git('add', '-A');
    git('commit', '-q', '-m', 'c');
    return git('rev-parse', 'HEAD');
  };
  return {dir, commit, cleanup: () => fs.rmSync(dir, {recursive: true, force: true})};
}

function run(cindy, manifest, ...args) {
  return spawnSync(process.execPath, [script, '--cindy', cindy, '--manifest', manifest, ...args], {encoding: 'utf8'});
}

test('ports are reported behind until accepted', (t) => {
  const repo = upstream();
  t.after(repo.cleanup);
  const first = repo.commit({'src/send.ts': 'v1\n', 'src/other.ts': 'x\n'});
  const manifest = path.join(repo.dir, '..', path.basename(repo.dir) + '.json');
  t.after(() => fs.rmSync(manifest, {force: true}));
  fs.writeFileSync(manifest, JSON.stringify({ports: [{
    id: 'composer', native: ['build/check-cindy-ports.mjs'],
    upstream: [{path: 'src/send.ts', revision: first, sha256: sha('v1\n')}],
  }]}));

  let result = run(repo.dir, manifest, '--ref', 'HEAD', '--strict');
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /ok {6}composer/);

  repo.commit({'src/other.ts': 'y\n'});
  assert.equal(run(repo.dir, manifest, '--ref', 'HEAD', '--strict').status, 0, 'an unrelated upstream file does not make the port behind');

  const second = repo.commit({'src/send.ts': 'v2\nmore\n'});
  result = run(repo.dir, manifest, '--ref', 'HEAD', '--strict');
  assert.equal(result.status, 1);
  assert.match(result.stdout, /behind {2}composer/);
  assert.match(result.stdout, /src\/send\.ts {2}\+2 -1/);

  assert.equal(run(repo.dir, manifest, '--ref', 'HEAD', '--accept', 'composer').status, 0);
  const accepted = JSON.parse(fs.readFileSync(manifest, 'utf8')).ports[0].upstream[0];
  assert.deepEqual(accepted, {path: 'src/send.ts', revision: second, sha256: sha('v2\nmore\n')});
  assert.equal(run(repo.dir, manifest, '--ref', 'HEAD', '--strict').status, 0);
});

test('a wrong record or a missing native file fails the check', (t) => {
  const repo = upstream();
  t.after(repo.cleanup);
  const first = repo.commit({'src/send.ts': 'v1\n'});
  const manifest = path.join(repo.dir, '..', path.basename(repo.dir) + '.json');
  t.after(() => fs.rmSync(manifest, {force: true}));
  fs.writeFileSync(manifest, JSON.stringify({ports: [{
    id: 'composer', native: ['apps/does-not-exist.go'],
    upstream: [{path: 'src/send.ts', revision: first, sha256: sha('other\n')}],
  }]}));
  const result = run(repo.dir, manifest, '--ref', 'HEAD');
  assert.equal(result.status, 1);
  assert.match(result.stderr, /native file missing: apps\/does-not-exist\.go/);
  assert.match(result.stderr, /src\/send\.ts SHA-256 differs/);
  assert.equal(run(repo.dir, manifest, '--ref', 'HEAD', '--accept', 'composer').status, 1, 'accept refuses a broken manifest');
});
