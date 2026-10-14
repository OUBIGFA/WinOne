// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {SelectionPage} from "./selection";
import {setApplicationLanguage} from "../i18n";
const calls=vi.hoisted(()=>({quick:vi.fn(),selection:vi.fn()}));
vi.mock("../bridge",()=>({quickAssistantCall:calls.quick,selectionCall:calls.selection,captureCall:vi.fn(async()=>({providers:[],tasks:{}}))}));
afterEach(async()=>{cleanup();vi.clearAllMocks();await setApplicationLanguage("zh-CN");});
it("opens chat and saves independent custom actions through the quick assistant",async()=>{
 const prefs={schemaVersion:1,revision:3,actions:[{id:"copy",name:"复制",icon:"copy",kind:"copy",enabled:true}]};
 calls.quick.mockImplementation(async(method,payload)=>method==="settings.save"?{...payload,revision:4}:prefs);
 const user=userEvent.setup();render(<SelectionPage moduleId="quick-assistant"/>);
 await user.click(await screen.findByRole("button",{name:"打开快捷助手"}));
 expect(calls.quick).toHaveBeenCalledWith("show");
 await user.click(screen.getByRole("tab",{name:"常规"}));
 expect(screen.getByRole("switch",{name:"默认置顶快捷助手"}).getAttribute("aria-checked")).toBe("false");
 await user.click(screen.getByRole("switch",{name:"默认置顶快捷助手"}));
 await user.click(screen.getByRole("switch",{name:"显示快捷助手的思考过程"}));
 await user.click(screen.getByRole("tab",{name:"工具栏"}));
 await user.click(await screen.findByRole("button",{name:"添加自定义功能"}));
 fireEvent.change(screen.getByLabelText("功能名称"),{target:{value:"邮件回复"}});
 fireEvent.change(screen.getByLabelText("AI 指令"),{target:{value:"写一封邮件回复。"}});
 await user.click(screen.getByRole("button",{name:"保存设置"}));await screen.findByText("已保存");
 const saved=calls.quick.mock.calls.find(([method])=>method==="settings.save")?.[1];
 expect(saved.alwaysOnTop).toBe(true);expect(saved.showReasoning).toBe(false);
 expect(saved.actions).toHaveLength(2);expect(saved).not.toHaveProperty("excludedApps");
 expect(saved.actions[1]).toMatchObject({name:"邮件回复",prompt:"写一封邮件回复。"});
 expect(calls.selection).not.toHaveBeenCalled();
});

