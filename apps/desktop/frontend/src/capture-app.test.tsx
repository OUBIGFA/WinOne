// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {CaptureApp} from "./capture-app";

const api=vi.hoisted(()=>({snapshot:vi.fn(),read:vi.fn(),ack:vi.fn(),subscribe:vi.fn(),draft:vi.fn(),open:vi.fn(),reveal:vi.fn(),close:vi.fn()}));
vi.mock("./bridge",()=>({getSnapshot:api.snapshot,captureLaunchPort:{read:api.read,acknowledge:api.ack,subscribe:api.subscribe},captureOpeningPort:{},revealCaptureEditor:api.reveal,windowFramePort:{command:api.close}}));
vi.mock("./features/capture-draft-api",()=>({readCaptureDraft:api.draft}));
vi.mock("./features/capture-surface-session",()=>({openCaptureEditor:api.open}));
vi.mock("./features/capture-draft-workspace",()=>({CaptureDraftWorkspace:({onDone}:{onDone:()=>void})=><section aria-label="Screenshot"><button onClick={onDone}>Complete</button></section>}));
beforeEach(()=>{
 vi.clearAllMocks();vi.stubGlobal("matchMedia",()=>({matches:false,addEventListener:vi.fn(),removeEventListener:vi.fn()}));
 api.subscribe.mockReturnValue(()=>{});api.reveal.mockResolvedValue(undefined);api.close.mockResolvedValue(undefined);
});
afterEach(()=>{cleanup();vi.unstubAllGlobals();});

it("loads preferences before showing only the screenshot and keeps its final frame mounted",async()=>{
 let release!:(value:unknown)=>void;
 api.snapshot.mockImplementation(()=>new Promise(resolve=>{release=resolve;}));
 api.read.mockResolvedValueOnce({id:"launch",draftId:"a".repeat(32)}).mockResolvedValue(null);
 api.ack.mockResolvedValue(true);api.draft.mockResolvedValue({id:"a".repeat(32)});api.open.mockResolvedValue({warning:""});
 const view=render(<CaptureApp/>);
 expect(view.container.childElementCount).toBe(0);expect(api.read).not.toHaveBeenCalled();
 release({settings:{theme:"dark",language:"en-US"},locale:"en-US"});
 await screen.findByRole("region",{name:"Screenshot"});
 expect(document.documentElement.dataset.theme).toBe("dark");
 expect(screen.queryByRole("navigation")).toBeNull();
 fireEvent.click(screen.getByRole("button",{name:"Complete"}));
 expect(screen.getByRole("region",{name:"Screenshot"})).toBeTruthy();
 expect(api.open).toHaveBeenCalledOnce();
});

it("shows a closeable capture error instead of falling back to settings",async()=>{
 api.snapshot.mockRejectedValue(Error("Preferences unavailable"));
 render(<CaptureApp/>);
 await screen.findByText(/Preferences unavailable/);
 expect(screen.queryByRole("navigation")).toBeNull();expect(api.read).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button"));
 await waitFor(()=>expect(api.close).toHaveBeenCalledWith("close"));
});
