// Synthetic icon identities; no user programs or menus are invoked.
import {createRoot} from "react-dom/client";
import "../src/styles/shadcn.css";
import {WinTrayIcons,useCollectedIcons} from "../src/features/wintray-icons";
import "../src/style.css";

function icon(color:string,label:string) {
 const canvas=document.createElement("canvas");canvas.width=canvas.height=24;
 const context=canvas.getContext("2d")!;
 context.fillStyle=color;context.fillRect(0,0,24,24);context.fillStyle="white";context.font="16px Segoe UI";context.fillText(label,6,18);
 return canvas.toDataURL("image/png").split(",")[1];
}
let calls=0;
const images=[icon("#496988","S"),icon("#55765b","S")];
type NativeApp=NonNullable<NonNullable<Window["go"]>["main"]>["App"];
window.go={main:{App:{WinTray:async(method:string)=>method==="icons"?JSON.stringify([{id:"sync",name:"同步工具",image:images[(calls++)%2]},{id:"notes",name:"笔记",image:icon("#756580","N")}]) : JSON.stringify({ok:true})} as unknown as NativeApp}};
function Fixture(){const {icons}=useCollectedIcons(true);return <main className="mx-auto max-w-2xl p-8"><WinTrayIcons icons={icons}/></main>;}
createRoot(document.getElementById("root")!).render(<Fixture/>);
