// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CapturePage } from "./capture";

const native = vi.hoisted(() => ({
  call: vi.fn(),
  pick: vi.fn(),
  reveal: vi.fn().mockResolvedValue(undefined),
}));
vi.mock("../bridge", () => ({
  captureCall: native.call,
  pickDirectory: native.pick,
  reveal: native.reveal,
}));
afterEach(() => {
  cleanup();
  native.call.mockReset();
});

it("shows the saved image when clipboard output fails", async () => {
  let failed = false;
  native.call.mockImplementation(async (method: string) => {
    if (method === "snapshot")
      return {
        settings: {
          schemaVersion: 1,
          revision: 0,
          outputDirectory: "C:\\Images",
          copyToClipboard: true,
          askWhereToSave: false,
          openEditor: false,
          pinAfterCapture: false,
          theme: "system",
          scrollDelay: 300,
          scrollFromTop: false,
        },
        lastCapture: failed
          ? {
              path: "C:\\Images\\saved.png",
              width: 32,
              height: 24,
              copied: false,
              pinned: false,
            }
          : null,
        busy: false,
        pins: 0,
      };
    if (method === "capture.start") {
      failed = true;
      return { id: "job", state: "failed", error: "截图已保存，复制失败" };
    }
    throw new Error(method);
  });
  render(<CapturePage />);
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "区域截图" }));
  await screen.findByText("C:\\Images\\saved.png");
  expect(screen.getByRole("button", { name: "复制" })).toBeTruthy();
});
