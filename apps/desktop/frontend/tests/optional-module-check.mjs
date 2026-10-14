// In-memory registered module; this fixture does not load executable packages.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/optional-module");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});const report=[];
try{
 const page=await browser.newPage(),errors=[];page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [1280,520]){
  await page.setViewportSize({width,height:1000});await page.goto(`${process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187"}/tests/shell-preview.html?optional=1&language=${language}`);
  const toggle=page.getByRole("switch",{name:language==="en-US"?"Example tool":"扩展示例",exact:true});await toggle.waitFor();
  if(await toggle.isChecked()||await page.getByRole("switch").count()!==5)throw Error("Optional module initial state incorrect");
  await toggle.click();await page.waitForFunction(()=>document.querySelector('[aria-labelledby="module-example.tool-name"]')?.getAttribute("aria-checked")==="true");
  if((await page.locator("body").innerText()).includes("moduleDescription.example.tool"))throw Error("Internal translation key visible");
  await page.waitForTimeout(250);await page.screenshot({path:resolve(output,`${language}-${width}.png`),fullPage:true});
  report.push({language,width,overflow:await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)});
  await toggle.click();await page.waitForFunction(()=>document.querySelector('[aria-labelledby="module-example.tool-name"]')?.getAttribute("aria-checked")==="false");
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));if(errors.length||report.some(item=>item.overflow))throw Error("Optional module UI check failed");
 console.log("PASS: registered optional module display and toggles in 4 bilingual home layouts");
}finally{await browser.close();}
