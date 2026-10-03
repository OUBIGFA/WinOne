import { afterEach, expect, it } from "vitest";
import { i18n, setApplicationLanguage } from "./i18n";
afterEach(async()=>{await setApplicationLanguage("zh-CN");});
it("uses the Host locale for system language and switches local resources",async()=>{
  await setApplicationLanguage("system","en-US");
  expect(i18n.t("nav.capture")).toBe("Capture");
  expect(i18n.t("aot.pinned",{count:1})).toBe("1 window pinned");
  expect(i18n.t("aot.pinned",{count:2})).toBe("2 windows pinned");
  await setApplicationLanguage("zh-CN","en-US");
  expect(i18n.t("nav.capture")).toBe("截图");
});
