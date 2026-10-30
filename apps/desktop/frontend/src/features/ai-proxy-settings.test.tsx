// @vitest-environment jsdom
import {useState} from "react";
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIProxySettings} from "./ai-proxy-settings";
import type {MagpieSettings} from "./ai-configuration";
afterEach(cleanup);

it("splits pasted credentials, protects the draft, and preserves the custom address through Direct",async()=>{
 const applied=vi.fn(),activity=vi.fn();
 function Fixture(){const [settings,set]=useState<MagpieSettings>({});return <AIProxySettings settings={settings} disabled={false} onActivity={activity} onChange={patch=>{applied(patch);set(old=>({...old,...patch}));}}/>;}
 render(<Fixture/>);const user=userEvent.setup();
 await user.click(screen.getByRole("radio",{name:"自定义代理"}));
 const address=screen.getByRole("textbox",{name:"代理地址"});
 fireEvent.change(address,{target:{value:"http://na%40me:p%3Ass@127.0.0.1:7890"}});
 expect((address as HTMLInputElement).value).toBe("http://127.0.0.1:7890");
 expect((screen.getByLabelText("用户名（可选）") as HTMLInputElement).value).toBe("na@me");
 expect((screen.getByLabelText("密码（可选）") as HTMLInputElement).value).toBe("p:ss");
 expect(screen.getByLabelText("密码（可选）").getAttribute("type")).toBe("password");
 expect(applied).not.toHaveBeenCalled();
 expect(activity).toHaveBeenLastCalledWith(true,false,"ai-proxy",expect.any(Function));
 await user.click(screen.getByRole("button",{name:"应用代理"}));
 expect(applied).toHaveBeenLastCalledWith({proxy:"http://na%40me:p%3Ass@127.0.0.1:7890",proxyCustom:"http://na%40me:p%3Ass@127.0.0.1:7890"});
 await waitFor(()=>expect(activity).toHaveBeenLastCalledWith(false,false,"ai-proxy",expect.any(Function)));
 await user.click(screen.getByRole("radio",{name:"直接连接"}));await user.click(screen.getByRole("button",{name:"应用代理"}));
 expect(applied).toHaveBeenLastCalledWith({proxy:"direct"});
 await user.click(screen.getByRole("radio",{name:"自定义代理"}));
 expect((screen.getByLabelText("用户名（可选）") as HTMLInputElement).value).toBe("na@me");
});
it("keeps an incomplete custom proxy unsaved and reports the missing address",async()=>{
 const applied=vi.fn();render(<AIProxySettings settings={{}} disabled={false} onChange={applied}/>);
 const user=userEvent.setup();await user.click(screen.getByRole("radio",{name:"自定义代理"}));
 await user.type(screen.getByLabelText("用户名（可选）"),"draft-user");
 await user.click(screen.getByRole("button",{name:"应用代理"}));
 expect((await screen.findByRole("alert")).textContent).toContain("请填写代理地址");expect(applied).not.toHaveBeenCalled();
});
