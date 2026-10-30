// Real browser regression for every manual settings action rail and disclosure.
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
const pages = ['settings', 'capture', 'selection', 'quick-assistant', 'convenience', 'wintray', 'ai-services', 'interconnect'];
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
      const footer = shell.querySelector('[data-slot="settings-actions"]');
      const footerRect = footer?.getBoundingClientRect();
      const navigation = footer?.querySelector('[data-slot="page-scroll-controls"]');
      const navigationButtons = navigation ? [...navigation.querySelectorAll('button')].map(node => node.getBoundingClientRect()) : [];
      const bars = [...shell.querySelectorAll('.settings-save-bar')].filter(visible).map(bar => {
        const rect = bar.getBoundingClientRect();
        const buttons = [...bar.querySelectorAll('button')].filter(visible).map(button => {
          const bounds = button.getBoundingClientRect();
          const hit = document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
          return {name: button.textContent.trim(), visible: bounds.top >= clip.top - 1 && bounds.bottom <= clip.bottom + 1,
            left: bounds.left, top: bounds.top, bottom: bounds.bottom, height: bounds.height, disabled: button.matches(':disabled'),
            unobscured: !!hit && (button.contains(hit) || hit === bar || bar.contains(hit))};
        });
        const style = getComputedStyle(bar);
        // Save is first and Undo directly below it, one 8px control gap apart.
        const [first, second] = buttons;
        return {top: rect.top, bottom: rect.bottom, height: rect.height, placeholder: bar.classList.contains('settings-save-placeholder'),
          undoBelow: !!first && !!second && Math.abs(second.left - first.left) <= 1 && Math.abs(second.top - first.bottom - 8) <= 1.5,
          paddingTop: parseFloat(style.paddingTop), paddingBottom: parseFloat(style.paddingBottom), background: style.backgroundColor,
          inFooter: !!bar.closest('[data-slot="settings-actions"]'),
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
        railSafe: !footerRect?.height || clip.right-footerRect.right>=11.5&&clip.right-footerRect.right<=40&&footerRect.width<=40&&footerRect.bottom<=clip.bottom-15.5&&footerRect.top>=clip.top,
        noReservedWidth: parseFloat(getComputedStyle(root).paddingRight)===0&&parseFloat(getComputedStyle(root).paddingLeft)===0,
        oldBar: !!shell.querySelector('[data-slot="settings-footer"]'),
        navigationInColumn: navigationButtons.length !== 2 || Math.abs(navigationButtons[0].left - navigationButtons[1].left) <= 1 && navigationButtons[0].bottom + 7.5 <= navigationButtons[1].top,
        saveNavigationGap: bars.length&&navigationButtons.length?navigationButtons[0].top-bars.at(-1).bottom:null,
        overflow: root.scrollWidth > root.clientWidth + 1 || !!footer && footer.scrollWidth > footer.clientWidth + 1,
        scrollTop: root.scrollTop, height: root.clientHeight, total: root.scrollHeight};
    });
    const failures = [
      ...(metrics.overflow ? ['horizontal overflow'] : []),
      ...(!metrics.railSafe ? ['action rail lacks safe edge insets'] : []),
      ...(!metrics.noReservedWidth ? ['action rail reserves a content lane'] : []),
      ...(metrics.oldBar ? ['full-width save bar remains'] : []),
      ...(!metrics.navigationInColumn ? ['scroll navigation is not vertical'] : []),
      ...(metrics.saveNavigationGap!==null&&metrics.saveNavigationGap<43.5 ? ['save/navigation gap is less than one extra button'] : []),
      ...(metrics.bars.length>1 ? ['duplicate global save buttons'] : []),
      ...(metrics.bars.length===0 ? ['save/undo missing from action rail'] : []),
      ...metrics.unanchored.map(name => `save outside footer: ${name}`),
      ...metrics.bars.flatMap(bar => [
        ...(!bar.visible ? ['footer outside viewport'] : []),
        ...(!bar.inFooter ? ['save is not in shared footer'] : []),
        ...(bar.buttons.some(button => button.height > 37) ? ['save button is not compact'] : []),
        ...(!bar.undoBelow ? ['undo is not directly below save'] : []),
        ...(bar.placeholder && bar.buttons.some(button => !button.disabled) ? ['placeholder save/undo is enabled'] : []),
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
    // Visiting never edits, so no unsaved-change prompt may stay open. A
    // brief "operation in progress" prompt closes when loading finishes.
    await page.waitForFunction(() => !document.querySelector('[data-slot="dialog-content"]'), null, {timeout: 5000});
    await visitTabs(page, id, config, depth + 1, [...path, name]);
  }
}
// Change only the fixture's scrollability, not the frame size or save form.
// This exercises the actual PageScrollControls observer and overlay hit testing.
async function verifyStationarySave(page,config){
 await page.goto(`http://127.0.0.1:5199/tests/settings-design-preview.html?${new URLSearchParams({...config,page:'capture',long:'1'})}`);
 const save=page.locator('.settings-save-bar:not([hidden]):not(.settings-save-placeholder) .settings-global-save');await save.waitFor();
 const undo=page.locator('.settings-save-bar:not([hidden]):not(.settings-save-placeholder) .settings-global-undo');
 const navigation=page.locator('[data-slot="page-scroll-controls"]');await navigation.waitFor();
 const before=await save.boundingBox();
 assert.equal(await undo.isDisabled(),true,'Undo is enabled without a draft');
 const layoutUnchanged=await page.locator('[data-slot="settings-actions"]').evaluate(rail=>{
  const root=rail.parentElement.querySelector('[data-slot="page-scroll"]');
  const measure=()=>[root.getBoundingClientRect().toJSON(),root.querySelector('[data-slot="page-content"]').getBoundingClientRect().toJSON()];
  const before=measure();rail.style.display='none';const hidden=measure();rail.style.removeProperty('display');
  return JSON.stringify(before)===JSON.stringify(hidden);
 });
 assert.equal(layoutUnchanged,true,'Overlay actions reserve layout space');
 await page.locator('[data-slot="page-scroll"]').evaluate(root=>{
  for(const node of [root,...root.querySelectorAll('[data-page-scroll-target]')]){node.dataset.priorOverflow=node.style.overflowY;node.style.overflowY='hidden';}
 });
 await navigation.waitFor({state:'hidden'});await settle(page);
 const hidden=await save.boundingBox();
 assert.deepEqual(hidden,before,'Save moved when scroll navigation disappeared');
 await page.locator('[data-slot="page-scroll"]').evaluate(root=>{
  for(const node of [root,...root.querySelectorAll('[data-page-scroll-target]')]){node.style.overflowY=node.dataset.priorOverflow??'';delete node.dataset.priorOverflow;}
 });
 await navigation.waitFor();await settle(page);
 const restored=await save.boundingBox();assert.deepEqual(restored,before,'Save moved when scroll navigation returned');
 const gapsReceiveInput=await page.locator('[data-slot="settings-actions"]').evaluate(rail=>{
  const box=rail.getBoundingClientRect();
  // Save 0-36, Undo 44-80, navigation 124-160 and 168-204: probe every gap.
  return [40,102,164].some(offset=>{const hit=document.elementFromPoint(box.x+box.width/2,box.y+offset);return hit===rail||rail.contains(hit);});
 });
 assert.equal(gapsReceiveInput,false,'Empty action-rail gaps block page content');
 cases.push({page:'capture',...config,scenario:'stationary save / navigation toggle / no layout reservation / click-through gaps',before,hidden,restored,failures:[]});
 // The action rail is above content, never above unsaved-change protection.
 const english=config.language==='en-US';
 const captureTabs=page.getByRole('tablist').first().getByRole('tab');
 await captureTabs.nth(1).click();
 const template=page.locator('#capture-file-name');
 await template.fill('overlay-draft-{timestamp}');
 await page.locator('.app-navigation [data-page-id="overview"]').click();
 await page.getByRole('dialog').waitFor();
 const saveBox=await save.boundingBox();
 const saveCalls=()=>page.evaluate(()=>window.settingsDesignFixture.calls.filter(call=>call.method==='capture.settings.save').length);
 const callsBefore=await saveCalls();
 const protectedByDialog=await save.evaluate(button=>{const box=button.getBoundingClientRect(),hit=document.elementFromPoint(box.x+box.width/2,box.y+box.height/2);return !button.contains(hit);});
 assert.equal(protectedByDialog,true,'Action rail sits above the unsaved-change dialog');
 await page.mouse.click(saveBox.x+saveBox.width/2,saveBox.y+saveBox.height/2);
 await page.getByRole('dialog').waitFor({state:'hidden'});
 assert.equal(await saveCalls(),callsBefore,'Backdrop click saved through the dialog');
 assert.equal(await template.inputValue(),'overlay-draft-{timestamp}','Backdrop click lost the draft');
 cases.push({page:'capture',...config,scenario:'modal protects save / outside click retains draft',failures:[]});
 // Undo sits directly below Save and reverts the same draft Save would write.
 assert.equal(await undo.isDisabled(),false,'Undo is disabled with a draft');
 await undo.click();
 assert.equal(await template.inputValue(),'','Undo did not restore the saved value');
 assert.equal(await undo.isDisabled(),true,'Undo stays enabled after reverting');
 assert.equal(await save.isDisabled(),true,'Save stays enabled after reverting');
 cases.push({page:'capture',...config,scenario:'rail undo reverts the draft',failures:[]});
 // A tab switch asks exactly like leaving the page: keep editing stays put,
 // discarding reverts the shared draft and then switches.
 await template.fill('tab-draft-{timestamp}');
 await captureTabs.nth(0).click();
 const dialog=page.getByRole('dialog');await dialog.waitFor();
 assert.match(await dialog.textContent(),english?/Switching tabs will discard/:/切换标签会放弃/);
 await dialog.getByRole('button',{name:english?'Keep editing':'继续编辑',exact:true}).click();
 await dialog.waitFor({state:'hidden'});
 assert.equal(await captureTabs.nth(1).getAttribute('aria-selected'),'true','Keep editing switched the tab');
 assert.equal(await template.inputValue(),'tab-draft-{timestamp}','Keep editing lost the draft');
 await captureTabs.nth(0).click();await dialog.waitFor();
 await dialog.getByRole('button',{name:english?'Discard and leave':'放弃并离开',exact:true}).click();
 await dialog.waitFor({state:'hidden'});
 assert.equal(await captureTabs.nth(0).getAttribute('aria-selected'),'true','Discard did not switch the tab');
 await captureTabs.nth(1).click();
 assert.equal(await dialog.count(),0,'A clean tab switch still asks');
 assert.equal(await template.inputValue(),'','Discard kept the draft');
 cases.push({page:'capture',...config,scenario:'tab switch asks / keep editing / discard reverts',failures:[]});
 // Pages without a manual save show the same controls disabled, in place.
 for(const id of ['settings','overview']){
  await page.goto(`http://127.0.0.1:5199/tests/settings-design-preview.html?${new URLSearchParams({...config,page:id,long:'1'})}`);
  const placeholder=page.locator('.settings-save-placeholder');await placeholder.waitFor();
  const box=await placeholder.locator('button').first().boundingBox();
  assert.deepEqual(box,before,`${id}: disabled Save is not where page saves appear`);
  assert.equal(await placeholder.locator('button:disabled').count(),2,`${id}: placeholder Save/Undo are not both disabled`);
 }
 cases.push({page:'settings/overview',...config,scenario:'disabled save and undo in the same place',failures:[]});
}
// Every navigation tab uses the same decision, not only one page's tabs.
async function verifyConvenienceGuard(page,config){
 const english=config.language==='en-US';
 await page.goto(`http://127.0.0.1:5199/tests/settings-design-preview.html?${new URLSearchParams({...config,page:'convenience'})}`);
 const tabs=page.getByRole('tablist').first();
 const links=tabs.getByRole('tab',{name:english?'File and folder links':'文件与目录链接',exact:true});
 const gestures=tabs.getByRole('tab',{name:english?'Mouse gestures':'鼠标手势',exact:true});
 await links.click();
 await page.waitForFunction(()=>!document.querySelector('[data-slot="dialog-content"]'),null,{timeout:5000});
 // The same native drop event that Explorer integration sends selects a source.
 await page.evaluate(()=>window.dispatchEvent(new CustomEvent('winone:directory-links-drop',{detail:{paths:['C:\\WinOne-test\\source\\document-1.txt']}})));
 await page.getByText(english?'1 entries selected':'已选择 1 项',{exact:true}).waitFor();
 await gestures.click();
 const dialog=page.getByRole('dialog');await dialog.waitFor();
 await dialog.getByRole('button',{name:english?'Keep editing':'继续编辑',exact:true}).click();await dialog.waitFor({state:'hidden'});
 assert.equal(await links.getAttribute('aria-selected'),'true','Keep editing left the links tab');
 await page.getByText(english?'1 entries selected':'已选择 1 项',{exact:true}).waitFor();
 await gestures.click();await dialog.waitFor();
 await dialog.getByRole('button',{name:english?'Discard and leave':'放弃并离开',exact:true}).click();await dialog.waitFor({state:'hidden'});
 assert.equal(await gestures.getAttribute('aria-selected'),'true','Discard did not open mouse gestures');
 await links.click();
 await page.getByText(english?'0 entries selected':'已选择 0 项',{exact:true}).waitFor();
 assert.equal(await dialog.count(),0,'Returning to a discarded tab asked again');
 cases.push({page:'convenience',...config,scenario:'links draft / switch to mouse gestures asks first',failures:[]});
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
    await verifyStationarySave(page,config);
    await verifyConvenienceGuard(page,config);
    console.log(`Action-rail/disclosure matrix: ${config.width} ${config.language} ${config.theme}`);
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
