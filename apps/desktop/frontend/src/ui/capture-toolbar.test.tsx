// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CaptureToolbar } from "./capture-toolbar";
afterEach(cleanup);

it("routes image actions and skips unavailable tools during keyboard navigation",async()=>{
  const action=vi.fn();
  render(<CaptureToolbar onAction={action} busy={false} canEdit={false} hasPins={false}/>);
  const user=userEvent.setup();
  screen.getByRole("button",{name:"复制"}).focus();
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(screen.getByRole("button",{name:"另存为"}));
  await user.keyboard("{End}");
  expect(document.activeElement).toBe(screen.getByRole("button",{name:"截图置顶"}));
  await user.click(screen.getByRole("button",{name:"另存为"}));
  expect(action).toHaveBeenCalledWith("save");
});

it("blocks all new actions while an image operation is running",()=>{
  render(<CaptureToolbar onAction={()=>{}} busy canEdit hasPins/>);
  expect((screen.getByRole("button",{name:"复制"}) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole("button",{name:"另存为"}) as HTMLButtonElement).disabled).toBe(true);
});
