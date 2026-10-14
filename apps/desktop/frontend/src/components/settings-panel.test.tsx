// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {SettingsPanel} from "./settings-panel";
const api=vi.hoisted(()=>({readModuleLogs:vi.fn(),openModuleLogFolder:vi.fn()}));
vi.mock("../bridge",()=>api);
vi.mock("../features/updates",()=>({Updates:()=> <p>Update preferences</p>}));
afterEach(()=>{cleanup();vi.clearAllMocks();});
it("keeps general settings and the final diagnostics group independent of AI services",async()=>{
 api.readModuleLogs.mockResolvedValue({folderAvailable:true,entries:[]});
 render(<SettingsPanel><input aria-label="General preference" defaultValue="saved"/></SettingsPanel>);
 expect(screen.getAllByRole("tab").map(tab=>tab.textContent)).toEqual(["诊断日志"]);
 expect(api.readModuleLogs).not.toHaveBeenCalled();
 await userEvent.type(screen.getByLabelText("General preference")," draft");
 await userEvent.click(screen.getByRole("tab",{name:"诊断日志"}));
 expect((screen.getByLabelText("General preference") as HTMLInputElement).value).toBe("saved draft");
 expect(screen.getByRole("button",{name:"刷新日志"})).toBeTruthy();
});
