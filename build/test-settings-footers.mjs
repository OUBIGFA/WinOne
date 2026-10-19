// Real browser regression for every manual settings footer and HTML disclosure.
// Uses the production App with its existing in-memory Host fixture; no native
// settings, hooks, accounts or files are changed. Run from the repository root.
import {createRequire} from 'node:module';
import {mkdir, writeFile} from 'node:fs/promises';
import {resolve, join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';

const frontend = resolve('apps/desktop/frontend');
const output = resolve(process.env.WINONE_LAYOUT_OUTPUT || '_temp/settings-footers');
execFileSync('git', ['check-ignore', '--quiet', '--no-index', join(output, 'probe')]);
await mkdir(join(output, 'browser-temp'), {recursive: true});
process.env.TEMP = process.env.TMP = join(output, 'browser-temp');
const {chromium} = createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE || resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer} = await import(pathToFileURL(join(frontend, 'node_modules/vite/dist/node/index.js')));
const server = await createServer({root: frontend, configFile: join(frontend, 'vite.config.ts'), server: {host: '127.0.0.1', port: 5199, strictPort: true}});
await server.listen();
const browser = await chromium.launch({headless: true, ...(process.env.WINONE_CHROMIUM ? {executablePath: process.env.WINONE_CHROMIUM} : {channel: 'msedge'})});
const cases = [], errors = [];
const quick = process.env.WINONE_SETTINGS_QUICK === '1';
const matrix = quick ? [{width: 900, language: 'zh-CN', theme: 'light'}]
  : [520, 900, 1280].flatMap(width => ['zh-CN', 'en-US'].flatMap(language => ['light', 'dark'].map(theme => ({width, language, theme}))));
