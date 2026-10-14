import {test,expect} from 'bun:test';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {createScriptBroker} from './scheduler-capabilities.mjs';

test('script capability discovery distinguishes grants from unconfigured connections',async()=>{
 const broker=createScriptBroker({cwd:process.cwd(),dispatch:()=>{throw Error('must not dispatch')}});
 const result=await broker.call({method:'host.capabilities'},new Set(['jira.read']),{schedule:{}});
 expect(result.methods.find(item=>item.method==='jira.get')).toMatchObject({configured:false,available:false});
 await expect(broker.call({method:'jira.get',params:{issue_key:'EX-1'}},new Set(),{schedule:{}})).rejects.toThrow('Capability not granted');
 await expect(broker.call({method:'jira.get',params:{issue_key:'EX-1'}},new Set(['jira.read']),{schedule:{}})).rejects.toThrow('no MCP method mapping');
 await broker.close();
});
test('MCP mappings execute the configured tool, preserve paging and save only within the project',async()=>{
 const directory=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'winone-capabilities-')),calls=[];
 const broker=createScriptBroker({cwd:directory,mcpServers:[{name:'jira',url:'http://127.0.0.1'}],settings:{methods:{'jira.search_jql':{server:'jira',tool:'jira_issues',arguments:{action:'search_jql'},parameterMap:{jql:'query'}}}},connect:async()=>({callTool:async(name,args)=>{calls.push({name,args});return {structuredContent:{issues:['EX-1'],next_page_token:'page2'}};},close:async()=>{}})});
 try{
  const result=await broker.call({method:'jira.search_jql',params:{jql:'project = EX',max_results:50,out_file:'result.json'}},new Set(['jira.read']),{schedule:{}});
  expect(result).toEqual({saved_to:'result.json'});expect(calls[0]).toEqual({name:'jira_issues',args:{query:'project = EX',max_results:50,action:'search_jql'}});
  expect(JSON.parse(await fs.readFile(path.join(directory,'result.json'),'utf8')).next_page_token).toBe('page2');
  await expect(broker.call({method:'jira.search_jql',params:{jql:'x',out_file:'../outside.json'}},new Set(['jira.read']),{schedule:{}})).rejects.toThrow('within the project');
 }finally{await broker.close();}
});
test('Jira and Feishu methods and notification channels keep their grants and arguments',async()=>{
 const calls=[];const methods=['jira.get','jira.search_jql','jira.add_comment','feishu.recent_chats','feishu.recent_messages','notify.feishu','notify.wecom'];
 const broker=createScriptBroker({cwd:process.cwd(),mcpServers:[{name:'fixture',url:'http://127.0.0.1'}],settings:{methods:Object.fromEntries(methods.map(method=>[method,{server:'fixture',tool:method}]))},connect:async()=>({callTool:async(method,args)=>{calls.push({method,args});return {structuredContent:{ok:true}};},close:async()=>{}})});
 try{
  const context={schedule:{}},granted=new Set(['jira.read','jira.comment','feishu.read']);
  await broker.call({method:'jira.get',params:{issue_key:'EX-1',fields:['summary']}},granted,context);
  await broker.call({method:'jira.search_jql',params:{jql:'project=EX',max_results:100,next_page_token:'page2'}},granted,context);
  await broker.call({method:'jira.add_comment',params:{issue_key:'EX-1',body_adf:{type:'doc',version:1,content:[]}}},granted,context);
  await broker.call({method:'feishu.recent_chats',params:{count:50}},granted,context);
  await broker.call({method:'feishu.recent_messages',params:{chat_id:'chat',count:20,start_time:'2026-10-10T00:00:00Z'}},granted,context);
  await broker.notify('feishu','Local test notification');await broker.notify('wecom','Local test notification');
  expect(calls.map(call=>call.method)).toEqual(methods);expect(calls[2].args.body_adf.type).toBe('doc');expect(calls[4].args.start_time).toBe('2026-10-10T00:00:00Z');
  await expect(broker.call({method:'jira.add_comment',params:{issue_key:'EX-1',body_text:'x',body_adf:{}}},granted,context)).rejects.toThrow('exactly one');
  await expect(broker.call({method:'feishu.recent_chats',params:{count:51}},granted,context)).rejects.toThrow('1–50');
 }finally{await broker.close();}
});
