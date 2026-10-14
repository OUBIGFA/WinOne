import {useCallback,useEffect,useRef,useState} from "react";
import {chatCall} from "../bridge";
import type {ScheduleRun,SchedulerEvent,SchedulerRuntimeSnapshot,WinOneSchedule} from "./chat-automations-types";
const errorText=(error:unknown)=>error instanceof Error?error.message:String(error);

/** The Host holds events for up to 25 seconds. Empty responses only renew that
 * subscription; catalog and history refresh only on actual scheduler events. */
export function useAutomations(){
 const [schedules,setSchedules]=useState<WinOneSchedule[]>([]),[runtime,setRuntime]=useState<SchedulerRuntimeSnapshot>(),[selectedId,setSelectedId]=useState(""),[runs,setRuns]=useState<ScheduleRun[]>([]),[loading,setLoading]=useState(true),[runsLoading,setRunsLoading]=useState(false),[error,setError]=useState(""),[runsError,setRunsError]=useState(""),[eventError,setEventError]=useState(""),[connection,setConnection]=useState(0),[hasOlder,setHasOlder]=useState(false);
 const alive=useRef(true),selection=useRef(""),listRequest=useRef(0),runRequest=useRef(0);
 const refresh=useCallback(async()=>{
  const revision=++listRequest.current;
  try{const [list,snapshot]=await Promise.all([chatCall<WinOneSchedule[]>("automations.list"),chatCall<SchedulerRuntimeSnapshot>("automations.runtime")]);if(!alive.current||revision!==listRequest.current)return;setSchedules(list);setRuntime(snapshot);setSelectedId(current=>list.some(s=>s.id===current)?current:list[0]?.id??"");setError("");}
  catch(error){if(alive.current&&revision===listRequest.current)setError(errorText(error));}
  finally{if(alive.current&&revision===listRequest.current)setLoading(false);}
 },[]);
 const loadRuns=useCallback(async(id:string,older=false)=>{
  if(!id)return;const revision=++runRequest.current;setRunsLoading(true);setRunsError("");
  try{const page=await chatCall<ScheduleRun[]>("automations.runs",{id,limit:50,...(older&&runs.length?{before:runs[runs.length-1].firedAt}:{})});if(!alive.current||selection.current!==id||revision!==runRequest.current)return;setRuns(current=>older?[...new Map([...current,...page].map(run=>[run.id,run])).values()]:page);setHasOlder(page.length===50);}
  catch(error){if(alive.current&&selection.current===id&&revision===runRequest.current)setRunsError(errorText(error));}
  finally{if(alive.current&&selection.current===id&&revision===runRequest.current)setRunsLoading(false);}
 },[runs]);
 const runLoader=useRef(loadRuns);runLoader.current=loadRuns;
 useEffect(()=>{selection.current=selectedId;setRuns([]);setHasOlder(false);setRunsError("");if(selectedId)void runLoader.current(selectedId);},[selectedId]);
 useEffect(()=>{alive.current=true;void refresh();return()=>{alive.current=false;};},[refresh]);
 useEffect(()=>{
  let disposed=false,cursor=0;setEventError("");
  void (async()=>{while(!disposed){try{
   const page=await chatCall<{cursor:number;events:SchedulerEvent[]}>("automations.events",{cursor});if(disposed)return;cursor=page.cursor;
   if(page.events.length){for(const event of page.events)if(event.type==="runtime-state")setRuntime(event.snapshot);
    await refresh();if(disposed)return;const id=selection.current;if(id&&page.events.some(event=>"scheduleId" in event?event.scheduleId===id:event.type==="all-read"||event.type==="ready"))await runLoader.current(id);
   }
  }catch(error){if(!disposed)setEventError(errorText(error));return;}}})();
  return()=>{disposed=true;};
 },[connection,refresh]);
 return {schedules,runtime,selectedId,select:setSelectedId,runs,loading,runsLoading,error,runsError,eventError,hasOlder,refresh,refreshRuns:()=>runLoader.current(selection.current),loadOlder:()=>runLoader.current(selection.current,true),reconnect:()=>{setConnection(value=>value+1);void refresh();}};
}
