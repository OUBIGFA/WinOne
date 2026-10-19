// Real Cindy image and document widgets; native clipboard/save requests are
// recorded by the renderer fixture. Native bitmap formats have Rust tests.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);assert.equal(new URL(base).hostname,'127.0.0.1');execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const labels=JSON.parse(await readFile('apps/desktop/frontend/src/features/cindy/panel-ui/en.json','utf8')),media=labels.chat.media;
const policy=(await readFile('apps/desktop/frontend/index.html','utf8')).match(/content="(default-src[^"]+)"/)[1];
const png=await readFile('internal/branding/logo.png'),files=new Map(),uploads=[];
const document='# Image and diagram\n\n![Local picture](picture.png)\n\n```mermaid\nflowchart LR\n  A[Read] --> B[Edit]\n```\n\nReady.\n';
let origin='';
const server=createServer(async(req,res)=>{const url=new URL(req.url,'http://localhost');res.setHeader('Access-Control-Allow-Origin','*');res.setHeader('Cache-Control','no-store');if(req.method==='OPTIONS'){res.setHeader('Access-Control-Allow-Methods','POST');res.setHeader('Access-Control-Allow-Headers','Content-Type');res.writeHead(204);res.end();return;}
 if(url.pathname==='/image-upload'){const chunks=[];for await(const chunk of req)chunks.push(chunk);const data=Buffer.concat(chunks),name=`media-${uploads.length+1}.png`,path='C:/fixture/Alpha/'+name;files.set('/'+name,data);uploads.push({name,bytes:data.length});res.setHeader('Content-Type','application/json');res.end(JSON.stringify({attachment:{path,name,size:data.length,mimeType:'image/png'},preview:{path,name,size:data.length,mimeType:'image/png',kind:'image',modified:1700000000000,revision:name,url:origin+'/'+name}}));return;}
 if(url.pathname.endsWith('.png')){res.setHeader('Content-Type','image/png');res.end(files.get(url.pathname)??png);}else{res.setHeader('Content-Type','text/plain; charset=utf-8');res.end(document);}});
