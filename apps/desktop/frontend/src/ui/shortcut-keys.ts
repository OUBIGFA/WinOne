type KeyInput={key:string;code:string;ctrlKey:boolean;altKey:boolean;shiftKey:boolean;metaKey:boolean;repeat:boolean;isComposing:boolean;altGraph:boolean};
type RecordedKey={kind:"shortcut"|"modifier";value:string}|{kind:"invalid";reason:"modifier"|"key"|"reserved"}|{kind:"ignore"};
// Match the backend's supported vocabulary. KeyboardEvent.key respects the
// active layout; code is used only for shifted digit symbols such as Shift+1.
export function recordedShortcut(event:KeyInput):RecordedKey {
 if(event.repeat)return {kind:"ignore"};
 if(event.isComposing||event.altGraph)return {kind:"invalid",reason:"key"};
 const modifiers=[event.ctrlKey?"Ctrl":"",event.altKey?"Alt":"",event.shiftKey?"Shift":"",event.metaKey?"Win":""].filter(Boolean);
 if(["Control","Alt","Shift","Meta"].includes(event.key))return {kind:"modifier",value:modifiers.join("+")};
 let key=event.key.toUpperCase();
 if(/^Numpad[0-9]$/.test(event.code)){
  if(!/^[0-9]$/.test(key))return {kind:"invalid",reason:"key"};
  key=`NUM${event.code.slice(-1)}`;
 }
 if(!/^[A-Z0-9]$/.test(key)&&/^Digit[0-9]$/.test(event.code))key=event.code.slice(-1);
 if(!/^[A-Z0-9]$/.test(key)&&!/^NUM[0-9]$/.test(key)&&!/^F([1-9]|1[0-9]|2[0-4])$/.test(key))return {kind:"invalid",reason:"key"};
 if(key==="F12")return {kind:"invalid",reason:"reserved"};
 if(!modifiers.length)return {kind:"invalid",reason:"modifier"};
 return {kind:"shortcut",value:[...modifiers,key.replace(/^NUM/,"Num")].join("+")};
}
