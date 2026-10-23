import type {UsageRow} from "./ai-usage-types";

// Magpie c4a5e615, plugins/usage/page.js: gateway-only ledFloor_/ledNoSpeed_.
// Host supplies the measured decode speed. Floors belong to request rows,
// never to the aggregate speed KPI; retain the upstream sampling thresholds.
export function requestSpeed(row:UsageRow){
 const none=(reason:string)=>({value:0,floor:false,reason});
 if(row.status>=400||row.err)return none("speedFailed");
 if(row.speed&&row.speed>0)return {value:row.speed,floor:false,reason:""};
 const first=row.ttft_ms??0,reasoning=row.reasoning??0,firstText=row.first_text_ms??0,flow=row.flow_ms??0;
 const answer=row.out-reasoning,window=row.ms-first;
 if(first>0&&reasoning>0&&!(firstText>0)&&answer>=100&&window>=1000&&answer*1000<=10000*window&&!(flow>0&&flow*4<window))return {value:answer*1000/window,floor:true,reason:"speedAnswerFloor"};
 if(row.out>=1000&&row.ms>=1000&&row.out*1000<=10000*row.ms)return {value:row.out*1000/row.ms,floor:true,reason:"speedWholeFloor"};
 if(!(row.ms>0))return none("speedNoDuration");
 if(!(row.out>0))return none("speedNoOutput");
 if(!(first>0))return none("speedNoFirstToken");
 if(!(answer>0))return none("speedReasoningOnly");
 const from=reasoning>0&&firstText>0?firstText:first,w=row.ms-from;
 if(w>=100&&(answer*1000>10000*w||flow>0&&flow*4<w||reasoning>0&&firstText>0&&answer*(firstText-first)>20*reasoning*w))return none("speedBurst");
 return none("speedTooLittle");
}
