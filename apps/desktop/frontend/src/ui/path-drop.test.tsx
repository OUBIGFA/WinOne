// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,waitFor} from "@testing-library/react";
import {usePathDrop,droppedTextPaths,type PathDropOptions} from "./path-drop";
const api=vi.hoisted(()=>({inspect:vi.fn(),target:vi.fn()}));
vi.mock("../bridge",()=>({inspectDroppedPaths:api.inspect,setFileDropTarget:api.target}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
function Zone(props:PathDropOptions){return <div data-testid="zone" {...usePathDrop(props)}><span>drop here</span></div>;}
function nativeDrop(element:Element,paths:string[]){return fireEvent(element,new CustomEvent("winone:paths-drop",{bubbles:true,cancelable:true,detail:{paths}}));}
it("validates native paths and leaves directory fields unchanged for files and multiple entries",async()=>{
 const onPaths=vi.fn(),onError=vi.fn();const view=render(<Zone kind="directory" onPaths={onPaths} onError={onError}/>);
 api.inspect.mockResolvedValue([{path:"C:\\file.txt",kind:"file"}]);
 nativeDrop(view.getByTestId("zone"),["C:\\file.txt"]);
 await waitFor(()=>expect(onError).toHaveBeenCalledWith(expect.stringContaining("只接收文件夹")));
 expect(onPaths).not.toHaveBeenCalled();
 nativeDrop(view.getByTestId("zone"),["C:\\one","C:\\two"]);
 await waitFor(()=>expect(onError).toHaveBeenCalledWith(expect.stringContaining("一个项目")));
 expect(api.inspect).toHaveBeenCalledTimes(1);
 api.inspect.mockResolvedValue([{path:"C:\\folder",kind:"directory"}]);
 nativeDrop(view.getByTestId("zone"),["C:\\folder"]);
 await waitFor(()=>expect(onPaths).toHaveBeenCalledWith(["C:\\folder"]));
});
it("consumes disabled and nested targets without leaking into enclosing drop zones",async()=>{
 const inner=vi.fn(),outer=vi.fn(),onError=vi.fn();
 function Nested(){const drop=usePathDrop({multiple:true,onPaths:outer,onError});return <div {...drop}><Zone disabled onPaths={inner} onError={onError}/></div>;}
 const view=render(<Nested/>);
 expect(nativeDrop(view.getByTestId("zone").firstElementChild!,["C:\\file.txt"])).toBe(false);
 await Promise.resolve();expect(inner).not.toHaveBeenCalled();expect(outer).not.toHaveBeenCalled();expect(api.inspect).not.toHaveBeenCalled();
});
it("ignores an inspection completing after its target unmounted",async()=>{
 let finish!:(value:unknown)=>void;api.inspect.mockReturnValue(new Promise(resolve=>{finish=resolve;}));
 const onPaths=vi.fn();const view=render(<Zone onPaths={onPaths} onError={vi.fn()}/>);
 nativeDrop(view.getByTestId("zone"),["C:\\file.txt"]);view.unmount();finish([{path:"C:\\file.txt",kind:"file"}]);
 await Promise.resolve();await Promise.resolve();expect(onPaths).not.toHaveBeenCalled();
});
it("parses file URIs and quoted path lists without importing web URLs",()=>{
 const data=(values:Record<string,string>)=>({getData:(key:string)=>values[key]??""}) as DataTransfer;
 expect(droppedTextPaths(data({"text/uri-list":"# comment\nfile:///C:/My%20Files/a.txt\nfile://server/share/folder"}))).toEqual(["C:/My Files/a.txt","\\\\server\\share\\folder"]);
 expect(droppedTextPaths(data({"text/plain":'"C:\\My Files\\a.txt"\nC:/My Files/a.txt'}))).toEqual(["C:\\My Files\\a.txt"]);
 expect(()=>droppedTextPaths(data({"text/uri-list":"https://example.com/file"}))).toThrow("本地文件");
});
