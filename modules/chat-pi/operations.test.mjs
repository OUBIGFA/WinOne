import {test} from "node:test";
import assert from "node:assert/strict";
import {createOperationRecorder,readOperations} from "./operations.mjs";

test("operations retain a stable identity and monotonic elapsed time across reopen",()=>{
 const branch=[],events=[];let wall=1000,clock=200;
 const recorder=createOperationRecorder({write:data=>branch.push({type:"custom",customType:"winone.operation",data}),emit:data=>events.push(data),now:()=>wall,clock:()=>clock});
 recorder.begin("compact","compaction","turn-one");wall=800;clock=250;recorder.end("compact","completed");
 assert.equal(events.length,2);assert.equal(events[0].id,events[1].id);
 const saved=readOperations(branch,new Set(["turn-one"]));assert.equal(saved.length,1);assert.equal(saved[0].startedAt,1000);assert.equal(saved[0].durationMs,50);assert.equal(saved[0].status,"completed");
});
test("reopened unfinished work is stopped without inventing a duration or exposing other turns",()=>{
 const branch=[];const recorder=createOperationRecorder({write:data=>branch.push({type:"custom",customType:"winone.operation",data}),emit:()=>{}});
 recorder.begin("retry","retry","one",{attempt:1,maxAttempts:2,delayMs:500});recorder.begin("review","review","two");
 const saved=readOperations(branch,new Set(["one"]));assert.equal(saved.length,1);assert.equal(saved[0].status,"stopped");assert.equal(saved[0].durationMs,undefined);
 assert.equal(readOperations(branch,new Set(["one"]),"one")[0].status,"running");
});
