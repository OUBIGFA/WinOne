// Production settings UI with an in-memory Host fixture. This browser check
// complements, but never replaces, reviewing the real isolated WinOne window.
// node build/test-settings-design.mjs
// Set WINONE_SETTINGS_QUICK=1 for one visual pass; the default also checks the
// complete language/theme/width matrix, state transitions and keyboard use.
import {createRequire} from 'node:module';
import {mkdir, writeFile} from 'node:fs/promises';
import {resolve, join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';

const root = resolve('.'), frontend = resolve('apps/desktop/frontend');
const output = resolve(process.env.WINONE_LAYOUT_OUTPUT || '_temp/20261015-settings-design/review');
execFileSync('git', ['check-ignore', '--quiet', '--no-index', join(output, 'probe')]);
await mkdir(join(output, 'browser-temp'), {recursive: true});
process.env.TEMP = process.env.TMP = join(output, 'browser-temp');
const {chromium} = createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE || resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer} = await import(pathToFileURL(join(frontend, 'node_modules/vite/dist/node/index.js')));
const server = await createServer({root: frontend, configFile: join(frontend, 'vite.config.ts'), server: {host: '127.0.0.1', port: 5198, strictPort: true}});
await server.listen();
const browser = await chromium.launch({headless: true, ...(process.env.WINONE_CHROMIUM ? {executablePath: process.env.WINONE_CHROMIUM} : {channel: 'msedge'})});
const results = [], errors = [], interactions = [];
const quick = process.env.WINONE_SETTINGS_QUICK === '1';
const pages = process.env.WINONE_SETTINGS_PAGES?.split(',') || ['overview', 'settings', 'capture', 'selection', 'quick-assistant', 'wintray', 'convenience', 'ai-services'];
const matrix = process.env.WINONE_SETTINGS_SKIP_MATRIX === '1' ? [] : quick ? [{width: 1280, language: 'zh-CN', theme: 'light'}] : [520, 900, 1280].flatMap(width => ['zh-CN', 'en-US'].flatMap(language => ['light', 'dark'].map(theme => ({width, language, theme}))));
let screenshotIndex = 0;

