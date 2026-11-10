import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFile} from 'node:fs/promises';
import {test} from 'node:test';

const root = new URL('../../', import.meta.url);
const read = path => readFile(new URL(path, root));
const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const source = JSON.parse(await read('third_party/wintray-source.json'));

// Upstream hashes alone can describe reviewed code that was never imported.
// Check the actual production bytes and adapted test bytes before building.
test('WinTray WeChat production implementation is the recorded original', async () => {
  assert.match(source.wechatAutoLogin.revision, /^[a-f0-9]{40}$/);
  const provider = source.files.find(file => file.path === 'internal/orchestrator/wechat_uia_windows.go');
  assert.ok(provider, 'missing upstream UIA attribution');
  assert.equal(provider.targetSHA256, provider.sourceSHA256, 'production code must remain byte-identical to upstream');
  assert.equal(digest(await read(provider.targetPath)), provider.sourceSHA256);
});

test('WinTray WeChat native regression suite and MIT license match their records', async () => {
  const suite = source.files.find(file => file.path === 'internal/orchestrator/wechat_uia_windows_test.go');
  assert.ok(suite, 'missing upstream UIA regression attribution');
  assert.match(suite.sourceSHA256, /^[a-f0-9]{64}$/);
  assert.match(suite.targetSHA256, /^[a-f0-9]{64}$/);
  assert.equal(digest(await read(suite.targetPath)), suite.targetSHA256);
  assert.equal(digest(await read(source.licenseFile)), source.licenseSHA256);
});

test('WinTray login, working directory and junction imports match reviewed target bytes', async () => {
  const paths = [
    'internal/config/schema.go', 'internal/orchestrator/login_tray_windows.go',
    'internal/orchestrator/qq_login_windows.go', 'internal/orchestrator/wechat_login_windows.go',
    'internal/orchestrator/runner.go', 'internal/orchestrator/runner_association_windows.go',
    'internal/startup/app_task_windows.go', 'internal/startup/original_launch_windows.go',
    'internal/traybox/process_windows.go',
  ];
  for (const path of paths) {
    const file = source.files.find(file => file.path === path);
    assert.ok(file?.targetPath, `missing target attribution: ${path}`);
    assert.equal(file.revision, source.revision);
    assert.equal(digest(await read(file.targetPath)), file.targetSHA256, path);
  }
});
