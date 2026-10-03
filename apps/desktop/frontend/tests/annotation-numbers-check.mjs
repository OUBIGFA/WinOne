import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/annotation-numbers");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [800,520]){
  const en=language==="en-US",base=process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187";
  await page.setViewportSize({width,height:900});await page.goto(`${base}/tests/annotation-properties.html?language=${language}`);
  await page.getByRole("tab",{name:en?"Position and size":"位置与尺寸"}).click();
  const x=page.getByRole("spinbutton",{name:en?"Horizontal position":"水平位置"}),apply=page.getByRole("button",{name:en?"Apply":"应用",exact:true});
  await x.fill("");if(await x.inputValue()!==""||!await apply.isDisabled())throw Error("Empty coordinate can move the annotation");
  await page.waitForTimeout(250);await page.screenshot({path:resolve(output,`${language}-${width}.png`),fullPage:true});
  // Type the decimal separator as a real key, not just a final value assignment.
  await x.pressSequentially("12.5");if(await x.inputValue()!=="12.5")throw Error("Decimal typing changed the coordinate");
  await apply.click();
  await page.getByRole("complementary").waitFor({state:"detached"});
  const result=JSON.parse(await page.locator("#result").textContent());if(result.x!==12.5||result.y!==20||result.text!=="WinOne")throw Error("Applying geometry changed unrelated annotation data");
  await page.goto(`${base}/tests/annotation-properties.html?language=${language}&effect=1`);
  const strength=page.getByRole("spinbutton",{name:en?"Strength":"强度"});await strength.fill("3.5");
  if(!await apply.isDisabled())throw Error("Fractional effect strength accepted");
  await page.getByRole("button",{name:en?"Cancel":"取消",exact:true}).click();
  if(JSON.parse(await page.locator("#result").textContent()).effectSize!==12)throw Error("Cancelled editing changed the annotation");
  report.push({language,width,passed:true});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length)throw Error(errors.join("\n"));
 console.log("PASS: decimal typing, empty coordinate protection, integer effect strength and cancellation in 4 bilingual layouts");
}finally{await browser.close();}