async function settle(page) {
  await page.evaluate(async () => {await document.fonts.ready; await new Promise(requestAnimationFrame); await new Promise(requestAnimationFrame);});
}
async function open(page, id, config, extra = {}) {
  const params = new URLSearchParams({page: id, language: config.language, theme: config.theme, ...extra});
  await page.goto(`http://127.0.0.1:5198/tests/settings-design-preview.html?${params}`);
  await page.locator(`[data-slot="page-scroll"][data-page="${id}"]`).waitFor();
  await settle(page);
}
async function top(page) {
  await page.locator('[data-slot="page-scroll"]').evaluate(element => {element.scrollTop = 0;});
  await settle(page);
}
async function geometry(page) {
  return page.locator('[data-slot="page-content"]').evaluate(root => {
    const visible = element => {
      const rect = element.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && getComputedStyle(element).visibility !== 'hidden' && !element.closest('[aria-hidden="true"]');
    };
    const bounds = root.getBoundingClientRect();
    const containers = [document.documentElement, document.body, root, root.parentElement, ...root.querySelectorAll('.settings-group,.settings-group-body,.settings-field-grid,.settings-dependent,.wintray-program-detail,.wintray-program-list,.selection-action-row,.directory-links')];
    const overflow = containers.filter(element => visible(element) && element.scrollWidth > element.clientWidth + 1).map(element => ({element: element.className || element.tagName, client: element.clientWidth, scroll: element.scrollWidth}));
    const controls = [...root.querySelectorAll('button,input,textarea,summary,[role="combobox"]')].filter(visible);
    const clipped = controls.filter(element => {const rect = element.getBoundingClientRect(); return rect.left < bounds.left - 1 || rect.right > bounds.right + 1;}).map(element => ({name: element.getAttribute('aria-label') || element.id || element.textContent.slice(0, 100)}));
    const hiddenPanels = [...root.querySelectorAll('[data-slot="tabs-content"][hidden]')].filter(visible).map(element => element.id);
    const groups = [...root.querySelectorAll('.settings-group-heading')].filter(visible).flatMap(heading => {
      const body = heading.nextElementSibling;
      if (!body?.classList.contains('settings-group-body') || !visible(body)) return [];
      const title = heading.querySelector('.settings-group-title');
      const headingRect = heading.getBoundingClientRect(), bodyRect = body.getBoundingClientRect();
      const sideBySide = bodyRect.left >= headingRect.right;
      return [{title: title?.textContent, layout: sideBySide ? 'rail' : 'stack',
        gap: sideBySide ? bodyRect.left - headingRect.right : bodyRect.top - headingRect.bottom,
        aligned: !sideBySide || Math.abs(bodyRect.top - headingRect.top) <= 9,
        titleSize: parseFloat(getComputedStyle(title).fontSize)}];
    });
    const badGroups = groups.filter(group => group.gap < 19 || group.titleSize < 16 || !group.aligned);
    // A heading/body check alone missed the real regressions: adjacent groups
    // touching, preference rows stuck together, and text wrapping inside tabs.
    const groupSpacing = [...root.querySelectorAll('.settings-group')].filter(visible).flatMap(group => {
      const next = group.nextElementSibling;
      if (!next?.classList.contains('settings-group') || !visible(next)) return [];
      const before = group.getBoundingClientRect(), after = next.getBoundingClientRect();
      if (after.left >= before.right - 1 || after.right <= before.left + 1) return [];
      const heading = next.querySelector('.settings-group-heading,.settings-group-title');
      if (!heading) return [];
      return [{name: heading.textContent, gap: heading.getBoundingClientRect().top - group.getBoundingClientRect().bottom}];
    });
    const preferences = [...root.querySelectorAll('.settings-choice-field,.settings-option-field')].filter(visible);
    const preferenceSpacing = preferences.slice(1).flatMap((field, index) => field.parentElement === preferences[index].parentElement
      ? [{name: field.textContent, gap: field.getBoundingClientRect().top - preferences[index].getBoundingClientRect().bottom}] : []);
    const choiceGroups = [...root.querySelectorAll('.settings-options')].filter(visible).map(group => {
      const choices = [...group.querySelectorAll('.settings-choice')].map(choice => choice.getBoundingClientRect());
      return {name: document.getElementById(group.getAttribute('aria-labelledby'))?.textContent,
        rows: new Set(choices.map(rect => Math.round(rect.top))).size,
        columns: new Set(choices.map(rect => Math.round(rect.left))).size,
        minHeight: Math.min(...choices.map(rect => rect.height)),
        selected: group.querySelectorAll('[role="radio"][aria-checked="true"]').length};
    });
    const tabs = [...root.querySelectorAll('[data-slot="tabs-trigger"]')].filter(visible).map(tab => {
      const label = tab.querySelector('span > span') || tab;
      const range = document.createRange(); range.selectNodeContents(label);
      const lines = new Set([...range.getClientRects()].filter(rect => rect.width > 0).map(rect => Math.round(rect.y)));
      const truncated = label.clientWidth > 0 && label.scrollWidth > label.clientWidth + 1;
      return {name: tab.textContent, lines: lines.size, truncated, tooltip: tab.getAttribute('title') || label.getAttribute('title')};
    });
    // A navigation name that fits must not be squeezed onto a second line.
    const scopeLabels = [...root.querySelectorAll('.stroke-scene-label')].filter(visible).map(label => {
      const name = [...label.childNodes].find(node => node.nodeType === 3 && node.textContent.trim());
      const range = document.createRange(); range.selectNodeContents(name || label);
      return {name: label.textContent, lines: new Set([...range.getClientRects()].filter(rect => rect.width > 0).map(rect => Math.round(rect.y))).size};
    });
    const branding = document.querySelector('[data-settings-header="true"] [data-slot="window-branding"]');
    const brandName = branding?.querySelector('[data-slot="window-brand-name"]');
    const titlebarOverlap = !!(brandName && brandName.getBoundingClientRect().right > branding.getBoundingClientRect().right + 1);
    const startPage = root.querySelector('#default-page');
    const jumpButtons = document.querySelector('[data-slot="page-scroll-controls"]');
    const footerOverlap = !!(startPage && jumpButtons && (() => {
      const a = startPage.getBoundingClientRect(), b = jumpButtons.getBoundingClientRect(), clip = root.parentElement.getBoundingClientRect();
      // A partially scrolled field is clipped by its viewport. Only its visible
      // portion can be covered; the utility footer is outside that viewport.
      return a.right > b.left && a.left < b.right && Math.min(a.bottom, clip.bottom) > b.top && Math.max(a.top, clip.top) < b.bottom;
    })());
    const padding = getComputedStyle(root);
    const hierarchyFailures = [
      ...(titlebarOverlap ? [{kind: 'titlebar-overlap'}] : []),
      ...(footerOverlap ? [{kind: 'start-page-covered-by-scroll-buttons'}] : []),
      ...groupSpacing.filter(item => item.gap < 32).map(item => ({kind: 'group-spacing', ...item})),
      ...preferenceSpacing.filter(item => item.gap < 12).map(item => ({kind: 'preference-spacing', ...item})),
      ...choiceGroups.filter(item => item.rows > 1 && item.columns > 1 || item.minHeight < 40 || item.selected !== 1).map(item => ({kind: 'choice-group', ...item})),
      ...tabs.filter(tab => tab.lines > 1 || tab.truncated && tab.tooltip !== tab.name).map(item => ({kind: 'tab-title', ...item})),
      ...scopeLabels.filter(item => item.lines > 1).map(item => ({kind: 'scope-name-wrap', ...item})),
      ...(parseFloat(padding.paddingLeft) < 16 || parseFloat(padding.paddingRight) < 16 ? [{kind: 'page-inset'}] : []),
    ];
    return {overflow, clipped, hiddenPanels, groups, badGroups, groupSpacing, preferenceSpacing, choiceGroups, tabs, hierarchyFailures, horizontalScrollbar: root.parentElement.scrollWidth > root.parentElement.clientWidth + 1};
  });
}
async function capture(page, prefix) {
  const scroll = page.locator('[data-slot="page-scroll"]');
  const size = await scroll.evaluate(element => ({height: element.clientHeight, total: element.scrollHeight}));
  const positions = [...new Set([0, ...(size.total > size.height * 1.3 ? [Math.min(size.height * .8, size.total - size.height), size.total - size.height] : [])])];
  const files = [];
  for (let part = 0; part < positions.length; part++) {
    await scroll.evaluate((element, value) => {element.scrollTop = value;}, positions[part]);
    await settle(page); await page.mouse.move(0, 0);
    const file = `${String(++screenshotIndex).padStart(3, '0')}-${prefix}-${part}.png`;
    await page.screenshot({path: join(output, file)}); files.push(file);
  }
  await top(page);
  return files;
}
async function inspect(page, id, config, path, tabsOrigin) {
  await top(page);
  const metrics = await geometry(page);
  const tabs = page.getByRole('tablist').first();
  const current = await tabs.count() ? await tabs.boundingBox() : undefined;
  const navigationMoved = !!(current && tabsOrigin && (Math.abs(current.x - tabsOrigin.x) > 1 || Math.abs(current.y - tabsOrigin.y) > 1));
  const images = (config.width === 1280 && config.language === 'zh-CN' && config.theme === 'light' || config.width === 520 && config.language === 'en-US' && config.theme === 'dark')
    ? await capture(page, `${id}-${config.width}-${config.language}-${config.theme}-${path.join('-') || 'default'}`) : [];
  results.push({page: id, ...config, path, ...metrics, navigationMoved, images});
}
async function visitTabs(page, id, config, depth = 0, path = [], origin) {
  const list = page.getByRole('tablist').nth(depth);
  if (!await list.count() || depth > 2) {await inspect(page, id, config, path, origin); return;}
  const names = (await list.getByRole('tab').allTextContents()).map(name => name.trim());
  for (let index = 0; index < names.length; index++) {
    const tab = page.getByRole('tablist').nth(depth).getByRole('tab', {name: names[index], exact: true});
    if (await tab.isDisabled()) throw Error(`Unexpected unavailable settings tab: ${id}/${names[index]}`);
    await tab.click(); await settle(page); await top(page);
    if (!origin) origin = await page.getByRole('tablist').first().boundingBox();
    await visitTabs(page, id, config, depth + 1, [...path, String(index)], origin);
  }
}
async function clickTab(page, name, list = 0) {await page.getByRole('tablist').nth(list).getByRole('tab', {name, exact: true}).click(); await settle(page);}

