// Headless renderer check. Every request is intercepted from verified local
// files; this opens no listener and never reaches an external service.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE);
const directory=path.resolve(process.argv[2]);
const assets=path.resolve(process.argv[3]);
const fixtures=JSON.parse(await fs.readFile(path.join(directory,'fixtures.json'),'utf8'));
const browser=await chromium.launch({executablePath:process.env.WINONE_CHROMIUM,headless:true,args:['--disable-background-networking','--disable-component-update','--disable-default-apps','--no-first-run','--mute-audio']});
const failures=[];
try{
 for(const fixture of fixtures){
  for(const width of [420,800]){for(const dark of [false,true]){
   const page=await browser.newPage({viewport:{width,height:600},serviceWorkers:'block'});
   page.on('pageerror',error=>failures.push(`${fixture.name}: ${error.message}`));
   page.on('console',message=>{if(message.type()==='error')failures.push(`${fixture.name}: ${message.text()}`)});
   await page.route('**/*',async route=>{
    const url=new URL(route.request().url());
    if(url.hostname!=='anki.invalid'){failures.push(`External request blocked: ${url.origin}`);return route.abort();}
    const relative=decodeURIComponent(url.pathname).replace(/^\//,'');
    const base=relative.startsWith('_anki/')?assets:fixture.media;
    const name=relative.startsWith('_anki/')?relative.slice(6):relative;
    const file=relative==='index.html'?path.join(directory,fixture.name+(dark?'-dark':'')+'.html'):path.resolve(base,name);
    if(relative!=='index.html'&&!file.startsWith(path.resolve(base)+path.sep)){failures.push('Path escape');return route.abort();}
    try{const data=await fs.readFile(file);const ext=path.extname(file);const contentType={'.js':'text/javascript','.css':'text/css','.html':'text/html','.png':'image/png','.woff':'font/woff','.json':'application/json'}[ext]||'application/octet-stream';await route.fulfill({body:data,contentType});}
    catch(error){failures.push(`${fixture.name}: missing ${relative}`);await route.abort();}
   });
   await page.goto('https://anki.invalid/index.html');
   try{await page.waitForFunction(()=>document.querySelector('#qa')?.style.opacity==='1',null,{timeout:10000});}
   catch(error){await fs.writeFile(path.join(directory,fixture.name+'-failed.html'),await page.content());throw Error(`${error.message}\n${failures.join('\n')}`);}
   if(fixture.name==='math')await page.locator('mjx-container').waitFor();
   if(fixture.name==='typing'&&!await page.locator('.typeGood').count())throw Error('Typed answer comparison did not render');
   if(fixture.name==='occlusion'){
    await page.waitForFunction(()=>{const c=document.querySelector('#image-occlusion-canvas');return c&&c.width>0&&c.height>0});
    const pixels=await page.locator('#image-occlusion-canvas').evaluate(c=>{const data=c.getContext('2d').getImageData(0,0,c.width,c.height).data;return [...data].filter((v,i)=>i%4===3&&v>0).length});
    if(pixels===0)throw Error('Image mask did not render');
    const message=await page.locator('#err').textContent();if(message.trim())throw Error(message);
   }
   if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1))throw Error(`${fixture.name} overflow at ${width}`);
   await page.screenshot({path:path.join(directory,`${fixture.name}-${width}-${dark?'dark':'light'}.png`)});
   await page.close();
  }}
 }
 if(failures.length)throw Error(failures.join('\n'));
 console.log('PASS: original Anki math, image occlusion and typing renderer at 420/800, light/dark; all requests fulfilled locally');
}finally{await browser.close();}
