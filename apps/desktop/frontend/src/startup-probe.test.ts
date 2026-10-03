// @vitest-environment jsdom
import {expect,it,vi} from "vitest";
import {finishStartupProbe} from "./startup-probe";

it("leaves normal startup free of probe asset loading",async()=>{
 const complete=vi.fn(),load=vi.fn();
 await finishStartupProbe({nonce:async()=>"",complete},document,load);
 expect(complete).not.toHaveBeenCalled();expect(load).not.toHaveBeenCalled();
});

it("requires the rendered shell, stylesheet, logo and editor chunk",async()=>{
 document.body.innerHTML='<div class="shell" style="display:flex"><nav><button aria-current="page"></button></nav><img class="mark"><div data-slot="window-controls"><button></button><button></button><button></button></div></div>';
 const logo=document.querySelector<HTMLImageElement>(".mark")!;
 logo.decode=vi.fn().mockResolvedValue(undefined);
 Object.defineProperty(logo,"naturalWidth",{value:30});
 const complete=vi.fn(),load=vi.fn().mockResolvedValue({});
 await finishStartupProbe({nonce:async()=>"nonce",complete},document,load);
 expect(complete).toHaveBeenCalledWith("nonce");expect(load).toHaveBeenCalledOnce();
 load.mockRejectedValue(Error("missing chunk"));complete.mockClear();
 await expect(finishStartupProbe({nonce:async()=>"nonce",complete},document,load)).rejects.toThrow("missing chunk");
 expect(complete).not.toHaveBeenCalled();
});
