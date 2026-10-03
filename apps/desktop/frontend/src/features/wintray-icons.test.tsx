// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {WinTrayIcons,useCollectedIcons,iconsForProgram} from "./wintray-icons";
const native=vi.hoisted(()=>({call:vi.fn()}));
vi.mock("../bridge",()=>({wintrayCall:native.call}));
function Preview(){const {icons}=useCollectedIcons(true);return <WinTrayIcons icons={icons}/>;}

it.each([null,{}, {delivered:false}])("reports an icon action without delivery confirmation: %j",async result=>{
 native.call.mockResolvedValue(result);
 render(<WinTrayIcons icons={[{id:"one",name:"Example",exePath:"C:\\Example.exe"}]}/>);
 await userEvent.setup().click(screen.getByRole("button",{name:"打开 Example"}));
 expect(await screen.findByRole("alert")).toHaveProperty("textContent","无法确认操作结果");
});

it("uses WinOne actions for a hosted program instead of an independent original menu",async()=>{
 native.call.mockResolvedValue({delivered:true});
 render(<WinTrayIcons icons={[{id:"hosted:one",name:"Example",exePath:"C:\\Example.exe",hosted:true}]}/>);
 expect(screen.queryByRole("button",{name:"打开 Example 的原菜单"})).toBeNull();
 const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"程序操作"}));
 await user.click(await screen.findByRole("menuitem",{name:"隐藏窗口"}));
 expect(native.call).toHaveBeenCalledWith("icon.activate",{id:"hosted:one",right:false,action:"hide"});
});
afterEach(()=>{cleanup();native.call.mockReset();});
it("matches icons by executable path rather than display name",()=>{
 const icons=[{id:"a",name:"Same",exePath:"C:\\Apps\\One.exe"},{id:"b",name:"Same",exePath:"C:\\Apps\\Two.exe"}];
 expect(iconsForProgram(icons,"c:/apps/ONE.EXE").map(icon=>icon.id)).toEqual(["a"]);
});
it("does not query icons for an inactive list",()=>{
 function Inactive(){useCollectedIcons(false);return null;}
 render(<Inactive/>);expect(native.call).not.toHaveBeenCalled();
});
it("loads collected icons in the program row and forwards the original menu action",async()=>{
 native.call.mockImplementation(async(method:string)=>method==="icons"?[{id:"stable-icon",name:"Example"}]:{delivered:true});
 const {unmount}=render(<Preview/>);


 await userEvent.setup().click(await screen.findByRole("button",{name:"打开 Example 的原菜单"}));
 expect(native.call).toHaveBeenCalledWith("icon.activate",{id:"stable-icon",right:true});
 await waitFor(()=>expect((screen.getByRole("button",{name:"打开 Example"}) as HTMLButtonElement).disabled).toBe(false));
 unmount();
});
it("updates the image for the same native icon identity",async()=>{
 let reads=0;
 native.call.mockImplementation(async()=>[{id:"stable",name:"Example",image:++reads===1?"AAAA":"BBBB"}]);
 render(<Preview/>);

 await waitFor(()=>expect(document.querySelector("img")?.getAttribute("src")).toBe("data:image/png;base64,AAAA"));
 const image=document.querySelector("img");
 await waitFor(()=>expect(image?.getAttribute("src")).toBe("data:image/png;base64,BBBB"),{timeout:2500});
 expect(document.querySelector("img")).toBe(image);
});
