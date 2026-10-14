import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/model-icons-layout");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const checks=[];
try{
 for(const scale of [1,1.25,1.5,2]){
 const page=await browser.newPage({deviceScaleFactor:scale});
 for(const theme of ["light","dark"]){
  await page.setViewportSize({width:980,height:1100});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://localhost:5195/tests/ai-settings-preview.html");url.searchParams.set("modelIcons","1");url.searchParams.set("theme",theme);
  await page.goto(url.href);
  await page.getByText("google/gemini-2.5-pro",{exact:true}).waitFor();
  for(const [name,icon,title] of [["google/gemini-2.5-pro","gemini-color","Gemini"],["anthropic/claude-sonnet-4","claude-color","Claude"],["openai/gpt-5","openai","OpenAI"],["deepseek/deepseek-r1","deepseek-color","DeepSeek"]]){
   const row=page.getByText(name,{exact:true}).locator("..");
   const references=await row.locator("[data-icon]").getAttribute("data-icon");
   if(references!==icon)throw Error("Wrong model icon: "+name);
   if(theme==="light"&&icon.endsWith("-color")){
    const image=row.locator("img");await image.evaluate(img=>img.decode());
    const size=await image.evaluate(img=>({natural:img.naturalWidth,width:img.getBoundingClientRect().width,height:img.getBoundingClientRect().height}));
    if(size.natural!==128||size.width!==20||size.height!==20)throw Error("Icon sample or display size is incorrect");
   }
  }
  const service=page.getByRole("combobox",{name:"服务",exact:true});
  if(await service.locator("[data-icon]").getAttribute("data-icon")!=="openrouter-color")throw Error("Provider icon was replaced by a model icon");
  await page.screenshot({path:resolve(output,`${scale}-${theme}-models.png`)});
  await page.getByRole("button",{name:"截图",exact:true}).click();
  await page.getByRole("combobox",{name:"结果对话 · 模型名称",exact:true}).click();
  const gemini=page.getByRole("option",{name:"google/gemini-2.5-pro",exact:true});
  if(await gemini.locator("[data-icon]").getAttribute("data-icon")!=="gemini-color")throw Error("Feature picker did not use the model icon");
  await page.screenshot({path:resolve(output,`${scale}-${theme}-picker.png`)});
  checks.push({scale,theme,provider:"openrouter-color",models:["gemini-color","claude-color","openai","deepseek-color"],featurePicker:true});
 }
 await page.close();}
 await writeFile(resolve(output,"report.json"),JSON.stringify(checks,null,2));
 console.log("PASS: model-family icons in catalog and feature pickers, provider identity retained; light/dark");
}finally{await browser.close();}
