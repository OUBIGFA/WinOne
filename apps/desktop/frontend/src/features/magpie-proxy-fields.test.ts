import {expect,it} from "vitest";
import {proxyFields,proxyAddress,proxyDisplay} from "./magpie-proxy-fields";

it("splits a pasted proxy login and round-trips encoded credentials unchanged",()=>{
 const value="socks5://na%40me:p%3Ass@127.0.0.1:1080",fields=proxyFields(value);
 expect(fields.address).toBe("socks5://127.0.0.1:1080");
 expect(fields.username).toBe("na@me");expect(fields.password).toBe("p:ss");
 expect(proxyAddress(fields.address,fields,fields.username,fields.password)).toBe(value);
 expect(proxyDisplay(value)).toBe(fields.address);
});
it("preserves non-UTF8 escapes until their field is edited",()=>{
 const fields=proxyFields("http://user:%FF@proxy.invalid:8080");
 expect(proxyAddress(fields.address,fields,fields.username,fields.password)).toBe("http://user:%FF@proxy.invalid:8080");
 expect(proxyAddress(fields.address,fields,"new@user","new pass")).toBe("http://new%40user:new%20pass@proxy.invalid:8080");
});