try {
  for (const config of matrix) {
    const page = await browser.newPage({viewport: {width: config.width, height: 940}, reducedMotion: 'reduce', colorScheme: config.theme});
    page.on('pageerror', error => errors.push({url: page.url(), error: String(error)}));
    for (const id of pages) {
      await open(page, id, config);
      await visitTabs(page, id, config);
    }
    console.log(`Reviewed settings: ${config.width} ${config.language} ${config.theme}`);
    await page.close();
  }
  if (!quick && process.env.WINONE_SETTINGS_SKIP_INTERACTIONS !== '1') {
    const page = await browser.newPage({viewport: {width: 1050, height: 900}, reducedMotion: 'reduce'});
    page.on('pageerror', error => errors.push({url: page.url(), error: String(error)}));
    const config = {width: 1050, language: 'zh-CN', theme: 'light'};

    // All global tasks stay mounted, while the expensive diagnostics read is
    // deferred. A recorded shortcut must survive another settings category.
    await open(page, 'settings', config);
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.calls.some(call => call.method === 'logs.read')), false);
    const general = page.getByRole('tab', {name: '常规', exact: true});
    await general.focus(); await page.keyboard.press('ArrowRight'); await page.keyboard.press('Enter'); await settle(page);
    assert.equal(await page.getByRole('tab', {name: '启动与快捷键'}).getAttribute('aria-selected'), 'true');
    const shortcut = page.getByRole('textbox', {name: '打开 WinOne', exact: true});
    await shortcut.click(); await page.getByText('请按下组合键，Esc 取消，Delete 清除。', {exact: true}).waitFor();
    await page.keyboard.press('Control+Alt+F8');
    await clickTab(page, '关于与诊断'); await page.getByRole('button', {name: '刷新日志', exact: true}).waitFor();
    await clickTab(page, '启动与快捷键'); assert.equal(await shortcut.inputValue(), 'Ctrl+Alt+F8');
    await page.getByRole('button', {name: '保存快捷键', exact: true}).click();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().globalSettings.openShortcut), 'Ctrl+Alt+F8');
    interactions.push('global tabs / keyboard navigation / shortcut draft / lazy diagnostics / save');

    await clickTab(page, '常规');
    const theme = page.getByRole('radiogroup', {name: '主题', exact: true});
    const darkChoice = theme.locator('label.settings-choice').filter({hasText: '深色'});
    const box = await darkChoice.boundingBox();
    // Click the empty area, not the radio or text: the whole choice is a target.
    await darkChoice.click({position: {x: box.width - 12, y: box.height / 2}});
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().globalSettings.theme), 'dark');
    assert.equal(await page.locator('html').getAttribute('data-theme'), 'dark');
    await theme.getByRole('radio', {name: '深色', exact: true}).focus();
    await page.keyboard.press('ArrowLeft');
    assert.equal(await theme.getByRole('radio', {name: '浅色', exact: true}).getAttribute('aria-checked'), 'true');
    await page.getByRole('radio', {name: 'English', exact: true}).click();
    await page.getByRole('radiogroup', {name: 'Language', exact: true}).waitFor();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().globalSettings.language), 'en-US');
    await page.getByRole('radio', {name: '简体中文', exact: true}).click();
    await page.getByRole('radiogroup', {name: '语言', exact: true}).waitFor();
    await page.getByRole('radio', {name: '显示图标和名称', exact: true}).click();
    await page.reload(); await settle(page);
    assert.equal(await page.getByRole('radio', {name: '显示图标和名称', exact: true}).getAttribute('aria-checked'), 'true');
    assert.ok((await page.locator('.app-navigation').boundingBox()).width >= 180);
    await page.getByRole('radio', {name: '只显示图标', exact: true}).click();
    const navigationNames = (await page.locator('.app-navigation').getByRole('button').allTextContents()).map(name => name.trim()).filter(Boolean);
    await page.getByRole('combobox', {name: '起始页面', exact: true}).click();
    await page.getByRole('option', {name: '首页', exact: true}).waitFor();
    assert.deepEqual(await page.getByRole('option').allTextContents(), navigationNames);
    await page.getByRole('option', {name: '截图助手', exact: true}).click();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().globalSettings.defaultPage), 'capture');
    assert.equal(await page.locator('[data-settings-page="settings"]').count(), 1);
    interactions.push('whole choice target / theme keyboard / live language / sidebar reload / complete start-page catalog');

    await open(page, 'settings', config, {failure: 'settings.save'});
    const beforeFailure = await page.getByRole('tablist').first().boundingBox();
    await page.getByRole('radio', {name: '深色', exact: true}).click();
    await page.getByRole('alert').waitFor();
    assert.equal(await page.getByRole('radio', {name: '浅色', exact: true}).getAttribute('aria-checked'), 'true');
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().globalSettings.theme), 'light');
    const afterFailure = await page.getByRole('tablist').first().boundingBox();
    assert.ok(Math.abs(beforeFailure.x - afterFailure.x) <= 1 && Math.abs(beforeFailure.y - afterFailure.y) <= 1, 'A preference error must not move settings navigation');
    await page.evaluate(() => window.settingsDesignFixture.recover());
    await page.getByRole('radio', {name: '深色', exact: true}).click();
    assert.equal(await page.getByRole('alert').count(), 0);
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().globalSettings.theme), 'dark');
    interactions.push('preference save failure / unchanged selection / stable navigation / successful retry');

    await open(page, 'capture', config);
    await clickTab(page, '滚动截图');
    const delay = page.getByLabel('滚动间隔（毫秒）', {exact: true}); await delay.fill('450');
    await clickTab(page, '截图后'); await clickTab(page, '滚动截图'); assert.equal(await delay.inputValue(), '450');
    await delay.fill(''); await clickTab(page, '截图后');
    await page.getByRole('button', {name: '检查滚动间隔'}).click(); assert.equal(await delay.isVisible(), true);
    await delay.fill('450'); await page.getByRole('button', {name: '保存设置', exact: true}).click();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().capture.scrollDelay), 450);
    interactions.push('capture draft / cross-tab validation / return to invalid field / save');

    await open(page, 'selection', config);
    await page.getByRole('button', {name: '编辑 解释', exact: true}).click();
    const prompt = page.getByLabel('AI 指令', {exact: true}); await prompt.fill('用于验证的未保存指令');
    await page.getByRole('button', {name: '编辑 解释', exact: true}).click();
    await clickTab(page, '常规');
    await page.getByRole('switch', {name: '划词后显示工具栏', exact: true}).click();
    assert.equal(await page.getByRole('switch', {name: '键盘选中文字后也显示', exact: true}).isDisabled(), true);
    await clickTab(page, '工具栏'); await page.getByRole('button', {name: '编辑 解释', exact: true}).click();
    assert.equal(await prompt.inputValue(), '用于验证的未保存指令');
    await page.getByRole('button', {name: '保存设置', exact: true}).click();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().selection.actions.find(action => action.id === 'explain').prompt), '用于验证的未保存指令');
    interactions.push('assistant accordion / hidden-label controls / parent dependency / draft / save');

    await open(page, 'wintray', config);
    // The row also contains switches and menus whose clicks do not select it.
    await page.getByRole('listitem', {name: '音乐播放器', exact: true}).getByText('音乐播放器', {exact: true}).click();
    await page.locator('.wintray-detail-header').getByRole('heading', {name: '音乐播放器', exact: true}).waitFor();
    assert.equal(await page.getByRole('switch', {name: '限制自动运行次数', exact: true}).isDisabled(), true);
    await page.getByRole('switch', {name: '自定义时间', exact: true}).click();
    await page.getByRole('switch', {name: '限制自动运行次数', exact: true}).click();
    await page.getByLabel('最多运行次数', {exact: true}).fill('3');
    await clickTab(page, '程序信息', 1); await clickTab(page, '启动与计划', 1);
    assert.equal(await page.getByLabel('最多运行次数', {exact: true}).inputValue(), '3');
    await page.getByRole('button', {name: '保存修改', exact: true}).click();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().programs.managedApps[1].schedule.frequencyRuns), 3);
    interactions.push('program run limits within time plan / dependency / cross-tab draft / save');

    await open(page, 'ai-services', config);
    const address = page.getByLabel('API 基础地址', {exact: true}); await address.fill('https://example.invalid/preserved');
    await clickTab(page, '默认 AI 服务'); await clickTab(page, '自定义供应商');
    assert.equal(await address.inputValue(), 'https://example.invalid/preserved');
    const limits = page.locator('summary').filter({hasText: '请求限制'}); await limits.click();
    await page.getByLabel('并发上限', {exact: true}).fill('3'); await limits.click();
    await clickTab(page, '高级设置');
    await page.locator('summary').filter({hasText: '自动预热与签到'}).click();
    const warmup = page.getByRole('region', {name: 'Codex 额度预热', exact: true});
    await warmup.getByRole('switch', {name: '每日定时预热'}).click(); await warmup.getByRole('button', {name: '添加时间'}).click();
    await clickTab(page, '隐私设置', 1); await clickTab(page, '额度设置', 1);
    assert.equal(await warmup.getByLabel(/预热时间 \d/).count(), 2);
    await page.getByRole('button', {name: '保存 AI 设置', exact: true}).click();
    assert.equal(await page.evaluate(() => window.settingsDesignFixture.state().ai.providers[0].maxConcurrency), 3);
    assert.deepEqual(await page.evaluate(() => window.settingsDesignFixture.state().ai.magpieSettings.codexWarmAts), ['06:00', '11:05']);
    interactions.push('AI provider draft / named advanced disclosure / all warm-up times / shared save');

    await open(page, 'capture', config, {failure: 'capture.snapshot'});
    await page.getByRole('alert').filter({hasText: '设置读取失败'}).waitFor();
    assert.equal(await page.getByRole('tab', {name: '截图后', exact: true}).isVisible(), true);
    await page.evaluate(() => window.settingsDesignFixture.recover());
    await page.getByRole('button', {name: '重试', exact: true}).click();
    await page.getByRole('radio', {name: '截图后编辑', exact: true}).waitFor();
    interactions.push('failed read / stable navigation / retry recovery');

    // Enlarge real text rather than merely scaling screenshot pixels. Each
    // choice must remain equal, named, selected and reachable in a narrow pane.
    for (const scale of [1.25, 1.5, 2]) {
      for (const width of [520, 1050]) for (const language of ['zh-CN', 'en-US']) for (const theme of ['light', 'dark']) {
        await page.setViewportSize({width, height: 940});
        await open(page, 'settings', {width, language, theme});
        await page.evaluate(value => {document.documentElement.style.fontSize = `${16 * value}px`;}, scale);
        await settle(page);
        await page.locator('[data-slot="page-scroll"]').evaluate(element => {element.scrollTop = element.scrollHeight;});
        await settle(page);
        results.push({page: 'settings-zoom', width, language, theme, scale, ...(await geometry(page))});
        if (scale === 2) await capture(page, `settings-zoom-${width}-${language}-${theme}`);
      }
    }
    await page.setViewportSize({width: 1050, height: 900});
    for (const scale of [1.25, 1.5, 2]) {
      await open(page, 'selection', config, {long: '1'});
      await page.evaluate(value => {document.documentElement.style.fontSize = `${16 * value}px`;}, scale);
      await page.getByRole('button', {name: '编辑 解释', exact: true}).click();
      await settle(page);
      const metrics = await geometry(page); results.push({page: 'selection-zoom', scale, ...metrics});
    }
    for (const id of ['capture', 'wintray', 'ai-services']) {
      await open(page, id, {...config, width: 520}, {long: '1'}); await page.setViewportSize({width: 520, height: 940}); await settle(page);
      results.push({page: id + '-long', ...(await geometry(page))});
    }
    // The untouched Chat fixture uses the same shell; settings tokens must not
    // reach its content, including after navigating back from a settings page.
    await page.goto('http://127.0.0.1:5198/tests/shell-preview.html');
    await page.getByRole('button', {name: 'Chat', exact: true}).click();
    assert.equal(await page.locator('[data-slot="page-content"].settings-surface').count(), 0);
    assert.equal(await page.locator('html[data-settings-ui]').count(), 0);
    assert.equal(await page.locator('[data-slot="page-content"]').getAttribute('data-settings-page'), null);
    interactions.push('Chat scope isolation');
    await page.close();
  }
} catch (error) {
  errors.push({error: String(error), stack: error.stack});
} finally {
  const failures = results.filter(result => result.overflow.length || result.clipped.length || result.hiddenPanels.length || result.badGroups.length || result.hierarchyFailures.length || result.navigationMoved);
  await writeFile(join(output, 'report.json'), JSON.stringify({quick, views: results.length, screenshots: screenshotIndex, interactions, results, errors}, null, 2));
  await browser.close(); await server.close();
  console.log(JSON.stringify({views: results.length, screenshots: screenshotIndex, interactions: interactions.length, layoutFailures: failures.length, errors: errors.length, output}));
  if (failures.length || errors.length) process.exitCode = 1;
}
