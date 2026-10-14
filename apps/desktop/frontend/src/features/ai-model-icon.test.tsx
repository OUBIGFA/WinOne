// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import geminiImage from "../../../../result-ui/assets/provider-bitmaps/gemini-color.png?url";
import claudeImage from "../../../../result-ui/assets/provider-bitmaps/claude-color.png?url";
import {AIProviderModels} from "./ai-provider-models";
vi.mock("../bridge",()=>({captureCall:vi.fn()}));
afterEach(cleanup);
it("uses a model family icon inside a provider model list",()=>{
 render(<AIProviderModels provider={{id:"proxy",name:"代理供应商",authMode:"api-key",icon:"openrouter-color",models:["google/gemini-2.5-pro","anthropic/claude-sonnet-4"],hasKey:false}} canFetch disabled={false} onChange={()=>{}} onBusy={()=>{}}/>);
 expect(screen.getByText("google/gemini-2.5-pro").closest("label")!.querySelector("img")!.getAttribute("src")).toBe(geminiImage);
 expect(screen.getByText("anthropic/claude-sonnet-4").closest("label")!.querySelector("img")!.getAttribute("src")).toBe(claudeImage);
});
