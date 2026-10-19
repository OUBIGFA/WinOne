// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import {useState} from "react";
import {ModulePanel} from "./module-panel";
afterEach(cleanup);
function Draft(){const [value,setValue]=useState("");return <input aria-label="draft" value={value} onChange={event=>setValue(event.target.value)}/>;}

it("keeps the module page free of a separate shortcut block", () => {
 render(<ModulePanel moduleId="always-on-top"><Draft/></ModulePanel>);
 expect(screen.getByLabelText("draft")).toBeTruthy();
 expect(screen.queryByRole("tab",{name:"快捷键"})).toBeNull();
 expect(screen.queryByLabelText("快捷键")).toBeNull();
});

it("leaves WinTray and the gesture editor as their pages own them", () => {
 render(<ModulePanel moduleId="wintray"><Draft/></ModulePanel>);
 expect(screen.getByLabelText("draft")).toBeTruthy();
 expect(screen.queryByRole("tab",{name:"快捷键"})).toBeNull();
});
