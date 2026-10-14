// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {ThinkingCard} from "./cindy/ThinkingCard";
import {tokenizeThinkingText} from "./cindy/thinking-text";
import fixtures from "../../../../../tests/chat/thinking-text.json";
afterEach(cleanup);
it("uses Cindy's compact text rules shared with the native result window",()=>{for(const f of fixtures)expect(tokenizeThinkingText(f.input)).toEqual(f.tokens);});
it("starts collapsed and keeps a manual expansion through completion",async()=>{
 const user=userEvent.setup();const view=render(<ThinkingCard content="**检查** [链接](https://example.com)" isStreaming startedAt={Date.now()} blockKey="test-running"/>);
 expect(screen.queryByText("检查")).toBeNull();await user.click(screen.getByRole("button"));
 await screen.findByText("检查");expect(screen.queryByRole("link")).toBeNull();
 view.rerender(<ThinkingCard content="**检查** [链接](https://example.com)" durationMs={5000} blockKey="test-running"/>);
 await waitFor(()=>expect(screen.getByRole("button").getAttribute("aria-expanded")).toBe("true"));expect(screen.getByText("已思考 5s")).toBeTruthy();
});
