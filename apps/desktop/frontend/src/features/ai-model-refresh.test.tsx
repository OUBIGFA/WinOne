// @vitest-environment jsdom
import {useState} from "react";
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIProviderModels} from "./ai-provider-models";
import type {AIProvider} from "./ai-configuration";

const api=vi.hoisted(()=>({captureCall:vi.fn()}));
vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.resetAllMocks();});

const original:AIProvider={
 id:"one",name:"Provider",authMode:"api-key",protocol:"chat",baseUrl:"https://fixture.invalid/v1",hasKey:true,
 models:["retired","retained","plain"],availableModels:["retired","retained","plain","disabled"],
 modelNames:{retired:"Retired model",retained:"Old model (free)",plain:"Old plain name",disabled:"Disabled model"},
 modelFree:{retired:true,retained:true,plain:true},
 modelMetadata:{retired:{rate:1},retained:{rate:.5,rateWas:1,listedContextWindow:1000000,vision:true,price:{input:2}},plain:{rate:2}},
};

function fixture(provider:AIProvider=original){
 let current:AIProvider;
 function Editor(){
  const [value,setValue]=useState(()=>structuredClone(provider));
  current=value;
  return <AIProviderModels provider={value} canFetch disabled={false} onBusy={()=>{}} onChange={patch=>setValue(previous=>({...previous,...patch}))}/>;
 }
 const view=render(<Editor/>);
 return {...view,current:()=>current};
}

it.each(["api-key","account"] as const)("replaces the %s catalog and labels while retaining only existing enabled selections",async authMode=>{
 api.captureCall.mockResolvedValue({models:[
  {id:"retained",name:"Current model",metadata:{rate:2,listedContextWindow:200000,vision:false}},
  {id:"plain"},{id:"new",name:"New model",free:true},{id:"disabled",name:"Current disabled"},
 ]});
 const view=fixture({...original,authMode,...(authMode==="account"?{agent:"codex"}:{})});
 await userEvent.click(screen.getByRole("button",{name:"获取模型列表"}));
 expect(await screen.findByRole("checkbox",{name:"Current model 2×",checked:true})).toBeTruthy();
 expect(screen.queryByRole("checkbox",{name:"Retired model"})).toBeNull();
 expect(screen.getByRole("checkbox",{name:"plain",checked:true})).toBeTruthy();
 expect(screen.getByRole("checkbox",{name:"New model 免费",checked:false})).toBeTruthy();
 expect(screen.getByRole("checkbox",{name:"Current disabled",checked:false})).toBeTruthy();
 expect(view.current()).toMatchObject({
  availableModels:["retained","plain","new","disabled"],models:["retained","plain"],
  modelNames:{retained:"Current model",new:"New model",disabled:"Current disabled"},
  modelFree:{retained:false,plain:false,new:true,disabled:false},
  modelMetadata:{retained:{rate:2,listedContextWindow:200000,vision:false}},
 });
 expect(Object.keys(view.current().modelNames!)).toEqual(["retained","new","disabled"]);
 expect(Object.keys(view.current().modelMetadata!)).toEqual(["retained"]);
 expect(view.current().modelMetadata!.retained).toEqual({rate:2,listedContextWindow:200000,vision:false});
 expect(view.container.querySelector('[data-model-badge="context"]')).toBeNull();
 expect(view.container.querySelector('[data-model-badge="rate"] s')).toBeNull();
});

it("clears the catalog and all model metadata when a successful discovery returns no models",async()=>{
 api.captureCall.mockResolvedValue({models:[]});
 const view=fixture();await userEvent.click(screen.getByRole("button",{name:"获取模型列表"}));
 await waitFor(()=>expect(screen.queryAllByRole("checkbox")).toHaveLength(0));
 expect(view.current()).toMatchObject({models:[],availableModels:[],modelNames:{},modelFree:{},modelMetadata:{}});
 expect(screen.getByRole("button",{name:"测试已选模型"})).toHaveProperty("disabled",true);
});

it("keeps the complete draft after discovery fails and allows a successful retry",async()=>{
 api.captureCall.mockRejectedValueOnce(Error("Provider unavailable")).mockResolvedValueOnce({models:[{id:"retained",name:"Recovered model"}]});
 const view=fixture(),user=userEvent.setup();await user.click(screen.getByRole("button",{name:"获取模型列表"}));
 expect(await screen.findByText(/Provider unavailable/)).toBeTruthy();
 expect(view.current()).toEqual(original);
 await user.click(screen.getByRole("button",{name:"获取模型列表"}));
 expect(await screen.findByRole("checkbox",{name:"Recovered model",checked:true})).toBeTruthy();
 expect(screen.queryByText(/Provider unavailable/)).toBeNull();
});

it("clears old connectivity results after replacing the model catalog",async()=>{
 api.captureCall.mockImplementation(async method=>{
  if(method==="ai.models.test")return {results:[{model:"retained",ok:true,ms:77,protocol:"chat"}],clients:{codex:"Old client status"}};
  if(method==="ai.models.fetch")return {models:[{id:"retained",name:"Current model"}]};
  throw Error(method);
 });
 fixture({...original,models:["retained"]});const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"测试已选模型"}));await screen.findByText(/77 ms/);
 await user.click(screen.getByRole("button",{name:"获取模型列表"}));await screen.findByRole("checkbox",{name:"Current model"});
 expect(screen.queryByText(/77 ms/)).toBeNull();
 expect(screen.queryByText(/Old client status/)).toBeNull();
 expect(screen.queryByText("全部 1 个模型测试通过")).toBeNull();
});
