import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve,dirname} from "node:path";
import {fileURLToPath} from "node:url";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const root=resolve(dirname(fileURLToPath(import.meta.url)),"../../../..");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||resolve(root,"_temp/ai-usage-layout"));
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{
 const page=await browser.newPage();page.setDefaultTimeout(20000);
 page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const width of [520,800,1280]){
  const zh=language==="zh-CN";await page.setViewportSize({width,height:920});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5237/tests/ai-settings-preview.html");url.searchParams.set("language",language);url.searchParams.set("theme",theme);
  await page.goto(url.href);await page.getByRole("tab",{name:zh?"用量统计":"Usage",exact:true}).click();
  await page.locator(".usage-ranking .rk").first().waitFor();
  if(await page.locator(".usage-ranking").count()!==4)throw Error("Overview lost a usage dimension");
  const position=await page.getByRole("tab",{name:zh?"概览":"Overview",exact:true}).boundingBox();
  if(width===800){await page.locator(".usage-breakdowns").scrollIntoViewIfNeeded();await page.mouse.move(10,10);await page.screenshot({path:resolve(output,`${language}-${theme}-overview.png`)});}
  await page.locator(".usage-ranking").nth(1).locator(".rk").first().click();
  const requestTab=page.getByRole("tab",{name:zh?"请求明细":"Request history",exact:true});
  if(await requestTab.getAttribute("aria-selected")!=="true")throw Error("Ranking did not open its filtered requests");
  await page.locator(".heat-grid").waitFor();
  if(await page.locator(".heat-grid i[data-date]").count()!==371)throw Error("Request heatmap did not retain all 53 weeks");
  await page.locator(".heat-grid i[data-date]").last().hover();await page.locator(".heat-tip").waitFor({state:"visible"});
  await page.getByRole("combobox",{name:zh?"时间范围":"Period",exact:true}).click();
  await page.getByRole("option",{name:zh?"自选日期":"Custom dates",exact:true}).click();
  await page.getByLabel(zh?"起始日期":"From date",{exact:true}).fill("2026-10-08");
  await page.getByLabel(zh?"截止日期":"To date",{exact:true}).fill("2026-10-09");
  await page.getByRole("button",{name:zh?"应用日期":"Apply dates",exact:true}).click();
  for(const view of ["requests","context","sessions"]){
   const label=({requests:["请求明细","Request history"],context:["上下文","Context"],sessions:["会话","Sessions"]})[view][zh?0:1];
   await page.getByRole("tab",{name:label,exact:true}).click();
   if(view==="requests")await page.locator(".usage-table").waitFor();
   if(view==="context"){await page.locator(".usage-context-agent").waitFor();await page.locator(".usage-context-spark").first().locator("..").locator("button").first().click();await page.locator(".usage-context-grid").waitFor();}
   if(view==="sessions"){await page.getByText(zh?"最近会话":"Latest sessions",{exact:true}).waitFor();await page.getByRole("button",{name:zh?"下一页":"Next",exact:true}).click();await page.getByText(zh?"第 16–16 条，共 16 条":"16–16 of 16",{exact:true}).waitFor();}
   const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"],.ai-usage')].filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+2).map(e=>e.className||e.tagName));
   const innerScrollers=await page.locator(".ai-usage").evaluate(root=>[...root.querySelectorAll("*")].filter(e=>{const style=getComputedStyle(e);return e.clientWidth>0&&e.clientHeight>0&&((e.scrollHeight>e.clientHeight+2&&/auto|scroll/.test(style.overflowY))||(e.scrollWidth>e.clientWidth+2&&/auto|scroll/.test(style.overflowX)));}).map(e=>e.className));
   const alerts=await page.getByRole("alert").allTextContents();
   await page.locator('[data-slot="page-scroll"]').evaluate(element=>{element.scrollTop=0;});
   const current=await page.getByRole("tab",{name:zh?"概览":"Overview",exact:true}).boundingBox();
   const navigationShift={x:current.x-position.x,y:current.y-position.y};
   report.push({language,theme,width,view,overflow,innerScrollers,alerts,navigationShift});
   if(width===800){await page.locator(view==="requests"?".led-heat":view==="context"?".usage-context-agent":".usage-session-calendar").first().scrollIntoViewIfNeeded();await page.mouse.move(10,10);await page.screenshot({path:resolve(output,`${language}-${theme}-${view}.png`)});}
  }
  console.log(`Verified ${language} ${theme} ${width}px`);
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(row=>row.overflow.length||row.innerScrollers.length||row.alerts.length||Math.abs(row.navigationShift.x)>1||Math.abs(row.navigationShift.y)>1))throw Error("AI usage visual checks failed; inspect report.json");
 console.log("PASS: Magpie rankings, filtered requests, 53-week heatmap, custom dates, context and paginated sessions; zh/en, light/dark, 520/800/1280px");
}finally{await browser.close();}
