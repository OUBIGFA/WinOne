import html from "../index.html?raw";
import {expect,it} from "vitest";

it("allows decoded local screenshot images without widening script origins",()=>{
 const policy=html.match(/http-equiv="Content-Security-Policy"\s+content="([^"]+)"/)?.[1];
 expect(policy).toBeTruthy();
 const directives=new Map(policy!.split(";").map(part=>{const [name,...values]=part.trim().split(/\s+/);return [name,values];}));
 expect(directives.get("img-src")).toContain("blob:");
 expect(directives.get("script-src")).toEqual(["'self'"]);
 expect(directives.get("object-src")).toEqual(["'none'"]);
});
