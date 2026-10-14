// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import {AIProviderIcon} from "./ai-provider-icon";
import * as avatars from "./provider-avatar-cache";
const call=vi.hoisted(()=>vi.fn());vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});

it("keeps the custom picture and replaces its immutable reference instead of showing brand artwork",async()=>{
 call.mockImplementation(async(_method,args)=>({data:btoa(args.icon.endsWith("1.png")?"new picture":"old picture"),size:11,mimeType:"image/png"}));
 const view=render(<AIProviderIcon icon="file:abcdef0123456780.png"/>);await waitFor(()=>expect(view.container.querySelector("img")?.getAttribute("src")).toBe("data:image/png;base64,"+btoa("old picture")));
 view.rerender(<AIProviderIcon icon="file:abcdef0123456781.png"/>);await waitFor(()=>expect(view.container.querySelector("img")?.getAttribute("src")).toBe("data:image/png;base64,"+btoa("new picture")));
 expect(view.container.querySelector("img")?.className).not.toContain("grayscale");expect(view.container.querySelector("img")?.className).toContain("object-contain");
});

it("retries a failed picture read when the user refreshes without remounting the icon",async()=>{
 call.mockRejectedValueOnce(Error("temporary read failure")).mockResolvedValue({data:btoa("good image"),size:10,mimeType:"image/png"});
 const view=render(<AIProviderIcon icon="file:abcdef0123456790.png"/>);await waitFor(()=>expect(call).toHaveBeenCalledOnce());await waitFor(()=>expect(view.container.querySelector("[title]")).toBeTruthy());
 act(()=>avatars.refreshProviderAvatars());await waitFor(()=>expect(view.container.querySelector("img")?.getAttribute("src")).toBe("data:image/png;base64,"+btoa("good image")));
});