await new Promise(done=>server.listen(0,'127.0.0.1',done));origin=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];let page;
try{for(const theme of ['light','dark']){
 page=await browser.newPage({viewport:{width:1320,height:920},reducedMotion:'reduce'});const errors=[];page.on('pageerror',error=>errors.push(error.message));
 await page.goto(`${base}/tests/chat-experience-preview.html?alignment=1&language=en-US&theme=${theme}&previewOrigin=${encodeURIComponent(origin)}`);await page.getByRole('region',{name:'Files and changes'}).waitFor();
 for(let attempt=0;;attempt++){try{await page.evaluate(async()=>{await Promise.all([import('/src/features/chat-file-preview.tsx'),import('/src/features/cindy/media/ImageLightbox.tsx')]);await document.fonts.ready;});await page.waitForLoadState('networkidle');break;}catch(error){if(attempt>=2||!/Execution context was destroyed/.test(String(error)))throw error;await page.waitForLoadState('domcontentloaded');}}
 await page.evaluate(policy=>{window.mediaPolicyViolations=[];document.addEventListener('securitypolicyviolation',event=>window.mediaPolicyViolations.push({directive:event.effectiveDirective,blocked:event.blockedURI,source:event.sourceFile,line:event.lineNumber}));const meta=document.createElement('meta');meta.httpEquiv='Content-Security-Policy';meta.content=policy;document.head.append(meta);},policy);
 await page.getByRole('region',{name:'Files and changes'}).locator('.chat-generated-files').getByRole('button',{name:'README.md',exact:true}).click();
 const panel=page.locator('.chat-side-panel'),editor=panel.locator('.cindy-file-editor');await editor.locator('.cm-md-image-widget img').waitFor();await editor.locator('.cm-md-image-widget img').click();
 const viewer=page.getByRole('dialog',{name:media.imagePreview});await viewer.waitFor();const image=viewer.locator('img').first();await image.waitFor();await image.evaluate(img=>img.decode());
 const background=await viewer.evaluate(el=>getComputedStyle(el).backgroundColor);assert.notEqual(background,'rgba(0, 0, 0, 0)');
 await image.dblclick();await page.keyboard.press('0');
 await viewer.getByRole('button',{name:media.annotate,exact:true}).click();
 const bounds=await image.boundingBox();assert(bounds);await page.mouse.move(bounds.x+bounds.width*.25,bounds.y+bounds.height*.35);await page.mouse.down();await page.mouse.move(bounds.x+bounds.width*.7,bounds.y+bounds.height*.6,{steps:14});await page.mouse.up();
 assert(await viewer.locator('svg path').count()>0);await viewer.getByRole('button',{name:media.annotateUndo,exact:true}).click();await viewer.getByRole('button',{name:media.annotateRedo,exact:true}).click();
 await page.screenshot({path:join(output,`image-annotate-${theme}.png`)});
 await viewer.getByRole('button',{name:media.sendToChat,exact:true}).click();await viewer.waitFor({state:'hidden'});
 await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='draft.save'&&call.params.draft.attachments?.some(file=>file.imageAnnotation?.strokes?.length)));
 const saved=await page.evaluate(()=>window.chatExperienceCalls.filter(call=>call.method==='draft.save'&&call.params.draft.attachments?.some(file=>file.imageAnnotation?.strokes?.length)).at(-1).params.draft.attachments.find(file=>file.imageAnnotation));
 assert(saved.imageAnnotation.sourcePath!==saved.path,'Annotation replaced its original');
 assert(files.get('/'+saved.name)?.length>0);assert.notDeepEqual(files.get('/'+saved.name),png,'Annotation did not change exported pixels');
 const edit=page.getByRole('button',{name:media.annotate+' · '+saved.name,exact:true});await edit.click();await viewer.waitFor();await viewer.getByRole('button',{name:media.annotateUndo,exact:true}).click();await viewer.getByRole('button',{name:media.annotateSave,exact:true}).click();await viewer.waitFor({state:'hidden'});
 await editor.locator('.cm-md-image-widget img').click();await viewer.waitFor();await viewer.getByRole('button',{name:media.copyImage,exact:true}).click();await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='media.native.copy'));
 await viewer.getByRole('button',{name:media.saveAs,exact:true}).click();await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='media.native.save-as'));
 await page.keyboard.press('Escape');await viewer.waitFor({state:'hidden'});
 const mermaid=editor.locator('.cm-md-mermaid-card > svg');await mermaid.waitFor();assert(await mermaid.locator('path').count()>0);await mermaid.click();const diagram=page.locator('[data-mermaid-lightbox-overlay]');await diagram.waitFor();assert.notEqual(await diagram.evaluate(el=>getComputedStyle(el).backgroundColor),'rgba(0, 0, 0, 0)');await page.screenshot({path:join(output,`diagram-${theme}.png`)});await page.keyboard.press('Escape');await diagram.waitFor({state:'hidden'});
 const violations=await page.evaluate(()=>window.mediaPolicyViolations);assert.deepEqual(violations,[]);assert.deepEqual(errors,[]);results.push({theme,background,sourceRetained:true,nativeCopy:true,nativeSave:true,svg:true,violations});await page.close();page=null;
 }await writeFile(join(output,'report.json'),JSON.stringify({results,uploads},null,2));console.log(`Cindy media: ${results.length} image/annotation/draft/copy/save/SVG cases passed.`);
}catch(error){if(page&&!page.isClosed()){await page.screenshot({path:join(output,'failure.png')});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText());await writeFile(join(output,'failure-policy.json'),JSON.stringify(await page.evaluate(()=>window.mediaPolicyViolations??null),null,2));}throw error;}
finally{await browser.close();server.closeAllConnections();await new Promise(done=>server.close(done));}
