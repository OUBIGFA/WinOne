// Adapted from Cindy ChatInput.handlePaste/onDrop at a44ac274 (Apache-2.0,
// Copyright 2026 XD Inc.). Keep file-before-text and directory classification;
// WinOne replaces Electron getFilePath with the native CF_HDROP/OLE bridge.
export function composerTransferFiles(transfer:DataTransfer,allowNativeDirectory=false):File[]{
 const items=Array.from(transfer.items??[]),files:File[]=[];
 for(const item of items){
  if(item.kind!=='file')continue;
  const file=item.getAsFile();if(!file)continue;
  // A browser cannot read a folder as a file. Native paths are classified by
  // the Host before reaching this fallback, rather than uploading an empty dir.
  if(item.webkitGetAsEntry?.()?.isDirectory){if(allowNativeDirectory)continue;throw Error('fileDrop.localOnly');}
  files.push(file);
 }
 return items.some(item=>item.kind==='file')?files:Array.from(transfer.files??[]);
}
export function hasComposerFiles(transfer:DataTransfer){
 return Array.from(transfer.types??[]).includes('Files')||Array.from(transfer.items??[]).some(item=>item.kind==='file');
}
