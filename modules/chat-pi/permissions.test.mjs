import {test} from "node:test";
import assert from "node:assert/strict";
import {createPermissionGate} from "./permissions.mjs";

const cwd="C:\\work\\project";
const write=(target="C:\\work\\project\\proof.txt")=>({id:"write",tool:"write",args:{path:target,content:"proof"},cwd,outside:!target.startsWith(cwd),protectedFile:false});
test("Ask retains explicit tool review and session grants cover only the displayed file",async()=>{
 const requests=[];
 const gate=createPermissionGate({mode:"ask",cwd,authorize:async request=>{requests.push(request);return {allow:true,scope:"session"};}});
 await gate.check(write());await gate.check(write());await gate.check(write(cwd+"\\other.txt"));
 assert.equal(requests.length,2);assert.equal(requests[0].scopeLabel,cwd+"\\proof.txt");
 gate.setMode("ask");await gate.check(write());assert.equal(requests.length,3);
});
test("Auto uses Cindy's static classification and asks after reviewer failure",async()=>{
 let reviews=0,approvals=0;
 const gate=createPermissionGate({mode:"auto",cwd,review:async()=>{reviews++;throw Error("unavailable");},authorize:async request=>{approvals++;assert.equal(request.autoReviewUnavailable,true);return false;}});
 await gate.check(write());assert.equal(reviews,0);
 await assert.rejects(gate.check({id:"command",tool:"powershell",args:{command:"./unknown-tool.exe"},cwd}),/not authorize/);
 assert.equal(reviews,1);assert.equal(approvals,1);
});
test("Auto block returns a reason to Pi without claiming a user rejection",async()=>{
 const gate=createPermissionGate({mode:"auto",cwd,review:async()=>({verdict:"block",reason:"Use a scoped operation"}),authorize:async()=>{assert.fail("block must not show a confirmation");}});
 await assert.rejects(gate.check({id:"cmd",tool:"powershell",args:{command:"./unknown-tool.exe"},cwd}),/Automatic review: Use a scoped operation/);
});
test("Full access permits external work and still confirms writes to WinOne control files",async()=>{
 let asked=0;const gate=createPermissionGate({mode:"bypassPermissions",cwd,authorize:async()=>{asked++;return false;}});
 await gate.check(write("C:\\outside\\proof.txt"));
 await gate.check({id:"command",tool:"powershell",args:{command:"./unknown-tool.exe"},cwd});
 assert.equal(asked,0);
 await assert.rejects(gate.check({...write(),protectedFile:true}),/not authorize/);assert.equal(asked,1);
});
