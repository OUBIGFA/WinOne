// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {cleanup,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {AIProviderModels} from "./ai-provider-models";
import {AIModelSettings,AIRequestLimits} from "./ai-model-settings";
import type {AIProvider} from "./ai-configuration";
afterEach(cleanup);

it("edits a model's window and output without changing its routing ID",async()=>{
 let changed:AIProvider|undefined;
 function Fixture(){const [provider,setProvider]=useState<AIProvider>({id:"p",name:"P",authMode:"account",hasKey:false,models:["opaque"],modelNames:{opaque:"Visible"}});return <AIProviderModels provider={provider} canFetch={false} disabled={false} onBusy={()=>{}} onChange={patch=>setProvider(current=>changed={...current,...patch})}/>;}
 render(<Fixture/>);
 await userEvent.click(screen.getByRole("button",{name:"模型设置：Visible"}));
 await userEvent.type(screen.getByLabelText("上下文窗口（Token）"),"128000");
 await userEvent.type(screen.getByLabelText("最大输出（Token）"),"8000");
 await userEvent.type(screen.getByLabelText("压缩阈值（%）"),"80");
 expect(changed?.modelMetadata?.opaque).toMatchObject({contextWindow:128000,maxTokens:8000,compactionPercent:80,compactionThreshold:0});
 expect(changed?.models).toEqual(["opaque"]);
});

function modelFixture(metadata:AIProvider["modelMetadata"]={}){
 let current:AIProvider={id:"p",name:"P",authMode:"account",hasKey:false,models:["opaque"],modelMetadata:metadata};
 function Fixture(){const [provider,setProvider]=useState(current);return <AIModelSettings provider={provider} model="opaque" disabled={false} onChange={patch=>setProvider(value=>current={...value,...patch})} onClose={()=>{}}/>;}
 render(<Fixture/>);return ()=>current;
}
it("provides every upstream window and output shortcut and the requested percentage shortcuts",async()=>{
 const current=modelFixture();const user=userEvent.setup();const windowPresets=screen.getByRole("group",{name:"上下文窗口快捷值"}),outputPresets=screen.getByRole("group",{name:"最大输出快捷值"}),compactPresets=screen.getByRole("group",{name:"压缩阈值快捷值"});
 for(const label of ["128K","200K","256K","1M"])expect(within(windowPresets).getByRole("button",{name:label})).toBeTruthy();
 for(const label of ["8K","16K","32K","64K","128K"])expect(within(outputPresets).getByRole("button",{name:label})).toBeTruthy();
 for(const label of ["50%","60%","70%","80%","90%","95%"])expect(within(compactPresets).getByRole("button",{name:label})).toBeTruthy();
 await user.click(within(windowPresets).getByRole("button",{name:"256K"}));await user.click(within(outputPresets).getByRole("button",{name:"64K"}));await user.click(within(compactPresets).getByRole("button",{name:"80%"}));
 expect(current().modelMetadata?.opaque).toMatchObject({contextWindow:256000,maxTokens:64000,compactionPercent:80,compactionThreshold:0});expect((screen.getByLabelText("压缩阈值（%）") as HTMLInputElement).value).toBe("80");expect(within(compactPresets).getByRole("button",{name:"80%"}).getAttribute("aria-pressed")).toBe("true");
});
it("keeps a legacy token threshold until the user changes the percentage",async()=>{
 const current=modelFixture({opaque:{contextWindow:200000,compactionThreshold:150000}});const user=userEvent.setup();expect((screen.getByLabelText("压缩阈值（%）") as HTMLInputElement).value).toBe("75");await user.type(screen.getByLabelText("显示名称"),"Old setting");expect(current().modelMetadata?.opaque).toEqual({contextWindow:200000,compactionThreshold:150000});
 await user.click(screen.getByRole("button",{name:"90%"}));expect(current().modelMetadata?.opaque).toMatchObject({compactionPercent:90,compactionThreshold:0});
});
it("retains manual decimal percentages and clears the legacy value on reset",async()=>{
 const current=modelFixture({opaque:{contextWindow:200000,compactionThreshold:100000}});const user=userEvent.setup(),field=screen.getByLabelText("压缩阈值（%）");await user.clear(field);await user.type(field,"72.5");expect(current().modelMetadata?.opaque).toMatchObject({compactionPercent:72.5,compactionThreshold:0});await user.clear(field);expect(current().modelMetadata?.opaque).toMatchObject({compactionPercent:0,compactionThreshold:0});
});
it("offers the reported model maximum without replacing it with a guessed size",async()=>{
 const current=modelFixture({opaque:{contextWindow:272000,listedContextWindow:272000,maxContextWindow:872000}});await userEvent.click(screen.getByRole("button",{name:"模型最大值 · 872K"}));expect(current().modelMetadata?.opaque?.contextWindow).toBe(872000);
});
it("keeps all reasoning choices accessible as one-click controls",async()=>{
 const current=modelFixture();const group=screen.getByRole("group",{name:"思考档位快捷值"});for(const effort of ["none","minimal","low","medium","high","xhigh","max","ultra"])expect(within(group).getByRole("button",{name:effort})).toBeTruthy();await userEvent.click(within(group).getByRole("button",{name:"xhigh"}));expect(current().modelMetadata?.opaque?.reasoningEffort).toBe("xhigh");
});
it("offers the vendor's reported reasoning levels and service tiers",async()=>{
 modelFixture({opaque:{efforts:["low","high"],tiers:["priority"]}});
 const presets=screen.getByRole("group",{name:"思考档位快捷值"});
 expect(within(presets).getByRole("button",{name:"high"})).toBeTruthy();
 expect(within(presets).queryByRole("button",{name:"ultra"})).toBeNull();
 await userEvent.click(screen.getByRole("combobox",{name:"服务档位"}));
 expect(await screen.findByRole("option",{name:"priority"})).toBeTruthy();
 expect(screen.queryByRole("option",{name:"ultrafast"})).toBeNull();
});
it("offers a plugin's own reasoning levels instead of a fixed ladder",async()=>{
 // Our Free Model's MiMo V2.5 reports light/balanced/deep, not low..max.
 const current=modelFixture({opaque:{levels:["light","balanced","deep"],efforts:["light","balanced","deep"]}});
 const offered=screen.getByRole("group",{name:"向客户端提供的思考档位"});
 expect(within(offered).getAllByRole("checkbox").length).toBe(3);
 expect(within(offered).queryByRole("checkbox",{name:"high"})).toBeNull();
 await userEvent.click(within(offered).getByRole("checkbox",{name:"light"}));
 expect(current().modelMetadata?.opaque).toMatchObject({levels:["light","balanced","deep"],efforts:["balanced","deep"]});
 expect(within(screen.getByRole("group",{name:"向客户端提供的思考档位"})).getByRole("checkbox",{name:"light"})).toBeTruthy();
 const presets=screen.getByRole("group",{name:"思考档位快捷值"});
 expect(within(presets).queryByRole("button",{name:"high"})).toBeNull();
 await userEvent.click(within(presets).getByRole("button",{name:"deep"}));
 expect(current().modelMetadata?.opaque?.reasoningEffort).toBe("deep");
});
it("keeps a legacy saved vendor ladder when narrowing its offer",async()=>{
 const current=modelFixture({opaque:{efforts:["light","balanced","deep"]}});
 await userEvent.click(within(screen.getByRole("group",{name:"向客户端提供的思考档位"})).getByRole("checkbox",{name:"deep"}));
 expect(current().modelMetadata?.opaque).toMatchObject({levels:["light","balanced","deep"],efforts:["light","balanced"]});
});
it("retains manual request limits where upstream has no shortcut presets",async()=>{
 let current:AIProvider={id:"p",name:"P",authMode:"account",hasKey:false,models:[]};function Fixture(){const [provider,setProvider]=useState(current);return <AIRequestLimits provider={provider} disabled={false} onChange={patch=>setProvider(value=>current={...value,...patch})}/>;}render(<Fixture/>);const user=userEvent.setup();await user.click(screen.getByText("请求限制",{selector:"summary"}));for(const [name,value] of [["并发上限","3"],["每分钟请求数（RPM）","20"],["排队上限","10"],["排队等待（秒）","30"]])await user.type(screen.getByLabelText(name),value);expect(current).toMatchObject({maxConcurrency:3,maxRPM:20,queueLimit:10,queueWait:30});
});

