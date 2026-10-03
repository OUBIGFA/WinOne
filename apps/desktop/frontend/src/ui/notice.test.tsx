// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Notice} from "./notice";
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
