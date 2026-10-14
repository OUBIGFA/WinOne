// @vitest-environment jsdom
import {cleanup,render,screen} from "@testing-library/react";
import {afterEach,expect,it} from "vitest";
import {AIModelBadge} from "./ai-model-badge";

afterEach(cleanup);

it("retains free, discounted rate and large-context labels together",()=>{
 render(<AIModelBadge provider={{modelNames:{m:"示例模型（免费）"},modelMetadata:{m:{rate:.5,rateWas:2,listedContextWindow:1000000}}}} id="m"/>);
 expect(screen.getByText("免费")).toBeTruthy();expect(screen.getByText("0.5×")).toBeTruthy();expect(screen.getByText("2×").tagName).toBe("S");expect(screen.getByText("1M")).toBeTruthy();
});

it("does not duplicate an advertised context or invent free pricing",()=>{
 const {container}=render(<AIModelBadge provider={{modelNames:{m:"Freedom (1M)"},modelMetadata:{m:{listedContextWindow:1000000}}}} id="m"/>);
 expect(container.textContent).toBe("");
});

it("describes a zero-rate discount as free instead of a small positive charge",()=>{
 render(<AIModelBadge provider={{modelFree:{m:true},modelMetadata:{m:{rate:0,rateWas:2}}}} id="m"/>);
 expect(screen.getByTitle(/当前免费/)).toBeTruthy();
});
