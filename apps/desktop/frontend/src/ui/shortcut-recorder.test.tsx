// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {ShortcutRecorder} from "./shortcut-recorder";
afterEach(cleanup);
it("records actual key presses and restores registration",async()=>{
 const port={begin:vi.fn().mockResolvedValue("token"),end:vi.fn().mockResolvedValue(undefined)};
 function Preview(){const[value,setValue]=useState("Ctrl+Alt+T");return <><label htmlFor="hotkey">快捷键</label><ShortcutRecorder id="hotkey" value={value} onChange={setValue} port={port}/></>;}
 render(<Preview/>);const user=userEvent.setup();await user.click(screen.getByRole("textbox",{name:"快捷键"}));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");
 await user.keyboard('{Control>}{Shift>}{F9}{/Shift}{/Control}');
 await waitFor(()=>expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("Ctrl+Shift+F9"));expect(port.end).toHaveBeenCalledWith("token");
});
it("cancels without changing the value and releases on blur",async()=>{
 const port={begin:vi.fn().mockResolvedValue("token"),end:vi.fn().mockResolvedValue(undefined)},change=vi.fn();
 render(<><ShortcutRecorder id="hotkey" value="Ctrl+Alt+T" onChange={change} port={port}/><button>next</button></>);
 const user=userEvent.setup();await user.click(screen.getByRole("textbox"));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.keyboard('{Escape}');
 await waitFor(()=>expect(port.end).toHaveBeenCalledOnce());expect(change).not.toHaveBeenCalled();
 await user.click(screen.getByRole("textbox"));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.click(screen.getByText("next"));await waitFor(()=>expect(port.end).toHaveBeenCalledTimes(2));
});

it("waits for modifier release before restoring global shortcuts",async()=>{
 const port={begin:vi.fn().mockResolvedValue("token"),end:vi.fn().mockResolvedValue(undefined)},change=vi.fn();render(<ShortcutRecorder id="hotkey" value="" onChange={change} port={port}/>);
 const user=userEvent.setup();await user.click(screen.getByRole("textbox"));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.keyboard('{Control>}a');
 expect(port.end).not.toHaveBeenCalled();expect(change).not.toHaveBeenCalled();await user.keyboard('{/Control}');await waitFor(()=>expect(change).toHaveBeenCalledWith("Ctrl+A"));
});

it("clears a shortcut even when the recording field still has focus",async()=>{
 const port={begin:vi.fn().mockResolvedValue("token"),end:vi.fn().mockResolvedValue(undefined)},change=vi.fn();render(<ShortcutRecorder id="hotkey" value="Ctrl+Alt+T" onChange={change} port={port}/>);
 const user=userEvent.setup();await user.click(screen.getByRole("textbox"));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.click(screen.getByRole("button",{name:"清除"}));await waitFor(()=>expect(change).toHaveBeenCalledWith(""));expect(port.end).toHaveBeenCalledWith("token");
});

it("releases a late recording response after the control unmounts",async()=>{
 let resolve!:(token:string)=>void;const port={begin:vi.fn(()=>new Promise<string>(done=>{resolve=done;})),end:vi.fn().mockResolvedValue(undefined)};
 const view=render(<ShortcutRecorder id="hotkey" value="Ctrl+Alt+T" onChange={()=>{}} port={port}/>);await userEvent.setup().click(screen.getByRole("textbox"));view.unmount();await act(async()=>resolve("late-token"));await waitFor(()=>expect(port.end).toHaveBeenCalledWith("late-token"));
});

it("uses Delete to clear the binding",async()=>{
 const port={begin:vi.fn().mockResolvedValue("token"),end:vi.fn().mockResolvedValue(undefined)},change=vi.fn();render(<ShortcutRecorder id="hotkey" value="Ctrl+Alt+T" onChange={change} port={port}/>);
 const user=userEvent.setup();await user.click(screen.getByRole("textbox"));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.keyboard('{Delete}');await waitFor(()=>expect(change).toHaveBeenCalledWith(""));expect(port.end).toHaveBeenCalledWith("token");
});

it("restores shortcuts when the application loses focus",async()=>{
 const port={begin:vi.fn().mockResolvedValue("token"),end:vi.fn().mockResolvedValue(undefined)},change=vi.fn();render(<ShortcutRecorder id="hotkey" value="Ctrl+Alt+T" onChange={change} port={port}/>);
 await userEvent.setup().click(screen.getByRole("textbox"));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");fireEvent(window,new Event("blur"));await waitFor(()=>expect(port.end).toHaveBeenCalledWith("token"));expect(change).not.toHaveBeenCalled();
});
