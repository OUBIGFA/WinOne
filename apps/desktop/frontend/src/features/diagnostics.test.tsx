// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Diagnostics} from "./diagnostics";
import {setApplicationLanguage} from "../i18n";

const native=vi.hoisted(()=>({read:vi.fn(),open:vi.fn()}));
vi.mock("../bridge",()=>({readModuleLogs:native.read,openModuleLogFolder:native.open}));
afterEach(async()=>{cleanup();native.read.mockReset();native.open.mockReset();await setApplicationLanguage("zh-CN");});
const logs=(text:string)=>({folderAvailable:true,entries:[{name:"process",available:true,truncated:false,text}]});

it("reads only its owning module and opens that module folder",async()=>{
  native.read.mockResolvedValue(logs("test log"));native.open.mockResolvedValue(undefined);
  render(<Diagnostics moduleId="wintray"/>);const user=userEvent.setup();
  await screen.findByText("test log");
  expect(native.read).toHaveBeenCalledExactlyOnceWith("wintray");
  await user.click(screen.getByRole("button",{name:"打开日志文件夹"}));
  expect(native.open).toHaveBeenCalledExactlyOnceWith("wintray");
  expect(screen.queryByRole("combobox")).toBeNull();
});

it("discards a response from a module that is no longer selected",async()=>{
  let finish!:(value:ReturnType<typeof logs>)=>void;
  native.read.mockImplementation((id:string)=>id==="wintray"?new Promise(resolve=>{finish=resolve;}):Promise.resolve(logs("capture log")));
  const view=render(<Diagnostics moduleId="wintray"/>);
  await waitFor(()=>expect(native.read).toHaveBeenCalledWith("wintray"));
  view.rerender(<Diagnostics moduleId="capture"/>);
  await screen.findByText("capture log");
  await act(async()=>finish(logs("stale log")));
  expect(screen.queryByText("stale log")).toBeNull();
});

it("shows read errors and lets the user retry",async()=>{
  native.read.mockRejectedValueOnce(Error("cannot read log")).mockResolvedValue(logs("recovered log"));
  render(<Diagnostics moduleId="wintray"/>);const user=userEvent.setup();
  expect(await screen.findByRole("alert")).toBeTruthy();
  await user.click(screen.getByRole("button",{name:"刷新日志"}));
  await screen.findByText("recovered log");
});

it.each([true,false])("shows missing logs in English without offering a nonexistent folder (empty list: %s)",async empty=>{
  await setApplicationLanguage("en-US");
  native.read.mockResolvedValue({folderAvailable:false,entries:empty?[]:[{name:"process",available:false,truncated:false,text:""}]});
  render(<Diagnostics moduleId="host"/>);
  await screen.findByText("No log entries yet");
  expect((screen.getByRole("button",{name:"Open log folder"}) as HTMLButtonElement).disabled).toBe(true);
});
