// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Notice,InlineError} from "./notice";
import {Button} from "./button";

afterEach(cleanup);
it("keeps recovery actions accessible in the same error notice",async()=>{
 const retry=vi.fn();
 render(<Notice title="无法读取设置" actions={<Button onClick={retry}>重试</Button>}>文件正在使用</Notice>);
 const alert=screen.getByRole("alert");
 expect(within(alert).getByText("文件正在使用")).toBeTruthy();
 await userEvent.setup().click(within(alert).getByRole("button",{name:"重试"}));
 expect(retry).toHaveBeenCalledOnce();
 expect(screen.getAllByRole("alert")).toHaveLength(1);
});
it("announces nonblocking information politely",()=>{
 render(<Notice tone="info">启动记录未能写入</Notice>);
 expect(screen.getByRole("status").textContent).toContain("启动记录未能写入");
 expect(screen.queryByRole("alert")).toBeNull();
});
it("dismisses only current content and keeps underlying validation disabled",async()=>{
 const forbidden=vi.fn();
 const view=(message:string)=><><Notice>{<span>{message}</span>}</Notice><Button disabled onClick={forbidden}>Protected action</Button></>;
 const {rerender}=render(view("Current warning"));
 const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"关闭提示"}));
 rerender(view("Current warning"));
 expect(screen.queryByText("Current warning")).toBeNull();
 await user.click(screen.getByRole("button",{name:"Protected action"}));
 expect(forbidden).not.toHaveBeenCalled();
 rerender(view("A new warning"));
 expect(screen.getByRole("alert").textContent).toContain("A new warning");
});
it("provides keyboard-close for the original inline field-error primitive",async()=>{
 const {rerender}=render(<InlineError errors={[{message:"Invalid field"}]}/>);
 const user=userEvent.setup();
 await user.tab();await user.keyboard("{Enter}");
 expect(screen.queryByRole("alert")).toBeNull();
 rerender(<InlineError errors={[{message:"A new field error"}]}/>);
 expect(screen.getByRole("alert").textContent).toContain("A new field error");
});
