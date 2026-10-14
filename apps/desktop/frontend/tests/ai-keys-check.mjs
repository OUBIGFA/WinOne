import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/ai-keys-layout");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const base=process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5201";
const report=[],errors=[];
try{
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const width of [520,800,1280]){
  const zh=language==="zh-CN";
  const context=await browser.newContext({viewport:{width,height:1000},colorScheme:theme,reducedMotion:"reduce"});
  const page=await context.newPage();page.on("pageerror",error=>errors.push(String(error)));
  await page.goto(`${base}/tests/ai-settings-preview.html?multipleKeys=1&language=${language}&theme=${theme}`);
  await page.getByLabel(zh?"API 密钥 3":"API key 3",{exact:true}).waitFor();
  if(await page.locator('input[type="password"]').count()!==3)throw Error("Three independent key inputs missing");
  await page.getByRole("button",{name:zh?"添加密钥":"Add API key",exact:true}).click();
  await page.getByLabel(zh?"API 密钥 4":"API key 4",{exact:true}).fill("fixture-extra-secret");
  await page.getByRole("button",{name:zh?"删除密钥 2":"Remove key 2",exact:true}).click();
  if(await page.getByLabel(zh?"API 密钥 3":"API key 3",{exact:true}).inputValue()!=="fixture-extra-secret")throw Error("Removing another row changed the key draft");
  await page.getByRole("combobox",{name:zh?"密钥使用策略":"Key selection",exact:true}).click();
  await page.getByRole("option",{name:zh?"按顺序使用":"In list order",exact:true}).click();
  await page.getByLabel(zh?"最多尝试密钥数":"Maximum keys to try",{exact:true}).fill("2");
  await page.getByLabel(zh?"失败冷却时间（秒）":"Failure cooldown (seconds)",{exact:true}).fill("180");
  await page.getByRole("switch",{name:zh?"启用密钥 2":"Enable key 2",exact:true}).click();
  await page.getByRole("button",{name:zh?"保存 AI 设置":"Save AI settings",exact:true}).click();
  await page.getByRole("status").filter({hasText:zh?"已保存":"Saved"}).waitFor();
  const saved=await page.evaluate(()=>{
   const value=window.fixtureLastSave,provider=value.configuration.providers[0];
   return {count:provider.keys.length,removed:!provider.keys.some(key=>key.id==="second"),legacyPreserved:provider.keys[0].id==="primary",enabled:provider.keys.every(key=>key.enabled),policy:provider.keyPolicy,onlyNewSecret:Object.keys(value.keyUpdates.one).length===1&&value.keyUpdates.one[provider.keys[2].id]==="fixture-extra-secret"};
  });
  if(saved.count!==3||!saved.removed||!saved.legacyPreserved||!saved.enabled||!saved.onlyNewSecret||saved.policy.strategy!=="ordered"||saved.policy.maxAttempts!==2||saved.policy.cooldownSeconds!==180)throw Error("Incorrect saved key configuration");
  if(await page.getByLabel(zh?"API 密钥 3":"API key 3",{exact:true}).inputValue()!=="")throw Error("Saved secret remained in the input");
  const geometry=await page.evaluate(()=>({
   overflow:[...document.querySelectorAll('html,body,main,[data-slot="page-scroll"]')].filter(e=>e.clientWidth&&e.scrollWidth>e.clientWidth+1).map(e=>e.className||e.tagName),
   // Base UI keeps visually hidden native inputs for form/accessibility state.
   // Their intentional offscreen coordinates are not clipped visible controls.
   clipped:[...document.querySelectorAll('button,input,[role="combobox"]')].filter(e=>{const r=e.getBoundingClientRect(),style=getComputedStyle(e);if(e.tagName==="INPUT"&&(e.type==="hidden"||e.getAttribute("aria-hidden")==="true"||style.clip!=="auto"||style.clipPath!=="none"||style.opacity==="0"))return false;return r.width&&r.height&&(r.left<0||r.right>innerWidth+1)}).map(e=>e.id||e.textContent)
  }));
  report.push({language,theme,width,...saved,...geometry});
  if(width===800){await page.getByLabel(zh?"API 密钥 1":"API key 1",{exact:true}).scrollIntoViewIfNeeded();await page.evaluate(()=>Promise.all(document.getAnimations().map(animation=>animation.finished)));await page.screenshot({path:resolve(output,`${language}-${theme}.png`)});}
  await context.close();
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(row=>row.overflow.length||row.clipped.length))throw Error("Key settings layout failed");
 console.log("PASS: multi-key add/remove/enable, policy save, zh/en, light/dark, 520/800/1280px");
}finally{await browser.close();await writeFile(resolve(output,"browser-exit.json"),JSON.stringify({closed:true}));}
