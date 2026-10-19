// @vitest-environment jsdom
import {afterEach, expect, it, vi} from "vitest";
import {cleanup, render, screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {SettingsPanel} from "./settings-panel";
const api = vi.hoisted(() => ({readModuleLogs: vi.fn(), openModuleLogFolder: vi.fn()}));
vi.mock("../bridge", () => api);
vi.mock("../features/updates", () => ({Updates: () => <p>Update preferences</p>}));
afterEach(() => {cleanup(); vi.clearAllMocks();});

it("separates global tasks, retains drafts and reads diagnostics only when opened", async () => {
  api.readModuleLogs.mockResolvedValue({folderAvailable: true, entries: []});
  const user = userEvent.setup();
  render(<SettingsPanel
    startup={<input aria-label="Startup preference" defaultValue="startup"/>}
    permissions={<p>Permission controls</p>}
    profiles={<p>Import and export</p>}
  ><input aria-label="General preference" defaultValue="saved"/></SettingsPanel>);
  expect(screen.getAllByRole("tab").map(tab => tab.textContent)).toEqual(["常规", "启动与快捷键", "运行权限", "配置迁移", "关于与诊断"]);
  expect(api.readModuleLogs).not.toHaveBeenCalled();
  const general = screen.getByLabelText("General preference");
  await user.type(general, " draft");
  await user.click(screen.getByRole("tab", {name: "启动与快捷键"}));
  const startup = screen.getByLabelText("Startup preference");
  await user.type(startup, " draft");
  await user.click(screen.getByRole("tab", {name: "关于与诊断"}));
  expect(screen.getByRole("button", {name: "刷新日志"})).toBeTruthy();
  expect(api.readModuleLogs).toHaveBeenCalledOnce();
  await user.click(screen.getByRole("tab", {name: "常规"}));
  expect(screen.getByLabelText("General preference")).toBe(general);
  expect((general as HTMLInputElement).value).toBe("saved draft");
  await user.click(screen.getByRole("tab", {name: "启动与快捷键"}));
  expect(screen.getByLabelText("Startup preference")).toBe(startup);
  expect((startup as HTMLInputElement).value).toBe("startup draft");
});

it("keeps diagnostic navigation usable when the preference form is unavailable", async () => {
  api.readModuleLogs.mockResolvedValue({folderAvailable: false, entries: []});
  render(<SettingsPanel error={<div role="alert">设置保存失败，请重试。</div>}><fieldset disabled><input aria-label="Unavailable preference"/></fieldset></SettingsPanel>);
  expect(screen.getByRole("alert").textContent).toBe("设置保存失败，请重试。");
  await userEvent.click(screen.getByRole("tab", {name: "关于与诊断"}));
  expect(screen.getByRole("heading", {name: "诊断日志"})).toBeTruthy();
  expect(screen.getByRole("button", {name: "刷新日志"}).hasAttribute("disabled")).toBe(false);
  expect(screen.getByRole("alert").textContent).toBe("设置保存失败，请重试。");
});
