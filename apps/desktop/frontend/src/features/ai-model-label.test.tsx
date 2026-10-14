// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {AIProviderModels} from "./ai-provider-models";
import {AIModelSelect} from "./ai-model-select";
import {isFreeModel,modelLabel} from "./ai-model-label";
import type {AIProvider} from "./ai-configuration";
const api=vi.hoisted(()=>({captureCall:vi.fn()}));
vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});

const provider:AIProvider={id:"test",name:"Test",authMode:"account",agent:"workbuddy",hasKey:false,
 models:["opaque","vendor/model:free","freedom-7b","unknown"],modelNames:{opaque:"Model A"},modelFree:{opaque:true}};

it("marks vendor-free and named-free models without changing routing IDs",()=>{
 render(<AIProviderModels provider={provider} canFetch={false} disabled={false} onChange={()=>{}} onBusy={()=>{}}/>);
 expect(screen.getByText("Model A").closest("label")?.querySelector('[data-model-badge="free"]')?.textContent).toBe("免费");
 expect(screen.getByText("vendor/model:free").closest("label")?.querySelector('[data-model-badge="free"]')?.textContent).toBe("免费");
 expect(screen.getByText("freedom-7b")).toBeTruthy();
 expect(screen.getByText("unknown")).toBeTruthy();
 expect(isFreeModel(provider,"freedom-7b")).toBe(false);
 expect(isFreeModel({modelNames:{x:"Free Model"}},"x")).toBe(true);
 expect(modelLabel({modelNames:{x:"Model X (free)"}},"x")).toBe("Model X");
});

it("retains and clears explicit free metadata when fetching an updated catalog",async()=>{
 api.captureCall.mockResolvedValue({models:[{id:"opaque",name:"Model A",free:false},{id:"new-model",name:"Model B",free:true}]});
 const changed=vi.fn();
 function Fixture(){const [value,setValue]=useState(provider);return <AIProviderModels provider={value} canFetch disabled={false} onBusy={()=>{}} onChange={patch=>{changed(patch);setValue(current=>({...current,...patch}));}}/>;}
 render(<Fixture/>);
 await userEvent.click(screen.getByRole("button",{name:"获取模型列表"}));
 expect((await screen.findByText("Model B")).closest("label")?.querySelector('[data-model-badge="free"]')?.textContent).toBe("免费");
 expect(screen.getByText("Model A")).toBeTruthy();
 expect(screen.getByText("Model A").closest("label")?.querySelector('[data-model-badge="free"]')).toBeNull();
 expect(changed.mock.calls[0][0].modelFree).toEqual({opaque:false,"new-model":true});
 expect(changed.mock.calls[0][0].availableModels).toContain("new-model");
});

it("uses a separate free badge in feature model selectors",()=>{
 render(<AIModelSelect providers={[provider]} value={{provider:"test",model:"opaque"}} label="Task" onChange={()=>{}}/>);
 expect(screen.getByText("Model A")).toBeTruthy();
 expect(screen.getByText("免费").closest('[data-slot="select-trigger"]')).toBeTruthy();
});
