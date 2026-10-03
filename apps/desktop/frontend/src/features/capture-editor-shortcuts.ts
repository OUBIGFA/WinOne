type KeyInput={key:string;ctrlKey:boolean;metaKey:boolean;shiftKey:boolean;altKey:boolean};

// Called from the editor's React keyboard handler. Portal content is logically
// owned by that handler even when it is outside the popup's DOM subtree.
export function hasEditorTextSelection(root:HTMLElement|null,target:EventTarget|null):boolean {
 const selection=root?.ownerDocument.getSelection();
 if(!selection||selection.isCollapsed||!selection.anchorNode?.isConnected||!selection.focusNode?.isConnected)return false;
 const element=target instanceof Element?target:null;
 const portal=element?.closest('[data-slot="popover-content"],[role="menu"]');
 const controlled=(element?.closest('[aria-controls]')?.getAttribute("aria-controls")??"").split(/\s+/).filter(Boolean).map(id=>root?.ownerDocument.getElementById(id));
 const owned=[root,portal,...controlled].some(scope=>!!scope?.isConnected&&scope.contains(selection.anchorNode)&&scope.contains(selection.focusNode));
 return owned&&selection.toString().length>0;
}
// Input controls keep their editing commands. The caller also checks read-only
// DOM selections before executing image-copy commands.
export function editorShortcut(event:KeyInput,editingText:boolean):"copy"|"save"|"saveAs"|"undo"|"redo"|"delete"|undefined {
 if(editingText||event.altKey)return;
 if(event.ctrlKey||event.metaKey) {
  const key=event.key.toLowerCase();
  if(key==="c"&&!event.shiftKey)return "copy";
  if(key==="s")return event.shiftKey?"saveAs":"save";
  if(key==="z")return event.shiftKey?"redo":"undo";
  if(key==="y"&&!event.shiftKey)return "redo";
  return;
 }
 if(event.key==="Delete")return "delete";
}
