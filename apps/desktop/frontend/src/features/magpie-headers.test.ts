import {expect,it} from "vitest";
import {headersOf,prettyJSON} from "./magpie-headers";
it("matches Magpie header serialization without changing plain scalar values",()=>{
 expect(headersOf([["X-Team","first",false],["x-team"," last ",false],["Empty","",false],["Version","2.0",false],["X-JSON",'{\n "mode": "on"\n}',true]])).toEqual({"x-team":"last",Version:"2.0","X-JSON":'{"mode":"on"}'});
 expect(prettyJSON('{"a":1}')).toBe('{\n  "a": 1\n}');
});
