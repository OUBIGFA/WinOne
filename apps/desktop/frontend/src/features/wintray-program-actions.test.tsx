// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {WinTrayProgramActions} from './wintray-program-actions';
const native=vi.hoisted(()=>({call:vi.fn()}));vi.mock('../bridge',()=>({wintrayCall:native.call}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
it('reads live state independently and only closes after explicit confirmation',async()=>{
 native.call.mockResolvedValue({running:true,hasWindow:true,visible:true});const run=vi.fn(async()=>{}),user=userEvent.setup();
 render(<WinTrayProgramActions id="one" name="Editor" disabled={false} busy={false} onRun={run}/>);
 await user.click(await screen.findByRole('button',{name:'隐藏窗口'}));expect(run).toHaveBeenCalledWith('program.window',{id:'one',visible:false});run.mockClear();
 await user.click(screen.getByRole('button',{name:'关闭程序'}));expect(run).not.toHaveBeenCalled();await user.click(screen.getByRole('button',{name:'取消'}));expect(run).not.toHaveBeenCalled();
 await user.click(screen.getByRole('button',{name:'关闭程序'}));await user.click(screen.getByRole('button',{name:'确认关闭'}));expect(run).toHaveBeenCalledWith('program.stop',{id:'one',confirmed:true});
});
it('keeps startup and visibility independent and blocks operations on unsaved settings',async()=>{
 native.call.mockResolvedValue({running:false,hasWindow:false,visible:false});const run=vi.fn(async()=>{}),user=userEvent.setup();
 const view=render(<WinTrayProgramActions id="one" name="App" disabled={false} busy={false} onRun={run}/>);
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('program.state',{id:'one'}));await user.click(screen.getByRole('button',{name:'立即启动'}));expect(run).toHaveBeenCalledWith('launch',{id:'one'});
 expect(screen.getByRole('button',{name:'显示窗口'})).toHaveProperty('disabled',true);
 view.rerender(<WinTrayProgramActions id="one" name="App" disabled busy={false} onRun={run}/>);expect(screen.getByRole('button',{name:'立即启动'})).toHaveProperty('disabled',true);
});
it('reports a state read failure without replacing settings or inventing a running state',async()=>{
 native.call.mockRejectedValue(Error('module unavailable'));render(<WinTrayProgramActions id="one" name="App" disabled={false} busy={false} onRun={async()=>{}}/>);
 expect(await screen.findByRole('alert')).toHaveProperty('title','Error: module unavailable');expect(screen.queryByRole('button',{name:'关闭程序'})).toBeNull();
});
