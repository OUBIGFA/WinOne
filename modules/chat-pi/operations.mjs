import {randomUUID} from "node:crypto";

// These are UI records in Pi's native session file, never model messages.
// A start is persisted as well as its end so a crashed worker leaves honest
// interrupted history. Elapsed time uses the monotonic clock, not wall time.
export function createOperationRecorder({write,emit,now=Date.now,clock=()=>performance.now()}){
 const pending=new Map();
 function end(key,status="completed"){
  const current=pending.get(key);if(!current)return;
  const record={...current.record,status,durationMs:Math.max(0,Math.round(clock()-current.clock))};
  write(record);pending.delete(key);emit(record);
 }
 return {
  begin(key,kind,turnId,details={}){
   if(!turnId)return;
   if(pending.has(key))end(key,"failed");
   const record={id:randomUUID(),turnId,kind,startedAt:now(),status:"running",...details};
   write(record);pending.set(key,{record,clock:clock()});emit(record);return record.id;
  },end,
  endAll(status){for(const key of pending.keys())end(key,status);},
 };
}

const kinds=new Set(["turn","compaction","retry","review","approval"]);
const statuses=new Set(["running","completed","failed","stopped","allowed","denied"]);
export function readOperations(branch,turnIDs,runningTurn){
 const latest=new Map();
 for(const entry of branch){
  if(entry.type!=="custom"||entry.customType!=="winone.operation")continue;
  const op=entry.data;
  if(typeof op?.id!=="string"||op.id.length>128||!turnIDs.has(op.turnId)||!kinds.has(op.kind)||!statuses.has(op.status)||!Number.isFinite(op.startedAt))continue;
  const value={id:op.id,turnId:op.turnId,kind:op.kind,startedAt:op.startedAt,status:op.status==="running"&&op.turnId!==runningTurn?"stopped":op.status};
  for(const field of ["durationMs","attempt","maxAttempts","delayMs"])if(Number.isFinite(op[field])&&op[field]>=0)value[field]=op[field];
  latest.set(op.id,value);
 }
 return [...latest.values()];
}
