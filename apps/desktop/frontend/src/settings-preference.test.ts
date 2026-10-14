// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {savePreferenceChange,type Settings} from "./bridge";

afterEach(()=>{delete window.go;});

it("saves an appearance change against the latest revision without reverting other settings",async()=>{
 const latest:Settings={schemaVersion:1,revision:8,theme:"light",language:"en-US",captureShortcut:"Alt+S",startup:{runAtLogon:true,startMinimized:true,preferAdministrator:true}};
 const save=vi.fn(async(value:Settings)=>({...value,revision:9}));
 window.go={main:{App:{Snapshot:async()=>({settings:latest,modules:[]}),SaveSettings:save}}} as unknown as NonNullable<Window["go"]>;
 await expect(savePreferenceChange({theme:"dark"})).resolves.toMatchObject({theme:"dark",revision:9});
 expect(save).toHaveBeenCalledWith({...latest,theme:"dark"});
 await savePreferenceChange({startup:{startMinimized:false}});
 expect(save).toHaveBeenLastCalledWith({...latest,startup:{...latest.startup,startMinimized:false}});
});

it("reports a concurrent save conflict without overwriting or silently retrying",async()=>{
 const save=vi.fn(async()=>{throw Error("conflict");});
 window.go={main:{App:{Snapshot:async()=>({settings:{schemaVersion:1,revision:8,theme:"light"},modules:[]}),SaveSettings:save}}} as unknown as NonNullable<Window["go"]>;
 await expect(savePreferenceChange({theme:"dark"})).rejects.toThrow("conflict");
 expect(save).toHaveBeenCalledOnce();
});
