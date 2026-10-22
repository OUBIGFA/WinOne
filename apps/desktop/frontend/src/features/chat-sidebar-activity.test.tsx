// @vitest-environment jsdom
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import {ChatSidebar} from './chat-sidebar';
import {ChatProjectFields} from './chat-project-dialog';
import type {Conversation} from './chat-state';
afterEach(()=>{cleanup();localStorage.clear();});
const conversation=(id:string):Conversation=>({id,title:id,directory:'C:/project',sessionDirectory:'C:/sessions',model:{provider:'p',model:'m'},permission:'ask',createdAt:'2026-01-01',updatedAt:'2026-01-01',hasMessages:id!=='empty'});
const props={catalog:{projects:[],conversations:['running','empty','awaiting','paused','failed'].map(conversation)},activeId:'empty',busy:false,archived:false,onArchived:vi.fn(),onChoose:vi.fn(),onNew:vi.fn(),onProject:vi.fn(),onHide:vi.fn(),onSettings:vi.fn(),onAction:vi.fn(),onProjectAction:vi.fn(),onAutomations:vi.fn()};
it('projects real activity for non-selected sessions without inferring it from titles',()=>{
 const view=render(<ChatSidebar {...props} activities={{running:{phase:'running',attention:false},awaiting:{phase:'needs-interaction',attention:false},paused:{phase:'idle',paused:true,attention:false},failed:{phase:'error',attention:true}}}/>);
 expect(screen.getByRole('img',{name:'正在执行任务'})).toBeTruthy();expect(screen.getByRole('img',{name:'等待你的处理'})).toBeTruthy();expect(screen.getByRole('img',{name:'任务已暂停'})).toBeTruthy();expect(screen.getByRole('img',{name:'任务异常，点击查看'})).toBeTruthy();
 expect(screen.getByRole('button',{name:'empty'}).title).toBe('尚未发送任务');
 view.rerender(<ChatSidebar {...props} activities={{running:{phase:'completed',attention:true}}}/>);
 expect(screen.queryByRole('img',{name:'正在执行任务'})).toBeNull();expect(screen.getByRole('img',{name:'任务已完成'})).toBeTruthy();
});
it('does not hide an error behind paused state and treats omitted hasMessages as empty',()=>{
 render(<ChatSidebar {...props} catalog={{...props.catalog,conversations:props.catalog.conversations.map(value=>value.id==='empty'?{...value,hasMessages:undefined}:value)}} activities={{failed:{phase:'error',attention:true,paused:true}}}/>);
 expect(screen.getByRole('img',{name:'任务异常，点击查看'})).toBeTruthy();
 expect(screen.queryByRole('img',{name:'任务已暂停'})).toBeNull();
 expect(screen.getByRole('button',{name:'empty'}).title).toBe('尚未发送任务');
});
it('places every visible activity indicator immediately before the title without an empty status column',()=>{
 const view=render(<ChatSidebar {...props} activities={{running:{phase:'running',attention:false},failed:{phase:'interrupted',attention:false},awaiting:{phase:'needs-interaction',attention:false}}}/>);
 for(const name of ['running','failed','awaiting']){
  const row=screen.getByRole('button',{name});
  expect(row.firstElementChild?.classList.contains('chat-sidebar-leading-status')).toBe(true);
  expect(row.firstElementChild?.nextElementSibling?.classList.contains('chat-sidebar-title')).toBe(true);
 }
 expect(screen.getByRole('button',{name:'empty'}).firstElementChild?.classList.contains('chat-sidebar-title')).toBe(true);
 view.rerender(<ChatSidebar {...props} activeId="failed" activities={{failed:{phase:'interrupted',attention:false}}}/>);
 expect(screen.getByRole('img',{name:'任务已中断'})).toBeTruthy();
});
it('keeps title generation in the single leading status slot',()=>{
 const view=render(<ChatSidebar {...props} titleJobs={{running:{pending:true},empty:{pending:true}}} activities={{running:{phase:'running',attention:false}}}/>);
 for(const id of ['running','empty']){
  const item=view.container.querySelector(`[data-session-id="${id}"]`)!;
  expect(item.querySelectorAll('[data-sidebar-right-status]')).toHaveLength(1);
  expect(item.querySelector('[data-sidebar-right-status]')?.classList.contains('chat-sidebar-leading-status')).toBe(true);
  expect(item.querySelector('.chat-title-status')).toBeNull();
  expect(item.querySelector('[role=status]')?.classList.contains('sr-only')).toBe(true);
 }
});
it('replaces Projects with one Archived heading and reveals standalone archives even when Conversations was collapsed',()=>{
 localStorage.setItem('winone.chat.sections.v1',JSON.stringify({conversations:true}));
 const catalog={projects:[{id:'alpha',name:'Alpha',path:'C:/Alpha'}],conversations:[{...conversation('child'),projectId:'alpha',archived:true},{...conversation('standalone'),archived:true}]};
 const view=render(<ChatSidebar {...props} catalog={catalog}/>);
 expect(screen.getByRole('heading',{name:'项目'})).toBeTruthy();
 fireEvent.click(screen.getByRole('button',{name:'查看归档'}));expect(props.onArchived).toHaveBeenCalledWith(true);
 view.rerender(<ChatSidebar {...props} catalog={catalog} archived/>);
 expect(screen.getAllByRole('heading',{name:'已归档'})).toHaveLength(1);
 expect(screen.queryByRole('heading',{name:'项目'})).toBeNull();expect(screen.queryByRole('heading',{name:'对话'})).toBeNull();
 expect(screen.getByRole('button',{name:'child'})).toBeTruthy();expect(screen.getByRole('button',{name:'standalone'})).toBeTruthy();
});
it('remembers collapsed projects and the standalone section across remounts and empty catalog loads',()=>{
 const catalog={projects:[{id:'alpha',name:'Alpha',path:'C:/Alpha'}],conversations:[{...conversation('child'),projectId:'alpha'},conversation('standalone')]};
 const first=render(<ChatSidebar {...props} catalog={catalog}/>);
 fireEvent.click(screen.getByRole('button',{name:'Alpha'}));fireEvent.click(screen.getByRole('button',{name:'收起对话'}));
 expect(screen.queryByRole('button',{name:'child'})).toBeNull();first.unmount();
 const next=render(<ChatSidebar {...props} catalog={{projects:[],conversations:[]}}/>);next.rerender(<ChatSidebar {...props} catalog={catalog}/>);
 expect(screen.getByRole('button',{name:'Alpha'}).getAttribute('aria-expanded')).toBe('false');
 expect(screen.getByRole('button',{name:'展开对话'}).getAttribute('aria-expanded')).toBe('false');
 fireEvent.change(screen.getByRole('textbox',{name:'搜索对话'}),{target:{value:'standalone'}});fireEvent.change(screen.getByRole('textbox',{name:'搜索对话'}),{target:{value:''}});
 expect(screen.getByRole('button',{name:'展开对话'})).toBeTruthy();
 expect(JSON.parse(localStorage.getItem('winone.chat.collapsed-projects.v1')!)).toEqual(['alpha']);
});
it.each(['null','[]','"invalid"'])('recovers from invalid stored section preferences %s',saved=>{
 localStorage.setItem('winone.chat.sections.v1',saved);render(<ChatSidebar {...props}/>);expect(screen.getByRole('button',{name:'收起对话'})).toBeTruthy();
});
it.each(['open','create'] as const)('shares the composer permission icons in the %s project form',mode=>{
 const view=render(<ChatProjectFields mode={mode} onMode={()=>{}} name="Project" onName={()=>{}} directory="C:/project" onChooseDirectory={()=>{}} busy={false} permission="ask" onPermission={()=>{}}/>);
 for(const icon of ['shield-user','sparkles','triangle-alert'])expect(view.container.querySelector(`svg.lucide-${icon}`)).toBeTruthy();
});
