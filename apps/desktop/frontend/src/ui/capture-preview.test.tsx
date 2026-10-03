// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CapturePreview } from "./capture-preview";
afterEach(cleanup);

it("uses an image identity URL and exposes load failures",()=>{
  const refresh=vi.fn();
  render(<CapturePreview id={"a".repeat(32)} ready width={2000} height={1000} onRefresh={refresh}/>);
  const image=screen.getByRole("img",{name:"最近截图预览"});
  expect(image.getAttribute("src")).toBe(`/capture-preview/${"a".repeat(32)}.png`);
  fireEvent.error(image);
  expect(screen.getByText("截图加载失败")).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"重新加载截图"}));
  expect(refresh).toHaveBeenCalledOnce();
});
