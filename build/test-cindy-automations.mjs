// Real browser check of Cindy's automation page inside WinOne (light/dark,
// wide/narrow, empty state and the new-automation form) with in-memory data.
// Usage: node build/test-cindy-automations.mjs <vite base url> <_temp output folder>
import {createRequire} from 'node:module';
import {mkdir, writeFile} from 'node:fs/promises';
import {resolve, join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';

const {chromium} = createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base, folder] = process.argv.slice(2), output = resolve(folder);
assert.equal(new URL(base).hostname, '127.0.0.1');
execFileSync('git', ['check-ignore', '--quiet', '--no-index', join(output, 'probe')]);
await mkdir(output, {recursive: true});
const browser = await chromium.launch({headless: true, channel: 'msedge'}), results = [];
try {
  for (const [width, height] of [[1000, 720], [520, 720]]) for (const theme of ['light', 'dark']) for (const empty of [false, true]) {
    const page = await browser.newPage({viewport: {width, height}, reducedMotion: 'reduce'}), errors = [];
    page.on('pageerror', (error) => errors.push(String(error)));
    page.on('response', (response) => { if (response.status() >= 400) errors.push(response.status() + ' ' + response.url()); });
    page.on('console', (message) => { if (message.type() === 'error' && !message.text().startsWith('Failed to load resource')) errors.push(message.text()); });
    await page.goto(`${base}/tests/cindy-automations-preview.html?theme=${theme}${empty ? '&empty' : ''}`);
    await page.waitForSelector('.cindy-chat-ui header', {timeout: 20000});
    await page.waitForTimeout(400);
    const name = `automations-${width}-${theme}${empty ? '-empty' : ''}`;
    const horizontal = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    await page.screenshot({path: join(output, name + '.png')});
    let form = null;
    if (!empty) {
      await page.getByRole('button', {name: /新建自动化|New automation/}).first().click();
      await page.waitForSelector('[role=dialog]', {timeout: 10000});
      await page.waitForTimeout(300);
      await page.screenshot({path: join(output, name + '-form.png')});
      form = await page.evaluate(() => {
        const dialog = document.querySelector('[role=dialog]');
        const rect = dialog.getBoundingClientRect();
        return {width: Math.round(rect.width), inside: rect.left >= 0 && rect.right <= window.innerWidth + 1, horizontal: [...dialog.querySelectorAll('*')].some((element) => element.scrollWidth > element.clientWidth + 1 && getComputedStyle(element).overflowX === 'scroll')};
      });
    }
    results.push({name, errors, horizontal, form});
    await page.close();
  }
} finally { await browser.close(); }
await writeFile(join(output, 'report.json'), JSON.stringify(results, null, 1));
console.log(JSON.stringify(results.map(({name, errors, horizontal, form}) => ({name, errors: errors.length, horizontal, form})), null, 0));
const failed = results.filter((result) => result.errors.length || result.horizontal || (result.form && (!result.form.inside || result.form.horizontal)));
if (failed.length) { console.error(JSON.stringify(failed, null, 1)); process.exitCode = 1; }
