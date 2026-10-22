// Keep transport envelopes distinct from actual document text. In particular,
// never JSON.parse an arbitrary string: a JSON file may legitimately contain a
// `content` field, and backslashes in source are not newline escape sequences.
export function toolResultText(result:unknown):string|undefined{
 if(typeof result==='string')return result;
 if(!result||typeof result!=='object')return undefined;
 const value=result as {content?:unknown;output?:unknown};
 const content=Array.isArray(value.content)?value.content:Array.isArray(value.output)?value.output:undefined;
 return content?.filter((part):part is {type:'text';text:string}=>!!part&&typeof part==='object'&&part.type==='text'&&typeof part.text==='string').map(part=>part.text).join('\n');
}
export type ToolFileContent={kind:'document';text:string;source:'read'|'write'}|{kind:'diff';text:string;source:'edit'};
export function toolFileContent(name:string,input:Record<string,unknown>,result:unknown):ToolFileContent|undefined{
 const tool=name.toLowerCase();
 if(tool==='read'){const text=toolResultText(result);return text===undefined?undefined:{kind:'document',text,source:'read'};}
 if(tool==='write'&&typeof input.content==='string')return {kind:'document',text:input.content,source:'write'};
 if(tool==='edit'||tool==='multiedit'){
  const entries=Array.isArray(input.edits)?input.edits:[input];
  const parts=entries.flatMap((entry,index)=>{
   if(!entry||typeof entry!=='object')return [];
   const oldText=entry.oldText??entry.old_string??entry.oldString,newText=entry.newText??entry.new_string??entry.newString;
   if(typeof oldText!=='string'||typeof newText!=='string')return [];
   return [`@@ ${index+1} @@\n`+oldText.split('\n').map(line=>'-'+line).join('\n')+'\n'+newText.split('\n').map(line=>'+'+line).join('\n')];
  });
  if(parts.length)return {kind:'diff',text:parts.join('\n'),source:'edit'};
 }
 return undefined;
}
