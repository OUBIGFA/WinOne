import {execFile} from "node:child_process";
import {promisify} from "node:util";
import {resolve} from "node:path";

// The PowerShell helper verifies executable, PID and private desktop before
// posting messages. This does not move or synthesize the user's system pointer.
export async function clickPrivateCanvas(output,x,y,action="click"){
 await promisify(execFile)("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/click-private-window.ps1"),"-StateFile",resolve(output,"state.json"),"-X",String(x),"-Y",String(y),"-Action",action],{windowsHide:true});
}