it("keeps grouping, protocol, offered efforts and prices separate from the model routing ID",async()=>{
 const current=modelFixture();const user=userEvent.setup();
 await user.type(screen.getByLabelText("映射为同一模型"),"gpt-5.5");
 // Base UI mounts the portal after focus has settled; querying synchronously
 // races the preceding dialog's cleanup when the whole file runs together.
 await user.click(screen.getByRole("combobox",{name:"此模型使用的 API"}));await user.click(await screen.findByRole("option",{name:"OpenAI Responses"}));
 await user.click(within(screen.getByRole("group",{name:"向客户端提供的思考档位"})).getByRole("checkbox",{name:"high"}));
 await user.type(screen.getByLabelText("输入价格（美元 / 百万 Token）"),"2.5");
 await user.type(screen.getByLabelText("输出价格（美元 / 百万 Token）"),"10");
 expect(current().modelMetadata?.opaque).toMatchObject({same:"gpt-5.5",api:"responses",efforts:["high"],price:{input:2.5,output:10}});
 expect(current().models).toEqual(["opaque"]);
});

it("restores upstream name and level overrides without deleting configured request limits",async()=>{
 const current=modelFixture({opaque:{same:"gpt-5.5",api:"chat",efforts:["high"],price:{input:2,output:8},vision:true,contextWindow:200000}});
 await userEvent.click(screen.getByRole("button",{name:"恢复模型默认值"}));
 expect(current().modelMetadata?.opaque).toEqual({contextWindow:200000});
});
