import html from "../index.html?raw";
import {expect,it} from "vitest";

it("allows decoded local screenshot images without widening script origins",()=>{
 const policy=html.match(/http-equiv="Content-Security-Policy"\s+content="([^"]+)"/)?.[1];
 expect(policy).toBeTruthy();
 const directives=new Map(policy!.split(";").map(part=>{const [name,...values]=part.trim().split(/\s+/);return [name,values];}));
 expect(directives.get("img-src")).toContain("blob:");
 // PDF.js 6 decodes fonts/images with bundled Wasm. This keyword permits
 // Wasm compilation only; inline scripts and JavaScript eval remain blocked.
 expect(directives.get("script-src")).toEqual(["'self'","'wasm-unsafe-eval'"]);
 expect(directives.get("object-src")).toEqual(["'none'"]);
 // Routing uses loopback; rich answers may embed the explicitly supported YouTube
 // player. Other remote frames remain blocked by the settings shell.
 expect(directives.get("frame-src")).toEqual(["'self'","http://127.0.0.1:*","https://www.youtube.com"]);
 expect(directives.get("connect-src")).toContain("http://127.0.0.1:*");
});
