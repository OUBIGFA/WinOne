// Runs only inside test-desktop-components.ps1 with a new profile and pipe.
// No captured text, model requests, user programs or startup settings are used.
import {createRequire} from "node:module";
import {writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
let browser,page;
try {
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
 if(!browser)throw Error("Isolated desktop did not become available");
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes("winone.localhost"));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 if(!page)throw Error("Desktop document did not load");
 const errors=[];page.on("pageerror",error=>errors.push(String(error)));
 const screenshot=async name=>{await page.evaluate(()=>Promise.all([document.fonts.ready,...document.getAnimations().map(a=>a.finished)]));await page.screenshot({path:resolve(output,`${name}.png`)});};
 await page.getByRole("button",{name:"划词助手",exact:true}).click();
 await page.getByRole("tab",{name:"工具栏",exact:true}).click();
 await page.locator(".selection-action-trigger").first().waitFor();
 if(await page.getByLabel("功能名称",{exact:true}).count())throw Error("Toolbar opens all editors at once");
 await screenshot("selection-list");
 await page.getByRole("button",{name:"添加自定义功能",exact:true}).click();
 await page.getByLabel("功能名称",{exact:true}).fill("邮件回复验收");
 await page.getByLabel("AI 指令",{exact:true}).fill("根据选中文字写一封简短的回复。");
 await page.getByRole("button",{name:"设置",exact:true}).click();
 await page.getByRole("dialog").waitFor();
 await page.getByRole("button",{name:"继续编辑",exact:true}).click();
 if(await page.getByLabel("功能名称",{exact:true}).inputValue()!=="邮件回复验收")throw Error("Unsaved action was lost");
 await page.getByRole("button",{name:"保存设置",exact:true}).click();
 await page.getByText("已保存",{exact:true}).waitFor();
 const saved=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Selection("settings.get","null")));
 if(!saved.actions.some(a=>a.name==="邮件回复验收"&&a.prompt==="根据选中文字写一封简短的回复。"))throw Error("Host did not save the custom action");
 await screenshot("selection-editor");
 await page.getByRole("button",{name:"AI服务",exact:true}).click();
 await page.getByRole("button",{name:"添加自定义供应商",exact:true}).click();
 await page.getByLabel("服务名称",{exact:true}).fill("界面验收服务");
 await page.getByLabel("API 基础地址",{exact:true}).fill("https://example.invalid/v1");
 await page.getByLabel("搜索或填写模型 ID",{exact:true}).fill("gpt-5");
 await page.getByRole("button",{name:"添加模型",exact:true}).click();
 await page.getByRole("button",{name:"保存 AI 设置",exact:true}).click();
 await page.getByText("已保存",{exact:true}).waitFor();
 await screenshot("ai-services");
 await page.getByRole("button",{name:"截图",exact:true}).click();
 await page.getByRole("tab",{name:"OCR 与翻译",exact:true}).click();
 await page.getByRole("heading",{name:"后续对话",exact:true}).waitFor();
 await screenshot("capture-model-groups");
 await page.getByRole("button",{name:"设置",exact:true}).click();
 await page.getByRole("radio",{name:"深色",exact:true}).click();
 await page.waitForFunction(()=>document.documentElement.dataset.theme==="dark");
 await screenshot("settings-dark");
 await page.getByRole("button",{name:"划词助手",exact:true}).click();
 await page.getByRole("tab",{name:"工具栏",exact:true}).click();
 await page.getByRole("button",{name:"编辑 邮件回复验收",exact:true}).click();
 await screenshot("selection-dark");
 const overflow=await page.locator('[data-slot="page-scroll"]').evaluate(e=>e.scrollWidth>e.clientWidth+1);
 if(overflow||errors.length)throw Error(JSON.stringify({overflow,errors}));
 await writeFile(resolve(output,"result.json"),JSON.stringify({customActionSaved:true,unsavedProtection:true,sharedAIProviderSaved:true,captureTaskGroups:true,darkTheme:true,overflow,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand("close");}).catch(error=>{if(!String(error).includes("closed"))throw error;});
} catch(error) {
 await writeFile(resolve(output,"node-error.txt"),String(error.stack||error));
 if(page)await page.screenshot({path:resolve(output,"failure.png")}).catch(()=>{});
 throw error;
} finally {if(browser)await browser.close();}
