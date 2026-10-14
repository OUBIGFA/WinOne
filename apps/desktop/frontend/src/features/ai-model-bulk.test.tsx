// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {AIProviderModels} from "./ai-provider-models";
import {AIGatewayModels} from "./ai-gateway-models";
import type {AIProvider,AISelection} from "./ai-configuration";
vi.mock("../bridge",()=>({captureCall:()=>{throw Error("bulk selection must not send or save a request");}}));
afterEach(cleanup);
const provider:AIProvider={id:"p",name:"Provider",authMode:"api-key",hasKey:false,models:["alpha"],availableModels:["alpha","beta"],modelNames:{alpha:"Alpha",beta:"Beta"}};
function ProviderEditor({mode}:{mode:"api-key"|"account"}){const [value,setValue]=useState({...provider,authMode:mode});return <AIProviderModels provider={value} canFetch={false} disabled={false} onChange={patch=>setValue(value=>({...value,...patch}))} onBusy={()=>{}}/>;}
it.each(["api-key","account"] as const)("selects and clears the entire %s model list without losing candidates",async mode=>{
 render(<ProviderEditor mode={mode}/>);const user=userEvent.setup();
 await user.type(screen.getByRole("textbox"),"Alpha");await user.click(screen.getByRole("button",{name:"全选"}));
 await user.clear(screen.getByRole("textbox"));expect(screen.getAllByRole("checkbox").every(box=>box.getAttribute("aria-checked")==="true")).toBe(true);
 await user.click(screen.getByRole("button",{name:"全不选"}));expect(screen.getAllByRole("checkbox")).toHaveLength(2);expect(screen.getAllByRole("checkbox").every(box=>box.getAttribute("aria-checked")==="false")).toBe(true);
});
it("uses the same bulk actions for public gateway routes",async()=>{
 function Editor(){const [value,setValue]=useState<AISelection[]>([]);return <AIGatewayModels providers={[provider,{...provider,id:"other",name:"Other"}]} value={value} onChange={setValue} disabled={false}/>;}
 render(<Editor/>);const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"全选"}));expect(screen.getAllByRole("checkbox").every(box=>box.getAttribute("aria-checked")==="true")).toBe(true);
 await user.click(screen.getByRole("button",{name:"全不选"}));expect(screen.getAllByRole("checkbox").every(box=>box.getAttribute("aria-checked")==="false")).toBe(true);
});
