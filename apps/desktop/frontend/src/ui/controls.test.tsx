// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";
import { Switch } from "./switch";

afterEach(cleanup);

it("keeps a busy button named and prevents duplicate activation", async () => {
  const action = vi.fn();
  render(<Button busy onClick={action}>保存设置</Button>);
  const button = screen.getByRole("button", { name: "保存设置" });
  await userEvent.setup().click(button);
  expect(button.getAttribute("aria-busy")).toBe("true");
  expect(action).not.toHaveBeenCalled();
});

it("allows keyboard operation of a named switch", async () => {
  const changed = vi.fn();
  render(<Switch aria-label="播放提示音" onCheckedChange={changed} />);
  const user = userEvent.setup();
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("switch", { name: "播放提示音" }));
  await user.keyboard(" ");
  expect(changed.mock.calls[0][0]).toBe(true);
});

it("allows pointer operation of a named switch", async () => {
  const changed = vi.fn();
  render(<Switch aria-label="播放提示音" onCheckedChange={changed} />);
  await userEvent.setup().click(screen.getByRole("switch", { name: "播放提示音" }));
  expect(changed.mock.calls[0][0]).toBe(true);
});

it("respects a disabled settings fieldset", async () => {
  const changed = vi.fn();
  render(<fieldset disabled><Switch aria-label="播放提示音" onCheckedChange={changed} /></fieldset>);
  await userEvent.setup().click(screen.getByRole("switch", { name: "播放提示音" }));
  expect(changed).not.toHaveBeenCalled();
});
