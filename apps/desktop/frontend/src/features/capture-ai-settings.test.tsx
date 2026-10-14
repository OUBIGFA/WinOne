// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {CaptureAISettings} from "./capture-ai-settings";
const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});

it("can return a feature to the shared default without rewriting other tasks",async()=>{
 const configuration={revision:1,engine:"llm",ocrEngine:"local",target:"auto",prompt:"",providers:[{id:"one",name:"共享服务",models:["shared"]},{id:"two",name:"单独服务",models:["specific"]}],tasks:{default:{provider:"one",model:"shared"},translate:{provider:"two",model:"specific"},chat:{provider:"two",model:"specific"}}};let saved:any;
 call.mockImplementation(async(method,value)=>{if(method==="ai.configuration.get")return configuration;saved=value;return {...value.configuration,revision:2}});render(<CaptureAISettings/>);const user=userEvent.setup();await user.click(await screen.findByRole("combobox",{name:"截图翻译 · 供应商"}));await user.click(await screen.findByRole("option",{name:"跟随 AI 服务默认设置"}));await user.click(screen.getByRole("button",{name:"保存设置"}));await screen.findByText("已保存");expect(saved.configuration.tasks.translate).toBeUndefined();expect(saved.configuration.tasks.chat).toEqual(configuration.tasks.chat);expect(saved.configuration.tasks.default).toEqual(configuration.tasks.default);
});
it("assigns independent models to screenshot features and excludes unchecked models",async()=>{
 const original={revision:2,engine:"llm",ocrEngine:"ai",target:"auto",prompt:"",providers:[{id:"one",name:"视觉服务",models:["vision"],availableModels:["vision","disabled"],hasKey:true},{id:"two",name:"文本服务",models:["fast","precise"],hasKey:true}],tasks:{ocr:{provider:"one",model:"vision"},translate:{provider:"two",model:"fast"},chat:{provider:"two",model:"fast"}}};
 let saved:any;call.mockImplementation(async(method,value)=>{if(method==="ai.configuration.get")return original;saved=value;return {...value.configuration,revision:3};});
 render(<CaptureAISettings/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("combobox",{name:"截图 OCR · 模型名称"}));
 expect(screen.queryByRole("option",{name:"disabled"})).toBeNull();await user.keyboard("{Escape}");
 await user.click(screen.getByRole("combobox",{name:"结果对话 · 模型名称"}));await user.click(await screen.findByRole("option",{name:"precise"}));
 await user.click(screen.getByRole("button",{name:"保存设置"}));await screen.findByText("已保存");
 expect(saved.configuration.tasks.chat).toEqual({provider:"two",model:"precise"});
 expect(saved.configuration.tasks.translate).toEqual(original.tasks.translate);
 expect(saved.configuration.tasks.ocr).toEqual(original.tasks.ocr);
 expect(saved.keys).toBeUndefined();
});
