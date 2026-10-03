import {captureCall} from "../bridge";
import type {ToolbarPreferencesPort} from "../ui/capture-drawing-tools";

const allowed=new Set(["select","crop","rectangle","ellipse","arrow","pen","cover","text","mosaic","blur"]);
export const captureToolbarPreferences:ToolbarPreferencesPort={
 read:async()=>{
  const settings=await captureCall<{toolbarTools?:unknown}>("settings.get");
  const tools=settings.toolbarTools;
  if(tools===undefined||tools===null)return null;
  if(!Array.isArray(tools)||tools.some(tool=>typeof tool!=="string"||!allowed.has(tool))||new Set(tools).size!==tools.length)throw Error("Invalid capture toolbar settings");
  return tools as string[];
 },
 save:async tools=>{
  // Read the latest preferences to preserve unrelated settings. Host revision
  // validation rejects concurrent changes between this read and the save.
  const current=await captureCall<Record<string,unknown>>("settings.get");
  await captureCall("settings.save",{...current,toolbarTools:tools});
 },
};
