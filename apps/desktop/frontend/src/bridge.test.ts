import { describe, expect, it } from "vitest";
import { getSnapshot } from "./bridge";

describe("desktop bridge", () => {
  it("reports missing native bridge instead of fabricating an online state", async () => {
    await expect(getSnapshot()).rejects.toThrow("请从 WinOne 桌面程序打开");
  });
});
