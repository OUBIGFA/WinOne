// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {RepositoryLink} from "./repository-link";

const open=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({openRepository:open}));
afterEach(()=>{cleanup();open.mockReset();});
it("labels the sample repository and requests the system browser",async()=>{
 open.mockResolvedValue(undefined);render(<RepositoryLink/>);
 expect(open).not.toHaveBeenCalled();
 await userEvent.setup().click(screen.getByRole("button",{name:"GitHub 仓库（示例）"}));
 expect(open).toHaveBeenCalledOnce();
});
it("reports a failed browser request",async()=>{
 open.mockRejectedValue(Error("Browser unavailable"));render(<RepositoryLink/>);
 await userEvent.setup().click(screen.getByRole("button",{name:"GitHub 仓库（示例）"}));
 expect((await screen.findByRole("alert")).textContent).toBe("Error: Browser unavailable");
});
