// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WinTrayPage } from "./wintray";

const native = vi.hoisted(() => ({ call: vi.fn(), pick: vi.fn() }));
vi.mock("../bridge", () => ({
  wintrayCall: native.call,
  pickProgram: native.pick,
}));
afterEach(() => {
  cleanup();
  native.call.mockReset();
});

it("keeps the user's draft and exposes recovery after a failed save", async () => {
  let failed = false;
  const settings = {
    schemaVersion: 3,
    language: "zh-CN",
    runAtLogon: false,
    startMinimizedToTray: true,
    exitAfterManagedAppsCompleted: false,
    closeWindowRetrySeconds: 10,
    startupIntervalSeconds: 3,
    managedApps: [
      {
        id: "a",
        name: "示例",
        exePath: "C:\\Apps\\example.exe",
        args: "",
        runOnStartup: true,
        collectTrayIcon: false,
        launchViaLogonTask: false,
        launchHiddenInBackground: false,
        trayBehavior: {
          autoMinimizeAndHideOnLaunch: false,
          closeDelaySeconds: 0,
        },
        schedule: {
          enabled: false,
          startDelayMinutes: 0,
          autoExitMinutes: 30,
          frequencyEnabled: false,
          frequencyDays: 1,
          frequencyRuns: 1,
        },
      },
    ],
  };
  native.call.mockImplementation(async (method: string) => {
    if (method === "snapshot")
      return {
        revision: "first",
        settings: structuredClone(settings),
        recoveryError: failed ? "需要恢复" : "",
      };
    if (method === "settings.save") {
      failed = true;
      return { id: "job", state: "failed", error: "保存失败" };
    }
    throw new Error(method);
  });
  render(<WinTrayPage />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: /^示例/ }));
  const name = screen.getByLabelText("名称") as HTMLInputElement;
  await user.clear(name);
  await user.type(name, "修改名称");
  await user.click(screen.getByRole("button", { name: "保存修改" }));
  await screen.findByRole("button", { name: "恢复上次设置" });
  expect(name.value).toBe("修改名称");
  expect(
    (screen.getByRole("button", { name: "保存修改" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});
