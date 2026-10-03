// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,renderHook,waitFor} from "@testing-library/react";
import {useSettingsNavigation} from "./settings-navigation";

afterEach(cleanup);
it("receives startup and repeated tray requests once through the page guard",async()=>{
  let sequence=1,notify=()=>{};
  const unsubscribe=vi.fn();
  const port={read:async()=>sequence,subscribe:(listener:()=>void)=>{notify=listener;return unsubscribe;}};
  const navigate=vi.fn(),error=vi.fn();
  const hook=renderHook(()=>useSettingsNavigation(port,navigate,error));
  await waitFor(()=>expect(navigate).toHaveBeenCalledWith("settings"));
  await act(async()=>notify());
  expect(navigate).toHaveBeenCalledTimes(1);
  sequence=2;
  await act(async()=>notify());
  expect(navigate).toHaveBeenCalledTimes(2);
  hook.unmount();expect(unsubscribe).toHaveBeenCalledOnce();
});
it("reports bridge failure instead of pretending navigation succeeded",async()=>{
  const error=vi.fn(),navigate=vi.fn();
  const port={read:async()=>{throw Error("unavailable");},subscribe:()=>()=>{}};
  renderHook(()=>useSettingsNavigation(port,navigate,error));
  await waitFor(()=>expect(error).toHaveBeenCalledWith("Error: unavailable"));
  expect(navigate).not.toHaveBeenCalled();
});
