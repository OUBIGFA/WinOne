export interface Totals {credits?:number;credited?:number;calls:number;errors:number;input:number;output:number;cache_read:number;cache_write:number;cache_write_1h?:number;reasoning?:number;cost:number;unpriced:number;timed?:number;ttft_ms?:number;decode_ms?:number;decode_out?:number}
export interface UsageShare extends Totals {id:string;name?:string}
export interface UsageRow {callerKeyId?:string;callerKeyName?:string;credits?:number;tier_asked?:string;tier_sent?:string;tier_served?:string;t:string;provider:string;agent:string;model:string;in:number;out:number;cache_read?:number;cache_write?:number;cache_write_1h?:number;reasoning?:number;ms:number;ttft_ms?:number;first_text_ms?:number;sent_ms?:number;flow_ms?:number;status:number;speed?:number;err?:string;err_type?:string;priced:boolean;cost:number;req?:string;served?:string;upstream?:string;ep?:string;effort?:string;stop?:string;providerAccount?:string;rejected?:boolean;kind?:string}
export interface UsagePoint extends Totals {time:string;by?:Record<string,Record<string,{calls:number;tokens:number;cost:number;timed?:number;ttft_ms?:number;decode_ms?:number;decode_out?:number}>>}
export interface UsageHeatmapData {from:string;to:string;days:{date:string;calls:number;tokens:number;cost:number;unpriced?:number}[]}
export interface UsagePage {callerKeys?:{id:string;callerKeyId?:string;callerKeyName?:string}[];heatmap?:UsageHeatmapData;rows:UsageRow[];sum:Totals;total:number;offset:number;limit:number;by:Record<string,UsageShare[]>;chartBy?:Record<string,UsageShare[]>;series?:UsagePoint[];bucket?:string;day?:string;priceWarning?:boolean;accounts?:{id:string;account:string;provider?:string;name?:string}[];purposes?:string[];csv?:string}
export interface UsageFilter {callerKey?:string;period:string;provider:string;source:string;failed:boolean;offset:number;day:string;model:string;account:string;purpose:string;query:string}
export interface SessionCounts {sessions:number;messages:number;calls:number;input:number;output:number;cache_read:number;cache_write:number;reasoning:number;errors:number;compactions:number;activeMs:number;cost:number;unpriced:number}
export interface UsageSession extends SessionCounts {id:string;title:string;project:string;provider:string;model:string;createdAt:string;updatedAt:string;archived:boolean;context:{time:string;input:number;cache_read:number;output:number;compaction?:boolean}[];tools:Record<string,number>;skills:Record<string,number>;roles:Record<string,number>;error?:string;partial?:boolean}
export interface SessionReport {rows:UsageSession[];sum:SessionCounts;series:(SessionCounts&{time:string})[];by:Record<string,Record<string,SessionCounts>>;total:number;offset:number;limit:number;median:number;p90:number;top:UsageSession[];tools:Record<string,number>;skills:Record<string,number>;warnings:string[];shape:Record<string,{edges:number[];counts:number[];total:number}>}
export const totalTokens=(x:{input:number;output:number;cache_read:number;cache_write:number})=>x.input+x.output+x.cache_read+x.cache_write;
// Go encodes a nil slice or map as JSON null. WinOne's own empty catalogs, and
// Host builds older than the current UI, therefore send null where the pages
// expect arrays; coerce once at the boundary instead of guarding every render.
export const listOrEmpty=<T,>(items:readonly T[]|null|undefined):T[]=>items?[...items]:[];
export const groupsOrEmpty=<T,>(items:Record<string,T>|null|undefined):Record<string,T>=>items??{};
export function normalizeSessionReport(value:SessionReport):SessionReport{
 const session=(item:UsageSession):UsageSession=>({...item,context:listOrEmpty(item.context),tools:groupsOrEmpty(item.tools),skills:groupsOrEmpty(item.skills),roles:groupsOrEmpty(item.roles)});
 return {...value,rows:listOrEmpty(value.rows).map(session),top:listOrEmpty(value.top).map(session),series:listOrEmpty(value.series),warnings:listOrEmpty(value.warnings),by:groupsOrEmpty(value.by),tools:groupsOrEmpty(value.tools),skills:groupsOrEmpty(value.skills),shape:groupsOrEmpty(value.shape)};
}
export function normalizeUsagePage(value:UsagePage):UsagePage{
 return {...value,callerKeys:listOrEmpty(value.callerKeys),rows:listOrEmpty(value.rows),by:groupsOrEmpty(value.by),series:listOrEmpty(value.series),accounts:listOrEmpty(value.accounts),purposes:listOrEmpty(value.purposes)};
}
