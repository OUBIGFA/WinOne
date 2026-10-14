// Adapted from Magpie internal/gui/assets/app.js (MIT), a8ca7908556ead568c3d3a72e6e58e7e11852caa.
// Draft rows stay ordered and retain partial input; serialization follows Magpie.
export type HeaderRow=[name:string,value:string,json:boolean];
export function headerRows(headers?:Record<string,string>):HeaderRow[]{return Object.entries(headers??{}).map(([name,value])=>[name,value,false]);}
export function looksJSON(value:string){const text=value.trim();return text.startsWith("{")||text.startsWith("[");}
export function minifyJSON(value:string){try{return JSON.stringify(JSON.parse(value));}catch{return value;}}
export function prettyJSON(value:string){try{return JSON.stringify(JSON.parse(value),null,2);}catch{return value;}}
export function headersOf(rows:HeaderRow[]):Record<string,string>{
 const result:Record<string,string>={};
 for(const [key,value] of rows){const name=key.trim(),text=value.trim();if(!name||!text)continue;for(const previous of Object.keys(result))if(previous.toLowerCase()===name.toLowerCase())delete result[previous];result[name]=looksJSON(text)?minifyJSON(text):text;}
 return result;
}
