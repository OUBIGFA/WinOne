// @vitest-environment jsdom
import {afterEach, expect, it, vi} from "vitest";
import {cleanup, render, screen, within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AppearanceSettings} from "./appearance-settings";

afterEach(cleanup);
it("sends theme and language choices to their own save actions", async () => {
  const onThemeChange=vi.fn(), onLanguageChange=vi.fn();
  render(<AppearanceSettings theme="system" language="zh-CN" disabled={false} onThemeChange={onThemeChange} onLanguageChange={onLanguageChange}/>);
  const user=userEvent.setup();
  const groups=screen.getAllByRole("radiogroup");
  const theme=within(groups[0]).getAllByRole("radio");
  await user.click(theme[1]);
  expect(onThemeChange).toHaveBeenLastCalledWith("light");
  expect(onLanguageChange).not.toHaveBeenCalled();
  await user.click(screen.getByRole("radio", {name:"English"}));
  expect(onLanguageChange).toHaveBeenLastCalledWith("en-US");
});
it("blocks changes while preferences are loading or being saved", async () => {
  const change=vi.fn();
  render(<AppearanceSettings theme="dark" language="en-US" disabled onThemeChange={change} onLanguageChange={change}/>);
  await userEvent.setup().click(screen.getByRole("radio", {name:"简体中文"}));
  expect(change).not.toHaveBeenCalled();
});
