import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/chat-project-layout');await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM}),report=[],errors=[];
try{
 const page=await browser.newPage();page.on('pageerror',error=>errors.push(String(error)));
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1000]){
  const zh=language==='zh-CN';await page.setViewportSize({width,height:860});
  const url=new URL(process.env.WINONE_LAYOUT_URL||'http://127.0.0.1:5192/tests/chat-project-preview.html');url.searchParams.set('language',language);url.searchParams.set('theme',theme);await page.goto(url.href);
  await page.getByRole('button',{name:'Alpha Project',exact:true}).waitFor();
  if(width===520)await page.getByRole('button',{name:zh?'收起会话列表':'Hide chat list',exact:true}).click();
  const context=page.locator('.chat-project-context');await context.getByRole('button').first().click();await page.getByRole('menuitem',{name:/Alpha Project/}).click();
  await page.getByText('feature/alpha',{exact:true}).waitFor();await page.locator('[role=menu]').waitFor({state:'detached'});
  const composer=page.getByRole('textbox',{name:zh?'消息':'Message',exact:true});await composer.fill('Preserved draft');
  const bounds=await context.boundingBox(),input=await composer.boundingBox();if(bounds.y+bounds.height>input.y)throw Error('Project context is not above composer');
  const clipped=await context.evaluate(root=>[...root.querySelectorAll('button,[role=status]')].filter(element=>{const r=element.getBoundingClientRect();return r.width>0&&(r.left<0||r.right>innerWidth+1)}).map(element=>element.textContent));if(clipped.length)throw Error('Clipped project context: '+clipped.join(','));
  await page.screenshot({path:resolve(output,`${language}-${theme}-${width}.png`)});
  await context.getByRole('button').first().click();await page.getByRole('menuitem',{name:zh?'打开或新建项目':'Open or create project',exact:true}).click();
  const dialog=page.getByRole('dialog');await dialog.getByRole('tab',{name:zh?'新建项目':'New project',exact:true}).click();
  await dialog.getByRole('textbox',{name:zh?'项目名称':'Project name',exact:true}).fill('Created');await dialog.getByRole('button',{name:zh?'选择位置':'Choose location',exact:true}).click();
  await page.screenshot({path:resolve(output,`${language}-${theme}-${width}-dialog.png`)});
  await dialog.getByRole('button',{name:zh?'创建':'Create',exact:true}).click();await context.getByRole('button',{name:zh?'项目：Created':'Project: Created',exact:true}).waitFor();
  if(await composer.inputValue()!=='Preserved draft')throw Error('Project creation lost the draft');report.push({language,theme,width,clipped,aboveComposer:true,draftPreserved:true});
 }
 await writeFile(resolve(output,'report.json'),JSON.stringify({report,errors},null,2));if(errors.length)throw Error(errors.join('\n'));console.log('PASS: project context, create dialog, branch and draft at 520/1000 px, zh/en and light/dark');
}finally{await browser.close();}
