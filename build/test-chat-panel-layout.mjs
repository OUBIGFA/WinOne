// Verify panel allocation and the composer boundary in the production React UI.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
if(new URL(base).hostname!=='127.0.0.1')throw Error('Use the isolated preview');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})}),report=[];
try{
 for(const width of [720,1280])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'});
  await page.goto(`${base}/tests/chat-experience-preview.html?theme=${theme}&language=en-US`);await page.locator('.chat-composer').waitFor();await page.getByRole('button',{name:'Show chat details',exact:true}).click();await page.locator('.chat-side-panel').waitFor();
  const geometry=await page.evaluate(()=>{
   const panel=document.querySelector('.chat-side-panel'),allocation=document.getElementById('details'),main=document.querySelector('.chat-main'),footer=document.querySelector('.chat-input-area'),tabs=panel.querySelector('.chat-pane-tabstrip');const box=node=>node.getBoundingClientRect().toJSON();
   return {panel:box(panel),allocation:box(allocation),main:box(main),footer:box(footer),panelPadding:getComputedStyle(panel).padding,mainOverflow:getComputedStyle(main).overflowX,tabScrollbar:getComputedStyle(tabs).scrollbarWidth};
  });
  if(Math.abs(geometry.panel.width-geometry.allocation.width)>1||Math.abs(geometry.panel.right-geometry.allocation.right)>1||geometry.panelPadding!=='0px')throw Error('Side panel does not fill its allocation: '+JSON.stringify(geometry));
  if(!['clip','hidden'].includes(geometry.mainOverflow)||geometry.footer.left<geometry.main.left||geometry.footer.right>geometry.main.right+1)throw Error('Composer gradient is not isolated to the conversation');
  if(geometry.tabScrollbar!=='none')throw Error('Panel tabs expose a horizontal scrollbar');
  await page.getByRole('button',{name:'Tab actions',exact:true}).click();await page.getByRole('menuitem',{name:'Move to bottom',exact:true}).click();
  const dock=await page.locator('.chat-side-panel').boundingBox();const allocation=await page.locator('#details').boundingBox();if(Math.abs(dock.width-allocation.width)>1||Math.abs(dock.height-allocation.height)>1)throw Error('Bottom dock does not fill its panel');
  await page.screenshot({path:join(output,`panels-${width}-${theme}.png`)});
  await page.goto(`${base}/tests/chat-experience-preview.html?welcome&theme=${theme}&language=en-US`);await page.locator('.chat-welcome-logo').waitFor();
  const welcome=await page.evaluate(()=>{const logo=document.querySelector('.chat-welcome-logo').getBoundingClientRect(),composer=document.querySelector('.chat-welcome-composer').getBoundingClientRect();return {gap:composer.top-logo.bottom,overflow:document.documentElement.scrollWidth-innerWidth};});
  if(welcome.gap<16||welcome.overflow>1)throw Error('Welcome spacing is too small or overflows: '+JSON.stringify(welcome));
  await page.screenshot({path:join(output,`welcome-${width}-${theme}.png`)});report.push({width,theme,geometry,welcome});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(report,null,2));console.log(JSON.stringify({cases:report.length,passed:true}));
}finally{await browser.close();}
