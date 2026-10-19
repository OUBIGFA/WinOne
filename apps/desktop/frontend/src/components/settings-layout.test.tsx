// @vitest-environment jsdom
import {afterEach, expect, it, vi} from "vitest";
import {cleanup, render, screen, within} from "@testing-library/react";
import {useState} from "react";
import userEvent from "@testing-library/user-event";
import {SettingsSection, SettingsDisclosure, SettingsOptions, SettingsToggle} from "./settings-layout";
afterEach(cleanup);

it("gives a group a real heading and an accessible name without a second card", () => {
  render(<SettingsSection title="保存位置" description="设置截图的保存位置。" level={2}><input aria-label="目录"/></SettingsSection>);
  const region = screen.getByRole("region", {name: "保存位置"});
  expect(region.contains(screen.getByRole("heading", {name: "保存位置", level: 2}))).toBe(true);
  expect(region.contains(screen.getByRole("textbox", {name: "目录"}))).toBe(true);
});

it("keeps a disclosure draft mounted across collapse and parent renders", async () => {
  const user = userEvent.setup();
  const content = <input aria-label="高级参数" defaultValue="saved"/>;
  const view = render(<SettingsDisclosure title="请求限制">{content}</SettingsDisclosure>);
  const summary = screen.getByText("请求限制", {selector: "summary"});
  const details = summary.parentElement as HTMLDetailsElement;
  expect(details.open).toBe(false);
  await user.click(summary);
  const input = screen.getByLabelText("高级参数"); await user.type(input, " draft");
  await user.click(summary); expect(details.open).toBe(false);
  view.rerender(<SettingsDisclosure title="请求限制" defaultOpen>{content}</SettingsDisclosure>);
  expect(details.open).toBe(false); // defaultOpen is not a command to reopen.
  await user.click(summary);
  expect(screen.getByLabelText("高级参数")).toBe(input);
  expect((input as HTMLInputElement).value).toBe("saved draft");
});

it.each(["segmented", "list"] as const)("keeps %s choices exclusive, keyboard accessible and fully labelled", async presentation => {
  function Form() {
    const [value, setValue] = useState("");
    return <><SettingsOptions label="处理方式" value={value} onChange={setValue} presentation={presentation} items={[
      {value: "", label: "自动"}, {value: "local", label: "本地", description: "不发送到在线服务。"},
      {value: "remote", label: "在线", disabled: true},
    ]}/><button>下一项</button></>;
  }
  render(<Form/>);
  const group = screen.getByRole("radiogroup", {name: "处理方式"});
  const automatic = within(group).getByRole("radio", {name: "自动"});
  const local = within(group).getByRole("radio", {name: "本地"});
  expect(automatic.getAttribute("aria-checked")).toBe("true");
  expect(document.getElementById(local.getAttribute("aria-describedby")!)?.textContent).toBe("不发送到在线服务。");
  automatic.focus(); const user = userEvent.setup();
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(local);
  expect(local.getAttribute("aria-checked")).toBe("true");
  expect(automatic.getAttribute("aria-checked")).toBe("false");
  await user.tab(); expect(document.activeElement).toBe(screen.getByRole("button", {name: "下一项"}));
  await user.click(within(group).getByText("在线"));
  expect(local.getAttribute("aria-checked")).toBe("true");
  await user.click(within(group).getByText("自动"));
  expect(automatic.getAttribute("aria-checked")).toBe("true");
});

it("does not announce help as part of a switch name and preserves its disabled value", async () => {
  const changed = vi.fn();
  const view = render(<SettingsToggle label="登录时启动" description="下次登录时生效。" checked={false} onChange={changed}/>);
  const control = screen.getByRole("switch", {name: "登录时启动"});
  expect(document.getElementById(control.getAttribute("aria-describedby")!)?.textContent).toBe("下次登录时生效。");
  await userEvent.setup().click(screen.getByText("登录时启动")); expect(changed).toHaveBeenCalledExactlyOnceWith(true);
  view.rerender(<SettingsToggle label="登录时启动" description="下次登录时生效。" disabled checked={false} onChange={changed}/>);
  await userEvent.setup().click(screen.getByText("登录时启动")); expect(changed).toHaveBeenCalledTimes(1);
});

it("starts configured advanced settings expanded", () => {
  render(<SettingsDisclosure title="请求限制" defaultOpen><input aria-label="并发上限" defaultValue="3"/></SettingsDisclosure>);
  expect((screen.getByText("请求限制", {selector: "summary"}).parentElement as HTMLDetailsElement).open).toBe(true);
});
