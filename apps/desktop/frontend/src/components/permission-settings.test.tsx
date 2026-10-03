// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {PermissionSettings} from "./permission-settings";
import {setApplicationLanguage} from "../i18n";
afterEach(async()=>{cleanup();await setApplicationLanguage("zh-CN");});
it("separates remembered startup from the current permission level",async()=>{
 const remember=vi.fn(),restart=vi.fn();render(<PermissionSettings administrator={false} remembered={false} disabled={false} restarting={false} onRemember={remember} onRestart={restart}/>);
 const user=userEvent.setup();await user.click(screen.getByRole("switch",{name:"始终以管理员身份运行"}));expect(remember.mock.calls[0][0]).toBe(true);
 expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("false");
 expect(screen.getByText("当前以普通权限运行")).toBeTruthy();await user.click(screen.getByRole("button",{name:"以管理员身份重新启动"}));expect(restart).toHaveBeenCalledOnce();
});
it("does not offer a second restart while already elevated",()=>{
 render(<PermissionSettings administrator remembered disabled={false} restarting={false} onRemember={()=>{}} onRestart={()=>{}}/>);
 expect(screen.getByRole("button",{name:"以管理员身份重新启动"}).hasAttribute("disabled")).toBe(true);
});
it("supports English permission controls",async()=>{
 await setApplicationLanguage("en-US");render(<PermissionSettings administrator={false} remembered disabled={false} restarting={false} onRemember={()=>{}} onRestart={()=>{}}/>);
 expect(screen.getByRole("switch",{name:"Always run as administrator"})).toBeTruthy();expect(screen.getByRole("button",{name:"Restart as administrator"})).toBeTruthy();
});
