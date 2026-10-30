// UI-only fixture. Protocol and filesystem behavior are exercised by the real worker tests.
export function interconnectFixture(){
 const tasks:any[]=[];
 let general={notify:true,clipboard:false,downloadDir:'C:/isolated/received'},menu={enabled:false},entry={visible:false};
 let chat={nickname:''};
 let running=false;
 let settings={bind:'127.0.0.1',port:5421,downloadDir:'C:\\isolated\\received'};
 let items:{id:string;kind:string;title:string;content?:string;size:number;exists:boolean}[]=[];
 return async(method:string,payload:string)=>{
  const params=JSON.parse(payload);
  if(method==='pending')return JSON.stringify({requested:false,paths:[]});
  if(method==='preferences.save'){settings=params;return JSON.stringify(settings);}
  if(method==='chat.preferences.get')return JSON.stringify(chat);
  if(method==='chat.preferences.save'){chat=params;return JSON.stringify(chat);}
  if(method==='transfer_open_cloud')return 'null';
  if(method==='general.get')return JSON.stringify(general);
  if(method==='general.save'){general=params;return JSON.stringify(general);}
  if(method==='menu.get')return JSON.stringify(menu);
  if(method==='menu.set'){menu=params;return JSON.stringify(menu);}
  if(method==='entry.get')return JSON.stringify(entry);
  if(method==='entry.show'||method==='entry.hide'){entry={visible:method==='entry.show'};return JSON.stringify(entry);}
  if(method==='transfer_stop'){const task=tasks.find(task=>task.id===params.id);if(task){task.state.status='cancelled';task.state.ready=false;}return 'null';}
  if(method==='transfer_restart'){const task=tasks.find(task=>task.id===params.id);if(task){task.state.status='running';task.state.ready=true;}return JSON.stringify({id:params.id});}
  if(method==='share_add'){const task=tasks.find(task=>task.id===params.id);task?.state.share.items.push(...params.paths.map((path:string,index:number)=>({id:'added-'+index,title:path,size:123456,exists:true})));return 'null';}
  if(method==='share_add_text'){const task=tasks.find(task=>task.id===params.id);task?.state.share.items.push({id:'text-'+task.state.share.items.length,kind:'text',title:params.text.slice(0,40),content:params.text,size:params.text.length,exists:true});return 'null';}
  if(method==='share_code_stop'||method==='share_code_restart'){const task=tasks.find(task=>task.id===params.id);if(task?.state.transfer)task.state.transfer.status=method==='share_code_stop'?'cancelled':'running';return 'null';}
  if(method==='share_remove'){const task=tasks.find(task=>task.id===params.id);if(task?.state.share)task.state.share.items=task.state.share.items.filter((item:any)=>item.id!==params.itemId);return 'null';}
  if(method==='transfer_start'){
   const id='task-'+tasks.length,stored=params.mode.startsWith('store_');
   if(params.mode.startsWith('share_')||params.mode.startsWith('tunnel_')){tasks.unshift({id,spec:params,state:{status:'running',ready:true,relayReady:true,code:'tc'+('verified-fixture-address-'.repeat(6)),active:1,sent:4567,received:8910,transfer:params.mode==='share_server'?{status:'running',code:'1234-fixture-code-phrase',browserUrl:'https://getcroc.com/?code=1234-fixture-code-phrase',qr:'data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"></svg>')}:undefined,share:params.mode==='share_server'?{info:{url:'http://192.168.1.50:5421/?key=ui-fixture',qr:'<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"></svg>',addresses:[]},items:[...params.paths.map((path:string,index:number)=>({id:'file-'+index,kind:'file',title:path.split(/[\\/]/).pop()??path,size:123456,exists:true})),...(params.text?[{id:'text-0',kind:'text',title:params.text.slice(0,40),content:params.text,size:params.text.length,exists:true}]:[])],downloads:[]}:undefined}});return JSON.stringify({id});}
   const cloud=params.storeOrigin==='https://gofile.io';
   const qr='data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256"></svg>');
   const result=params.mode==='store_send'?{id:'fixture-share',service:cloud?'gofile':undefined,code:cloud?undefined:'croc-store-v1.preview-capability',link:cloud?'https://gofile.io/d/Ab12Cd34':'https://example.invalid/s/fixture#test-key',qr,expiresAt:new Date(Date.now()+86400000).toISOString(),downloads:cloud?undefined:2}:params.mode==='store_inspect'?{manifest:{f:[{n:'received.txt',s:2048}]}}:undefined;
   tasks.unshift({id,created:'2026-10-24T12:00:00Z',spec:params,pendingUntil:params.mode==='store_send'?Date.now()+400:undefined,state:{status:params.mode==='store_send'?'running':'completed',code:stored?undefined:'1234-fixture-code-phrase',result}});
   return JSON.stringify({id});
  }
  if(method==='transfer_list'){for(const task of tasks)if(task.pendingUntil&&Date.now()>=task.pendingUntil){task.pendingUntil=undefined;task.state.status='completed';}return JSON.stringify(tasks);}
  if(method==='transfer_remove'){const index=tasks.findIndex(task=>task.id===params.id);if(index>=0)tasks.splice(index,1);return 'null';}
  if(method==='share_revoke'){const task=tasks.find(task=>task.id===params.id);if(task?.state.result)task.state.result.revoked=true;return 'null';}
  if(method==='start'){running=true;settings=params;}
  if(method==='stop')running=false;
  if(method==='add_files')items.push(...params.map((path:string,index:number)=>({id:'file-'+index,kind:'file',title:path.split(/[\\/]/).pop()??path,size:12345678,exists:true})));
  if(method==='add_text')items.push({id:'text-'+items.length,kind:'text',title:params,content:params,size:params.length,exists:true});
  if(method==='remove')items=items.filter(item=>item.id!==params);
  if(method==='download_name')return JSON.stringify('received.txt');
  if(method==='get')return JSON.stringify({running,settings,addresses:[{name:'Ethernet',ip:'192.168.1.50'}],snapshot:running?{info:{url:'http://127.0.0.1:5421/?key=ui-fixture',qr:'',addresses:[]},items,downloads:[]}:null});
  return JSON.stringify(null);
 };
}