const pages = ['settings', 'capture', 'selection', 'quick-assistant', 'convenience', 'wintray', 'ai-services'];
async function settle(page) {
  await page.evaluate(async () => {await document.fonts.ready; await new Promise(requestAnimationFrame); await new Promise(requestAnimationFrame);});
}
async function inspect(page, id, config, path) {
  const viewport = page.locator('[data-slot="page-scroll"]');
  const dimensions = await viewport.evaluate(node => ({height: node.clientHeight, total: node.scrollHeight}));
  const positions = [...new Set([0, Math.round((dimensions.total - dimensions.height) / 2), dimensions.total - dimensions.height])];
  for (const scrollTop of positions) {
    await viewport.evaluate((node, top) => {node.scrollTop = top;}, scrollTop);
    await settle(page);
    const metrics = await viewport.evaluate(root => {
      const visible = node => !!node.getClientRects().length && !node.closest('[hidden],[aria-hidden="true"]');
      const shell = root.parentElement;
      const clip = shell.getBoundingClientRect();
      const footer = shell.querySelector('[data-slot="settings-footer"]');
      const footerRect = footer?.getBoundingClientRect();
      const navigation = footer?.querySelector('[data-slot="page-scroll-controls"]');
      const navigationButtons = navigation ? [...navigation.querySelectorAll('button')].map(node => node.getBoundingClientRect()) : [];
      const bars = [...shell.querySelectorAll('.settings-save-bar')].filter(visible).map(bar => {
        const rect = bar.getBoundingClientRect();
        const buttons = [...bar.querySelectorAll('button')].filter(visible).map(button => {
          const bounds = button.getBoundingClientRect();
          const hit = document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
          return {name: button.textContent.trim(), visible: bounds.top >= clip.top - 1 && bounds.bottom <= clip.bottom + 1,
            unobscured: !!hit && (button.contains(hit) || hit === bar || bar.contains(hit))};
        });
        const style = getComputedStyle(bar);
        return {top: rect.top, bottom: rect.bottom, height: rect.height,
          paddingTop: parseFloat(style.paddingTop), paddingBottom: parseFloat(style.paddingBottom), background: style.backgroundColor,
          inFooter: !!bar.closest('[data-slot="settings-footer"]'),
          visible: rect.top >= clip.top - 1 && rect.bottom <= clip.bottom + 1, buttons};
      });
      // Detect an overlooked manual save even when its old container was not
      // assigned the shared footer class. Automatic preferences have no Save.
      const unanchored = [...shell.querySelectorAll('button')].filter(visible).filter(button =>
        /^(保存(设置|快捷键)?|Save( settings| shortcuts)?)$/i.test(button.textContent.trim()) && !button.closest('.settings-save-bar'))
        .map(button => button.textContent.trim());
      const disclosures = [...root.querySelectorAll('details > summary')].filter(visible).map(summary => {
        const pseudo = getComputedStyle(summary, '::before');
        const explicit = !!summary.querySelector('.lucide-chevron-down,.lucide-chevron-right,.shortcut-summary-chevron');
        return {title: summary.textContent.trim(), listStyle: getComputedStyle(summary).listStyleType,
          explicit, fallback: pseudo.content !== 'none' && pseudo.content !== 'normal', mask: pseudo.maskImage};
      });
      return {bars, unanchored, disclosures,
        footerAtBottom: !footerRect?.height || Math.abs(footerRect.bottom - clip.bottom) <= 1,
        footerPadding: footer ? parseFloat(getComputedStyle(footer).paddingTop) : 0,
        navigationInRow: navigationButtons.length !== 2 || Math.abs(navigationButtons[0].top - navigationButtons[1].top) <= 1 && navigationButtons[0].right <= navigationButtons[1].left,
        overflow: root.scrollWidth > root.clientWidth + 1 || !!footer && footer.scrollWidth > footer.clientWidth + 1,
        scrollTop: root.scrollTop, height: root.clientHeight, total: root.scrollHeight};
    });
    const failures = [
      ...(metrics.overflow ? ['horizontal overflow'] : []),
      ...(!metrics.footerAtBottom ? ['footer not at window bottom'] : []),
      ...(metrics.footerPadding > 6.1 ? ['footer has excessive vertical padding'] : []),
      ...(!metrics.navigationInRow ? ['scroll navigation is not horizontal'] : []),
      ...metrics.unanchored.map(name => `save outside footer: ${name}`),
      ...metrics.bars.flatMap(bar => [
        ...(!bar.visible ? ['footer outside viewport'] : []),
        ...(!bar.inFooter ? ['save is not in shared footer'] : []),
        ...(bar.background === 'rgba(0, 0, 0, 0)' ? ['transparent footer'] : []),
        ...(bar.paddingTop > 6.1 || bar.paddingBottom > 6.1 ? ['save footer has excessive vertical padding'] : []),
        ...bar.buttons.filter(button => !button.visible || !button.unobscured).map(button => `covered save action: ${button.name}`),
      ]),
      ...metrics.disclosures.filter(item => item.listStyle !== 'none' || item.explicit === item.fallback || !item.explicit && !item.mask.includes('chevron')).map(item => `missing/duplicate disclosure: ${item.title}`),
    ];
    cases.push({page: id, ...config, path, ...metrics, failures});
    if (failures.length && cases.filter(item => item.failures.length).length <= 12) {
      await page.mouse.move(0, 0);
      await page.screenshot({path: join(output, `failure-${cases.length}.png`)});
    }
  }
  await viewport.evaluate(node => {node.scrollTop = 0;});
}
async function visitTabs(page, id, config, depth = 0, path = []) {
  const list = page.getByRole('tablist').nth(depth);
  if (!await list.count() || depth > 2) {await inspect(page, id, config, path); return;}
  const names = (await list.getByRole('tab').allTextContents()).map(name => name.trim());
  for (const name of names) {
    const tab = page.getByRole('tablist').nth(depth).getByRole('tab', {name, exact: true});
    if (await tab.isDisabled()) continue;
    await tab.click(); await settle(page);
    await visitTabs(page, id, config, depth + 1, [...path, name]);
  }
}
try {
  for (const config of matrix) {
    const page = await browser.newPage({viewport: {width: config.width, height: 720}, reducedMotion: 'reduce', colorScheme: config.theme});
    page.on('pageerror', error => errors.push({url: page.url(), error: String(error)}));
    for (const id of pages) {
      await page.goto(`http://127.0.0.1:5199/tests/settings-design-preview.html?${new URLSearchParams({...config, page: id, long: '1'})}`);
      await page.locator(`[data-slot="page-scroll"][data-page="${id}"]`).waitFor();
      await settle(page);
      await visitTabs(page, id, config);
    }
    console.log(`Footer/disclosure matrix: ${config.width} ${config.language} ${config.theme}`);
    await page.close();
  }
  const failures = cases.filter(item => item.failures.length);
  await writeFile(join(output, 'report.json'), JSON.stringify({cases, errors}, null, 2));
  console.log(JSON.stringify({cases: cases.length, failures: failures.length, errors, samples: failures.slice(0, 6)}, null, 2));
  assert.equal(errors.length, 0, 'No browser runtime errors');
  assert.equal(failures.length, 0, 'All manual saves remain visible and disclosures use one outline chevron');
} finally {
  await browser.close(); await server.close();
}
