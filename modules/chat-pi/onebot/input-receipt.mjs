// Pi fixes a generation's model context at its committed cutoff. Keep the
// originating input there, rather than reading the newest user entry when a
// tool eventually executes (or a nested Code Mode call resumes).
const receiptMemo='winone.onebot.input-receipt';
const sourceMemo='winone.onebot.input-source';
export async function captureOneBotInputReceipt(harness,api,context){
 const saved=await api.memo(receiptMemo,context);
 if(saved!==undefined)return saved;
 const task=await harness.getTask(api.taskId,context);
 const cutoff=task?.state.checkpoint?.cutoff;
 if(!cutoff)throw Error('OneBot generation has no committed input boundary');
 const conversation=await harness.conversation(api.conversationId,context);
 let cursor,receipt=null,source=null,found=false;
 do{
  const page=await conversation.entries({order:'descending',maxEntryId:cutoff},64,cursor,context);
  for(const entry of page.items){
   if(entry.model?.some(message=>message.role==='user')){source=entry.data?.receipt??null;receipt=source?.id??null;found=true;break;}
   // A reset must not inherit authority from an unrelated historical input.
   if(entry.head===entry.id){found=true;break;}
  }
  if(found)break;
  cursor=page.cursor;
 }while(cursor);
 await api.memo(sourceMemo,source,context);
 return api.memo(receiptMemo,receipt,context);
}

export const oneBotToolInputReceipt=(api,context)=>readToolInputMemo(api,context,receiptMemo);
export const oneBotToolInputSource=(api,context)=>readToolInputMemo(api,context,sourceMemo);
async function readToolInputMemo(api,context,key){
 const visited=new Set();let id=api.taskId;
 while(id&&!visited.has(id)){
  visited.add(id);
  const task=await api.getTask(id,context);
  if(!task||task.conversationId!==api.conversationId)return null;
  if(task.kind==='pi.generation')return task.memos?.[key]??null;
  // Only the actual task ownership chain can carry authority. Parameters,
  // tool-call IDs and the mutable conversation's latest entry never can.
  if(task.kind!=='pi.tool')return null;
  id=task.owner;
 }
 return null;
}
