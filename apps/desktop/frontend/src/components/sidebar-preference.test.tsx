// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AppearanceSettings} from "./appearance-settings";
import {readSidebarLayout} from "./sidebar-preference";

afterEach(()=>{cleanup();localStorage.clear();});

it("starts collapsed even when an older session was left expanded",()=>{
 localStorage.setItem("winone.sidebar.v1",JSON.stringify({width:310,expandedWidth:310,collapsed:false}));
 expect(readSidebarLayout()).toEqual({width:64,expandedWidth:310,collapsed:true});
});

it("saves the chosen startup state separately from the last session width",async()=>{
 localStorage.setItem("winone.sidebar.v1",JSON.stringify({width:64,expandedWidth:310,collapsed:true}));
 render(<AppearanceSettings theme="system" language="zh-CN" disabled={false} onThemeChange={vi.fn()} onLanguageChange={vi.fn()}/>);
 await userEvent.setup().click(screen.getByRole("radio",{name:"默认展开"}));
 expect(readSidebarLayout()).toEqual({width:310,expandedWidth:310,collapsed:false});
 cleanup();
 render(<AppearanceSettings theme="system" language="zh-CN" disabled={false} onThemeChange={vi.fn()} onLanguageChange={vi.fn()}/>);
 expect(screen.getByRole("radio",{name:"默认展开"}).getAttribute("aria-checked")).toBe("true");
 await userEvent.setup().click(screen.getByRole("radio",{name:"默认收起"}));
 expect(readSidebarLayout()).toEqual({width:64,expandedWidth:310,collapsed:true});
});

it("uses a safe width when local preferences are malformed",()=>{
 localStorage.setItem("winone.sidebar.v1",JSON.stringify({width:64,expandedWidth:"bad"}));
 expect(readSidebarLayout()).toEqual({width:64,expandedWidth:256,collapsed:true});
});
