// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {TimedPowerPage,type PowerPreferences,type PowerSnapshot,validPowerPreferences} from './timed-power';
import {setApplicationLanguage} from '../i18n';

const call=vi.hoisted(()=>vi.fn());
vi.mock('../bridge',()=>({timedPowerCall:call}));
const defaults:PowerPreferences={schemaVersion:1,revision:7,action:'shutdown',mode:'countdown',delaySeconds:3600,timeOfDay:'23:00:00',weekdays:[1,2,3,4,5],reminderSeconds:60,force:true,dryRun:false};
let snapshot:PowerSnapshot;
const field=(label:string)=>screen.getByRole('spinbutton',{name:label}) as HTMLInputElement;
const enter=(label:string,value:string)=>fireEvent.change(field(label),{target:{value}});
beforeEach(()=>{
 snapshot={state:'idle',active:false,deadline:''};
 call.mockImplementation(async(method:string,payload?:any)=>{
  if(method==='settings.get')return structuredClone(defaults);
  if(method==='settings.save')return {...payload,revision:8};
  if(method==='snapshot')return structuredClone(snapshot);
  if(method==='start'){snapshot={state:'running',active:true,task:structuredClone(payload.preferences),deadline:new Date(Date.now()+payload.preferences.delaySeconds*1000).toISOString()};return snapshot;}
  if(method==='cancel'){snapshot={...snapshot,state:'cancelled',active:false};return snapshot;}
  throw Error(method);
 });
});
afterEach(async()=>{cleanup();call.mockReset();await setApplicationLanguage('zh-CN');});

