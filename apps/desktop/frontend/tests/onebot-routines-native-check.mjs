// build/test-desktop-components.ps1 owns the window, private desktop, profile
// and bundle. All UI calls below use its real Rust/Go/OneBot/Pi services.
import {createRequire} from 'node:module';
import {readFile, writeFile} from 'node:fs/promises';
import {resolve, join} from 'node:path';
import {pathToFileURL} from 'node:url';
import http from 'node:http';
import assert from 'node:assert/strict';
const {chromium} = createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint, output] = process.argv.slice(2), errors = [], views = [], requests = [];
let browser, page, devServer;
const vendor = http.createServer(async (request, response) => {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  const input = JSON.parse(Buffer.concat(chunks).toString('utf8'));
  assert.equal(request.headers.authorization, 'Bearer native-onebot-fixture');
  requests.push({model: input.model, maxTokens: input.max_tokens ?? input.max_completion_tokens ?? input.max_output_tokens, fields: Object.keys(input), roles: input.messages?.map(message => message.role)});
  await writeFile(resolve(output, 'provider-requests.json'), JSON.stringify(requests, null, 2));
  const nightly = input.messages.some(message => message.role === 'system' && String(message.content).includes("nightly learning"));
  const text = nightly ? JSON.stringify({memories: [{op: 'update', filename: 'user_style.md', title: 'Style', description: 'Reply style', body: 'Keep replies concise.', reason: 'Remove repeated wording'}]}) : 'NATIVE_ROUTINE_DONE';
  if (input.stream) {
    response.writeHead(200, {'Content-Type': 'text/event-stream'});
    response.end('data: ' + JSON.stringify({id: 'fixture', object: 'chat.completion.chunk', choices: [{index: 0, delta: {content: text}, finish_reason: null}]}) + '\n\ndata: ' + JSON.stringify({id: 'fixture', object: 'chat.completion.chunk', choices: [{index: 0, delta: {}, finish_reason: 'stop'}]}) + '\n\ndata: [DONE]\n\n');
  } else {
    response.writeHead(200, {'Content-Type': 'application/json'});
    response.end(JSON.stringify({choices: [{index: 0, finish_reason: 'stop', message: {role: 'assistant', content: text}}]}));
  }
});
await new Promise(resolve => vendor.listen(0, '127.0.0.1', resolve));
const call = (method, params = {}) => page.evaluate(async ({method, params}) => JSON.parse(await window.go.main.App.Chat(method, JSON.stringify(params))), {method, params});
const label = key => page.evaluate(key => window.oneBotRoutineFixture.t(key), key);
async function waitForReceipt(read, accepts) {
  const until = Date.now() + 30000;
  let value;
  while (Date.now() < until) {
    value = await read();
    if (accepts(value)) return value;
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  throw Error('Native receipt was not confirmed: ' + JSON.stringify(value));
}
try {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    try { browser = await chromium.connectOverCDP(endpoint, {timeout: 1000}); break; }
    catch { await new Promise(resolve => setTimeout(resolve, 100)); }
  }
  assert.ok(browser, 'the isolated native window did not expose its test connection');
  while (Date.now() < deadline) {
    page = browser.contexts().flatMap(context => context.pages()).find(page => page.url().includes('winone.localhost'));
    if (page) break;
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert.ok(page, 'native document is missing');
  page.setDefaultTimeout(12000);
  page.on('pageerror', error => errors.push(String(error.stack ?? error)));
  await page.locator('.app-navigation').waitFor({timeout: 30000});
  const baseURL = 'http://127.0.0.1:' + vendor.address().port + '/v1';
  await page.evaluate(async baseURL => {
    const app = window.go.main.App;
    const configuration = JSON.parse(await app.Capture('ai.configuration.get', 'null'));
    configuration.providers = [{id: 'nativefixture', name: 'Native fixture', authMode: 'api-key', protocol: 'chat', baseUrl: baseURL, models: ['native-model']}];
    configuration.tasks.chat = {provider: 'nativefixture', model: 'native-model'};
    configuration.magpieSettings = {...configuration.magpieSettings, noAccountDiscovery: true};
    await app.Capture('ai.configuration.save', JSON.stringify({configuration, keys: {nativefixture: 'native-onebot-fixture'}}));
  }, baseURL);
  const profile = await call('onebot.profiles.create', {input: {id: 'native-routines', name: 'Native routines'}});
  await call('onebot.bots.createCanonicalSession', {botId: 'native-routines', expectedCanonicalSessionId: null, expectedProfileVersion: profile.currentVersion});
  const routine = await call('onebot.routines.save', {botId: 'native-routines', input: {name: 'Native routine', prompt: 'Return NATIVE_ROUTINE_DONE.', enabled: true, silentWhenIdle: true, triggers: [{id: 'daily', kind: 'cron', expression: '0 9 * * *', timezone: 'Asia/Shanghai'}]}});

  const frontend = resolve('apps/desktop/frontend');
  const {createServer} = await import(pathToFileURL(join(frontend, 'node_modules/vite/dist/node/index.js')));
  devServer = await createServer({root: frontend, configFile: join(frontend, 'vite.config.ts'), server: {host: '127.0.0.1', port: 5219, strictPort: true, hmr: false}});
  await devServer.listen();
  await page.route('http://winone.localhost/**', async route => {
    const url = new URL(route.request().url());
    const pathname = url.pathname === '/' || url.pathname === '/index.html' ? '/tests/onebot-routines-native-preview.html' : url.pathname;
    const suffix = pathname.endsWith('onebot-routines-native-preview.html') ? '?botId=native-routines' : url.search;
    const response = await fetch('http://127.0.0.1:5219' + pathname + suffix);
    await route.fulfill({status: response.status, contentType: response.headers.get('content-type') ?? 'application/octet-stream', body: Buffer.from(await response.arrayBuffer())});
  });
  await page.goto('http://winone.localhost/?botId=native-routines', {waitUntil: 'domcontentloaded'});
  await page.locator('[data-onebot-routines-fixture]').waitFor();
  await page.getByRole('button', {name: /^Native routine/}).click();
  await page.getByRole('textbox', {name: await label('routines.name'), exact: true}).fill('Native routine saved');
  await page.getByText(await label('routines.advancedSettings'), {exact: true}).click();
  await page.getByRole('textbox', {name: await label('routines.checkCommand'), exact: true}).fill('exit 0');
  await page.getByRole('button', {name: await label('routines.save'), exact: true}).click();
  await waitForReceipt(() => call('onebot.routines.list', {botId: 'native-routines'}), rows => rows.some(row => row.id === routine.id && row.name === 'Native routine saved' && row.preRunHook?.command === 'exit 0'));
  await page.getByRole('button', {name: await label('routines.runNow'), exact: true}).click();
  await waitForReceipt(() => call('onebot.routines.history', {botId: 'native-routines', id: routine.id}), rows => rows[0]?.status === 'success' && rows[0]?.resultText === 'NATIVE_ROUTINE_DONE');
  await page.getByRole('button', {name: await label('routines.back'), exact: true}).click();
  await page.getByRole('button', {name: await label('routines.nightly.viewHistory'), exact: true}).click();
  await page.getByRole('switch', {name: await label('routines.nightly.switchLabel'), exact: true}).click();
  await waitForReceipt(() => call('onebot.routines.nightly.get', {botId: 'native-routines'}), view => view.enabled === false);
  await page.getByRole('radio', {name: await label('routines.nightly.weekly'), exact: true}).click();
  const nightly = await waitForReceipt(() => call('onebot.routines.nightly.get', {botId: 'native-routines'}), view => view.frequency === 'weekly');
  assert.equal(nightly.enabled, false);
  assert.equal(nightly.frequency, 'weekly');
  await call('onebot.memory.write', {botId: 'native-routines', input: {type: 'user', name: 'style', title: 'Style', description: 'Reply style', body: 'Keep replies concise. Keep replies concise.'}});
  await page.getByRole('button', {name: await label('routines.nightly.runNow'), exact: true}).click();
  await waitForReceipt(() => call('onebot.routines.nightly.get', {botId: 'native-routines'}), view => view.lastRun?.outcome === 'learned' && !view.running);
  await page.getByRole('button', {name: await label('routines.nightly.undo'), exact: true}).click();
  await waitForReceipt(() => call('onebot.memory.read', {botId: 'native-routines', filename: 'user_style.md'}), row => row.body === 'Keep replies concise. Keep replies concise.');
  await page.getByRole('button', {name: await label('routines.nightly.redo'), exact: true}).click();
  await waitForReceipt(() => call('onebot.memory.read', {botId: 'native-routines', filename: 'user_style.md'}), row => row.body === 'Keep replies concise.');
  assert.ok(requests.some(request => request.maxTokens === 8000), 'nightly output budget did not reach the real provider');
  for (const language of ['zh-CN', 'en-US']) for (const theme of ['light', 'dark']) for (const width of [320, 520, 1000]) {
    await page.setViewportSize({width, height: 820});
    await page.evaluate(async values => window.oneBotRoutineFixture.appearance(values.language, values.theme), {language, theme});
    await page.evaluate(async () => { await document.fonts.ready; });
    await page.mouse.move(0, 0);
    await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running'));
    const layout = await page.evaluate(() => ({width: innerWidth, scrollWidth: document.scrollingElement.scrollWidth,
      main: {width: document.querySelector('main').clientWidth, scrollWidth: document.querySelector('main').scrollWidth},
      buttons: Array.from(document.querySelectorAll('.cindy-button')).map(button => {const style=getComputedStyle(button), face=getComputedStyle(button,'::before');return {text:button.textContent,disabled:button.disabled,color:style.color,background:style.backgroundColor,face:face.backgroundColor,foreground:style.getPropertyValue('--foreground'),textPrimary:style.getPropertyValue('--text-primary')};})}));
    assert.ok(layout.scrollWidth <= layout.width + 1, JSON.stringify({language, theme, layout}));
    assert.ok(layout.main.scrollWidth <= layout.main.width + 1, JSON.stringify({language, theme, layout}));
    const name = language + '-' + theme + '-' + width + '.png';
    await page.screenshot({path: resolve(output, name)});
    views.push({language, theme, width, layout, screenshot: name});
  }
  assert.deepEqual(errors, []);
  await writeFile(resolve(output, 'result.json'), JSON.stringify({originalComponents: true, realNativeHost: true, routineSaved: true, preRunCheckExecuted: true, realPiExecution: true, nightlyPreferencesSaved: true, realNightlyExecution: true, realUndoRedo: true, requests, views, errors}, null, 2));
  await page.evaluate(() => { void window.go.main.App.WindowCommand('close'); }).catch(error => { if (!String(error).includes('closed')) throw error; });
} catch (error) {
  await writeFile(resolve(output, 'provider-requests.json'), JSON.stringify(requests, null, 2));
  await writeFile(resolve(output, 'node-error.txt'), String(error.stack ?? error));
  if (page) {
    await page.screenshot({path: resolve(output, 'failure.png')}).catch(() => {});
    await writeFile(resolve(output, 'failure.txt'), await page.locator('body').innerText().catch(() => String(error)));
  }
  throw error;
} finally {
  await devServer?.close();
  await browser?.close();
  vendor.closeAllConnections();
  await new Promise(resolve => vendor.close(resolve));
}
