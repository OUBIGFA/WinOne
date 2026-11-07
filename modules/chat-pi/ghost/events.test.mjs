import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createGhostEvents} from './platform-events.mjs';

const root=fileURLToPath(new URL('../../../',import.meta.url));
async function fixture(t,screen=async()=>({action:'allow'})){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(root,'_temp/ghost-events-tests');
 await fs.mkdir(base,{recursive:true});const directory=await fs.mkdtemp(path.join(base,'events-'));
 const published=[],rewrites=[];
 const deps={root:directory,gateway:{publish:(topic,name,data)=>published.push({topic,name,data}),screenAssistantMessage:screen},push:(_channel,value)=>rewrites.push(value),log:{warn(){}}};
 const events=createGhostEvents(deps);t.after(()=>events.close());
 return {events,published,rewrites,deps};
}
async function until(read,check){for(let n=0;n<100;n++){const value=await read();if(check(value))return value;await new Promise(resolve=>setTimeout(resolve,10));}throw Error('Expected durable presentation did not arrive');}

test('native lifecycle closes exact thinking and interaction pairs and reports interrupted work honestly',async t=>{
 const f=await fixture(t);const send=value=>f.events.receive({sessionId:'task',model:'fixture',...value});
 send({kind:'start'});send({kind:'thinking',stage:'start',blockId:'thinking'});
 send({kind:'interaction-start',id:'permission',question:false});send({kind:'interaction-start',id:'question',question:true});
 send({kind:'interaction-end',id:'permission',question:false});send({kind:'settled',status:'stopped'});send({kind:'closed'});
 const starts=f.published.filter(row=>row.name.endsWith('-start')),ends=f.published.filter(row=>row.name.endsWith('-end'));
 assert.equal(starts.length,4);assert.equal(ends.length,4);
 assert.equal(f.published.find(row=>row.name==='did-turn-end').data.endReason,'interrupted');
 assert.equal(f.published.filter(row=>row.name==='did-user-input-end').length,1);
 assert.equal(f.rewrites.length,0);
});

test('a rewritten reply keeps its message identity after later turns and reopening',async t=>{
 let calls=0;
 const f=await fixture(t,async({text})=>{calls++;return {action:'rewrite',text:'Shown: '+text,ghostName:'Fixture'};});
 for(const [id,text] of [['a1','First answer'],['a2','Second answer']]){
  f.events.receive({sessionId:'task',kind:'start',model:'fixture'});
  f.events.receive({sessionId:'task',kind:'settled',status:'completed',model:'fixture',message:{id,text}});
 }
 const rows=await until(()=>f.events.read('task'),rows=>rows.length===2);
 assert.equal(rows.find(row=>row.messageId==='a1').text,'Shown: First answer');
 assert.equal(rows.find(row=>row.messageId==='a2').text,'Shown: Second answer');
 f.events.close();const reopened=createGhostEvents(f.deps);t.after(()=>reopened.close());
 assert.deepEqual(await reopened.read('task'),rows);
 reopened.receive({sessionId:'task',kind:'settled',status:'completed',message:{id:'a1',text:'First answer'}});
 await new Promise(resolve=>setTimeout(resolve,20));assert.equal(calls,2,'reopening must not rerun a completed rewrite');
});

test('rendered overrides are sanitized and retain a source digest for viewing original text',async t=>{
 const f=await fixture(t,async()=>({action:'render',html:'<div>Shown<script>unsafe()</script></div>',text:'Original answer',ghostName:'Fixture'}));
 f.events.receive({sessionId:'task',kind:'start'});
 f.events.receive({sessionId:'task',kind:'settled',status:'completed',message:{id:'a1',text:'Original answer'}});
 const rows=await until(()=>f.events.read('task'),rows=>rows.length===1);
 assert.doesNotMatch(rows[0].html,/<script|unsafe/);
 assert.equal(rows[0].sourceSHA256.length,64);
 assert.equal(rows[0].messageId,'a1');
});
