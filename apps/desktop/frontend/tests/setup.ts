import { afterAll, beforeAll, vi } from "vitest";

// JSDOM 26 lacks PointerEvent. Base UI synthesizes keyboard and controlled
// activation clicks with modifier flags; MouseEvent supplies those test semantics.
// Production WebView2 uses its own PointerEvent implementation.
beforeAll(() => {
  if (typeof window !== "undefined" && !window.PointerEvent) vi.stubGlobal("PointerEvent", MouseEvent);
  // JSDOM has no layout engine; resizing itself is verified in the browser.
  if (typeof window !== "undefined" && !window.ResizeObserver) vi.stubGlobal("ResizeObserver", class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
});
afterAll(() => vi.unstubAllGlobals());
