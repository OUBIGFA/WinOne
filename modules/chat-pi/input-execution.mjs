// Native execution receipts belong to an accepted Pi input, not a UI row or
// mutable "last task" pointer. Keep terminal outcomes in Pi's append-only file
// so Host/OneBot restarts can reconcile without repeating the model request.
export function readInputExecution(manager,id){
 const branch=manager.getBranch();
 const terminal=branch.findLast(entry=>entry.type==='custom'&&entry.customType==='winone.terminal'&&entry.data?.requestId===id);
 if(terminal)return terminal.data;
 const receipt=branch.findLast(entry=>entry.type==='custom'&&['winone.queued','winone.input'].includes(entry.customType)&&entry.data?.id===id)?.data;
 const alias=branch.findLast(entry=>entry.type==='custom'&&entry.customType==='winone.execution'&&entry.data?.requestId===id)?.data;
 return alias?.execution?{requestId:id,execution:alias.execution}:receipt?.execution?{requestId:id,execution:receipt.execution}:null;
}
export function recordInputExecution(manager,requestId,status,error='',resultText){
 const input=readInputExecution(manager,requestId);
 if(!input||input.status)return input;
 let active=false,last,lastId;
 for(const entry of manager.getBranch()){
  if(entry.type==='custom'&&entry.customType==='winone.input')active=entry.data?.id===requestId;
  if(active&&entry.type==='message'&&entry.message.role==='assistant'){last=entry.message;lastId=entry.id;}
 }
 const text=resultText??(typeof last?.content==='string'?last.content:(last?.content??[]).filter(part=>part.type==='text').map(part=>part.text).join('\n'));
 const terminal={...input,status,error,resultText:text.slice(0,60000),completedAt:Date.now(),...(lastId?{outputMessageId:lastId}:{})};
 manager.appendCustomEntry('winone.terminal',terminal);
 return terminal;
}
