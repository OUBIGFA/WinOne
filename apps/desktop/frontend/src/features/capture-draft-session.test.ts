import {expect,it,vi} from "vitest";
import {DraftSession,documentKey} from "./capture-draft-session";
import {createHistory} from "./capture-editor-model";

it("treats native default fields as the same saved annotation",()=>{
 const document={...createHistory(100,80).document,marks:[{kind:"rectangle" as const,x:1,y:2,width:30,height:20,color:"#ff0000",strokeWidth:3}]};
 const native={...document,marks:document.marks.map(mark=>({...mark,text:"",fontSize:24,effectSize:12,points:[]}))};
 expect(documentKey(native)).toBe(documentKey(document));
});

it("serializes revisions and coalesces changes while a write is in flight",async()=>{
 const initial=createHistory(100,80).document;
 let release!:()=>void;
 const pending=new Promise<void>(resolve=>{release=resolve;});
 const revisions:number[]=[];
 const documents:unknown[]=[];
 const session=new DraftSession("a",0,initial,{update:async(revision,document)=>{revisions.push(revision);documents.push(document);if(revisions.length===1)await pending;return {draftId:"a",revision:revision+1};},read:async()=>({revision:0,document:initial})});
 const first={...initial,crop:{x:0,y:0,width:50,height:40},version:2 as const};
 const latest={...first,crop:{x:1,y:1,width:40,height:30}};
 const writes=[session.save(first)];
 await vi.waitFor(()=>expect(revisions).toEqual([0]));
 writes.push(session.save(first),session.save(latest));release();
 await Promise.all(writes);
 expect(revisions).toEqual([0,1]);expect(documents).toEqual([first,latest]);expect(session.revision).toBe(2);
});

it("reconciles an unconfirmed write but refuses another writer's changed document",async()=>{
 const document=createHistory(100,80).document;
 const changed={...document,crop:{x:1,y:1,width:20,height:20},version:2 as const};
 let remote={revision:0,document};
 let fail=true;
 const session=new DraftSession("a",0,document,{update:async(revision,value)=>{remote={revision:revision+1,document:value};if(fail){fail=false;throw Error("connection lost");}return {draftId:"a",revision:remote.revision};},read:async()=>remote});
 await expect(session.save(changed)).rejects.toThrow();
 await session.recover();expect(session.revision).toBe(1);await session.save(changed);
 const other=new DraftSession("a",0,document,{update:async()=>{throw Error("write failed");},read:async()=>remote});
 await expect(other.save({...changed,crop:{x:2,y:2,width:10,height:10}})).rejects.toThrow();
 await expect(other.recover()).rejects.toThrow();
});
