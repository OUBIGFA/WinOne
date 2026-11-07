import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';

const root = fileURLToPath(new URL('../../../', import.meta.url));

test('MCP Apps real native plugin fixture uses production Host, official App and isolated WebView2', {skip: !process.env.WINONE_GHOST_SANDBOX, timeout: 170000}, async () => {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(root, '_temp/plugin-task-close/resume-20261106/mcpapps/system');
  await fs.mkdir(base, {recursive: true});
  const directory = await fs.mkdtemp(path.join(base, 'native-mcp-'));
  const child = spawn('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', path.join(root, 'build/test-native-ui.ps1'),
    '-Modules', 'desktop-mygo', '-Packages', './chat/pluginhost/mcpapp', '-Run', '^TestNativeMcpAppEndToEndUsesActualPluginBackendAndOfficialApp$'], {
    cwd: root, windowsHide: true, env: {...process.env, WINONE_MCP_APPS_NATIVE: '1', WINONE_MCP_APPS_NATIVE_PROFILE: directory, WINONE_MCP_APPS_BUN: process.execPath},
  });
  const chunks = [];
  child.stdout.on('data', data => chunks.push(data)); child.stderr.on('data', data => chunks.push(data));
  const timer = setTimeout(() => child.kill(), 165000);
  const result = await new Promise((resolve, reject) => {child.once('error', reject); child.once('exit', code => resolve(code));}).finally(() => clearTimeout(timer));
  const log = Buffer.concat(chunks).toString('utf8');
  await fs.writeFile(path.join(directory, 'native-system.log'), log);
  assert.equal(result, 0, 'Actual plugin/official App/native system check failed; see ' + path.join(directory, 'native-system.log') + '\n' + log.slice(-12000));
});