it('defaults to force and only submits after explicit risk confirmation',async()=>{
 render(<TimedPowerPage/>);const user=userEvent.setup();
 expect((await screen.findByRole('switch',{name:'强制关闭程序'})).getAttribute('aria-checked')).toBe('true');
 await user.click(screen.getByRole('button',{name:'开始计时'}));
 const dialog=await screen.findByRole('dialog');expect(dialog.textContent).toContain('未保存内容可能丢失');
 expect(call.mock.calls.some(([method])=>method==='start')).toBe(false);
 await user.click(within(dialog).getByRole('button',{name:'继续编辑'}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 expect(call.mock.calls.some(([method])=>method==='start')).toBe(false);
 await user.click(screen.getByRole('button',{name:'开始计时'}));
 await user.click(within(await screen.findByRole('dialog')).getByRole('button',{name:'确认并开始'}));
 await screen.findByText('正在计时');
 expect(call).toHaveBeenCalledWith('start',{preferences:defaults,confirmed:true});
});

it.each([[2,30,0,9000],[1,30,0,5400],[0,15,0,900],[0,0,15,15]])('accepts typed duration %i:%i:%i without asking the user to convert seconds',async(hours,minutes,seconds,total)=>{
 render(<TimedPowerPage disabled/>);const user=userEvent.setup();
 await screen.findByRole('group',{name:'倒计时时长'});
 enter('时',String(hours));enter('分',String(minutes));enter('秒',String(seconds));
 expect([field('时').value,field('分').value,field('秒').value]).toEqual([String(hours),String(minutes),String(seconds)]);
 await user.click(screen.getByRole('button',{name:'保存设置'}));
 await screen.findByText('已保存。保存不会开始或改变任务。');
 expect(call).toHaveBeenCalledWith('settings.save',{...defaults,delaySeconds:total});
 expect(call.mock.calls.every(([method])=>['settings.get','settings.save'].includes(method))).toBe(true);
});

it('supports direct multi-digit typing, clearing one segment, and all quick presets',async()=>{
 render(<TimedPowerPage disabled/>);const user=userEvent.setup();await screen.findByRole('group',{name:'倒计时时长'});
 for(const [label,h,m] of [['15 分钟','0','15'],['30 分钟','0','30'],['1 小时','1','0'],['1 小时 30 分钟','1','30'],['2 小时','2','0'],['2 小时 30 分钟','2','30']]){
  await user.click(screen.getByRole('button',{name:label}));expect(field('时').value).toBe(h);expect(field('分').value).toBe(m);expect(field('秒').value).toBe('0');
 }
 await user.clear(field('分'));expect(field('分').value).toBe('');expect(field('时').value).toBe('2');
 expect((screen.getByRole('button',{name:'保存设置'}) as HTMLButtonElement).disabled).toBe(true);
 await user.type(field('分'),'45');expect(field('分').value).toBe('45');
 await user.click(screen.getByRole('button',{name:'保存设置'}));
 expect(call).toHaveBeenCalledWith('settings.save',{...defaults,delaySeconds:9900});
});

it('keeps inline unit labels accessible and quick durations keyboard-operable',async()=>{
 render(<TimedPowerPage disabled/>);const user=userEvent.setup();
 const duration=await screen.findByRole('group',{name:'倒计时时长'});
 for(const label of ['时','分','秒']){
  const input=within(duration).getByRole('spinbutton',{name:label}) as HTMLInputElement;
  const unit=input.labels?.[0]?.querySelector('[aria-hidden="true"]');
  expect(unit?.textContent).toBe(label);await user.click(unit!);expect(document.activeElement).toBe(input);
 }
 const shortcuts=screen.getByRole('group',{name:'快捷时间'});expect(within(shortcuts).getAllByRole('button')).toHaveLength(6);
 const shortcut=within(shortcuts).getByRole('button',{name:'2 小时 30 分钟'});shortcut.focus();await user.keyboard('{Enter}');
 expect([field('时').value,field('分').value,field('秒').value]).toEqual(['2','30','0']);
});

it.each(['指定时刻','每周重复'])('uses hours/minutes/seconds for %s, keeps edits across modes and saves second precision',async(mode)=>{
 render(<TimedPowerPage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole('radio',{name:mode}));
 const group=screen.getByRole('group',{name:'执行时间'});
 expect(within(group).getAllByRole('spinbutton')).toHaveLength(3);
 enter('时','21');enter('分','15');enter('秒','37');
 await user.click(screen.getByRole('radio',{name:'倒计时'}));
 expect([field('时').value,field('分').value,field('秒').value]).toEqual(['1','0','0']);
 await user.click(screen.getByRole('radio',{name:mode}));
 expect([field('时').value,field('分').value,field('秒').value]).toEqual(['21','15','37']);
 await user.click(screen.getByRole('button',{name:'保存设置'}));
 expect(call).toHaveBeenCalledWith('settings.save',{...defaults,mode:mode==='指定时刻'?'time':'weekly',timeOfDay:'21:15:37'});
});

it('reads legacy minute-only times as zero seconds without making the form dirty',async()=>{
 call.mockResolvedValueOnce({...defaults,timeOfDay:'22:45'});render(<TimedPowerPage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole('radio',{name:'指定时刻'}));
 expect([field('时').value,field('分').value,field('秒').value]).toEqual(['22','45','0']);
 await user.click(screen.getByRole('radio',{name:'倒计时'}));
 expect((screen.getByRole('button',{name:'保存设置'}) as HTMLButtonElement).disabled).toBe(true);
});

it('runs reminder-only without confirmation, keeps active settings independent, and cancels',async()=>{
 render(<TimedPowerPage/>);const user=userEvent.setup();
 await user.click(await screen.findByRole('switch',{name:'仅提醒，不执行电源操作'}));
 await user.click(screen.getByRole('button',{name:'开始计时'}));await screen.findByText('正在计时');
 expect(screen.queryByRole('dialog')).toBeNull();
 expect(call).toHaveBeenCalledWith('start',{preferences:{...defaults,dryRun:true},confirmed:false});
 enter('时','0');enter('分','30');
 await user.click(screen.getByRole('button',{name:'保存设置'}));await screen.findByText('已保存。保存不会开始或改变任务。');
 expect(snapshot.task?.delaySeconds).toBe(3600);
 await user.click(screen.getByRole('button',{name:'取消任务'}));await screen.findByText('任务已取消');
 expect(call.mock.calls.filter(([method])=>method==='start')).toHaveLength(1);
});

it('preserves drafts on module state changes and restores all three saved segments',async()=>{
 const view=render(<TimedPowerPage disabled/>);const user=userEvent.setup();await screen.findByRole('group',{name:'倒计时时长'});
 enter('时','0');enter('分','2');enter('秒','15');
 expect((screen.getByRole('button',{name:'开始计时'}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole('button',{name:'保存设置'}));await screen.findByText('已保存。保存不会开始或改变任务。');
 enter('分','4');enter('秒','30');
 view.rerender(<TimedPowerPage/>);await waitFor(()=>expect(call).toHaveBeenCalledWith('snapshot'));
 expect(field('分').value).toBe('4');expect(field('秒').value).toBe('30');
 await user.click(screen.getByRole('button',{name:'撤销修改'}));expect(field('分').value).toBe('2');expect(field('秒').value).toBe('15');
});

it('does not overwrite an unfinished form when live state updates',async()=>{
 snapshot={state:'running',active:true,task:defaults,deadline:new Date(Date.now()+60000).toISOString()};
 render(<TimedPowerPage/>);await screen.findByText('正在计时');enter('分','17');
 snapshot={...snapshot,state:'cancelled',active:false};
 await screen.findByText('任务已取消',{}, {timeout:2500});expect(field('分').value).toBe('17');
});

it('rejects out-of-range segments without silently clamping the typed value',async()=>{
 render(<TimedPowerPage/>);const user=userEvent.setup();await screen.findByRole('group',{name:'倒计时时长'});
 enter('分','60');expect(field('分').value).toBe('60');expect((screen.getByRole('button',{name:'开始计时'}) as HTMLButtonElement).disabled).toBe(true);
 enter('分','0');enter('时','0');expect((screen.getByRole('button',{name:'开始计时'}) as HTMLButtonElement).disabled).toBe(true);
 enter('秒','1');expect((screen.getByRole('button',{name:'开始计时'}) as HTMLButtonElement).disabled).toBe(false);
 await user.click(screen.getByRole('radio',{name:'指定时刻'}));enter('时','24');
 expect(field('时').value).toBe('24');expect((screen.getByRole('button',{name:'保存设置'}) as HTMLButtonElement).disabled).toBe(true);
 expect(validPowerPreferences({...defaults,mode:'weekly',weekdays:[]})).toBe(false);
 expect(validPowerPreferences({...defaults,timeOfDay:'24:00:00'})).toBe(false);
 expect(validPowerPreferences({...defaults,delaySeconds:604801})).toBe(false);
});

it('retries failed loading and keeps saved-form conflicts visible without losing changes',async()=>{
 call.mockRejectedValueOnce(Error('暂时无法连接'));
 render(<TimedPowerPage disabled/>);const user=userEvent.setup();await screen.findByRole('alert');
 await user.click(screen.getByRole('button',{name:'重试'}));await screen.findByRole('group',{name:'倒计时时长'});expect(screen.queryByRole('alert')).toBeNull();
 enter('分','2');call.mockRejectedValueOnce(Error('设置已变更'));
 await user.click(screen.getByRole('button',{name:'保存设置'}));await screen.findByRole('alert');expect(field('分').value).toBe('2');
});

it('exposes the same three-segment fields and confirmation in English',async()=>{
 await setApplicationLanguage('en-US');render(<TimedPowerPage/>);const user=userEvent.setup();
 await screen.findByRole('switch',{name:'Force close applications'});
 expect(within(screen.getByRole('group',{name:'Duration'})).getAllByRole('spinbutton').map(e=>(e as HTMLInputElement).value)).toEqual(['1','0','0']);
 await user.click(screen.getByRole('button',{name:'Start timer'}));
 const dialog=await screen.findByRole('dialog');expect(dialog.textContent).toContain('Unsaved work may be lost');
 await user.click(within(dialog).getByRole('button',{name:'Keep editing'}));
 expect(call.mock.calls.some(([method])=>method==='start')).toBe(false);
});
