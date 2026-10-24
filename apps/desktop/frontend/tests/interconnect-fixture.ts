// UI-only fixture. Protocol and filesystem behavior are exercised by the real worker tests.
export function interconnectFixture(){
 const tasks:any[]=[];
 let general={notify:true,clipboard:false,downloadDir:'C:/isolated/received'},menu={enabled:false},entry={visible:false};
 let running=false;
 let settings={bind:'127.0.0.1',port:5421,downloadDir:'C:\\isolated\\received'};
 let items:{id:string;kind:string;title:string;content?:string;size:number;exists:boolean}[]=[];
 return async(method:string,payload:string)=>{
  const params=JSON.parse(payload);
  if(method==='pending')return JSON.stringify({requested:false,paths:[]});
  if(method==='general.get')return JSON.stringify(general);
  if(method==='general.save'){general=params;return JSON.stringify(general);}
  if(method==='menu.get')return JSON.stringify(menu);
  if(method==='menu.set'){menu=params;return JSON.stringify(menu);}
  if(method==='entry.get')return JSON.stringify(entry);
  if(method==='entry.show'||method==='entry.hide'){entry={visible:method==='entry.show'};return JSON.stringify(entry);}
  if(method==='transfer_stop'){const task=tasks.find(task=>task.id===params.id);if(task){task.state.status='cancelled';task.state.ready=false;}return 'null';}
  if(method==='transfer_restart'){const task=tasks.find(task=>task.id===params.id);if(task){task.state.status='running';task.state.ready=true;}return JSON.stringify({id:params.id});}
  if(method==='share_add'){const task=tasks.find(task=>task.id===params.id);task?.state.share.items.push(...params.paths.map((path:string,index:number)=>({id:'added-'+index,title:path,size:123456,exists:true})));return 'null';}
  if(method==='share_remove'){const task=tasks.find(task=>task.id===params.id);if(task?.state.share)task.state.share.items=task.state.share.items.filter((item:any)=>item.id!==params.itemId);return 'null';}
  if(method==='transfer_start'){
   const id='task-'+tasks.length,stored=params.mode.startsWith('store_');
   if(params.mode.startsWith('share_')||params.mode.startsWith('tunnel_')){tasks.unshift({id,spec:params,state:{status:'running',ready:true,code:'tc'+('verified-fixture-address-'.repeat(6)),active:1,sent:4567,received:8910,share:params.mode==='share_server'?{items:params.paths.map((path:string,index:number)=>({id:'file-'+index,title:path,size:123456,exists:true}))}:undefined}});return JSON.stringify({id});}
   const result=params.mode==='store_send'?{id:'fixture-share',code:'croc-store-v1.preview-capability',link:'https://example.invalid/s/fixture#test-key',expiresAt:'2026-10-25T12:00:00Z',downloads:2}:params.mode==='store_inspect'?{manifest:{f:[{n:'received.txt',s:2048}]}}:undefined;
   tasks.unshift({id,created:'2026-10-24T12:00:00Z',spec:params,state:{status:'completed',code:stored?undefined:'1234-fixture-code-phrase',result}});
   return JSON.stringify({id});
  }
  if(method==='transfer_list')return JSON.stringify(tasks);
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
