import {readNativeState} from "./native-state.mjs";
import {clickPrivateCanvas} from "./native-probe-input.mjs";
import {createRequire} from "node:module";
import {readFile,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=process.env.WINONE_NATIVE_PROBE_OUTPUT;if(!output)throw Error("Set WINONE_NATIVE_PROBE_OUTPUT");
const browser=await chromium.connectOverCDP(process.env.WINONE_NATIVE_PROBE_CDP||"http://127.0.0.1:43192");
try{
 const page=browser.contexts()[0].pages()[0];
 const button=name=>page.getByRole("button",{name,exact:true});
 const field=()=>page.getByRole("textbox",{name:"输入文字",exact:true});
 const state=async()=>(await readNativeState(resolve(output,"state.json")));
 async function save(count){await button("保存").click();await page.waitForFunction(()=>!document.querySelector("button").disabled);const value=await state();if(value.saved!==count)throw Error("Save did not complete");return value;}
 await button("文字").click();await clickPrivateCanvas(output,124,124);await field().fill("保留这段文字");await field().press("Control+Enter");await field().waitFor({state:"detached"});
 const initial=await save(1);
 await button("文字").click();await clickPrivateCanvas(output,129,129);if(await field().inputValue()!=="保留这段文字")throw Error("Existing text was not loaded");
 const originalBounds=await field().boundingBox();
 await field().fill("取消修改");await field().press("Escape");await field().waitFor({state:"detached"});
 await save(2);
 await button("文字").click();await clickPrivateCanvas(output,129,129);if(await field().inputValue()!=="保留这段文字")throw Error("Cancelled edit changed text");
 const editBounds=await field().boundingBox();
 if(editBounds.x!==originalBounds.x||editBounds.y!==originalBounds.y||editBounds.width!==originalBounds.width)throw Error("Re-edit moved the text");
 await field().fill("保存新的内容");
 const edited=await save(3);if(edited.annotations!==initial.annotations)throw Error("Editing added another text layer");
 await button("撤销").click();await page.waitForFunction(()=>!document.querySelector("button").disabled);
 const undone=await save(4);if(undone.annotations!==initial.annotations)throw Error("One undo removed the entire text layer");
 await writeFile(resolve(output,"edit-verification.json"),JSON.stringify({originalBounds,editBounds,initial,edited,undone},null,2));
 const closed=page.waitForEvent("close",{timeout:10000});await button("取消").click();await closed;
 console.log("PASS: existing text loads in place, cancel preserves it, save updates one layer and one undo restores it");
}finally{await browser.close();}
