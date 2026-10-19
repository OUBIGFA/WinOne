// @vitest-environment jsdom
import {act,cleanup,renderHook,waitFor} from '@testing-library/react';
import {afterEach,beforeEach,describe,expect,it} from 'vitest';
import {useChatTheme} from './chat-desktop-kit';

let previousTheme:string|undefined,previousDark:boolean;
beforeEach(()=>{
 previousTheme=document.documentElement.dataset.theme;
 previousDark=document.documentElement.classList.contains('dark');
 document.documentElement.classList.remove('dark');
 delete document.documentElement.dataset.theme;
});
afterEach(()=>{
 cleanup();
 if(previousTheme===undefined)delete document.documentElement.dataset.theme;
 else document.documentElement.dataset.theme=previousTheme;
 document.documentElement.classList.toggle('dark',previousDark);
});

describe('Chat theme follows the native application',()=>{
 it('opens dark surfaces from data-theme without a dark class',()=>{
  document.documentElement.dataset.theme='dark';
  expect(renderHook(useChatTheme).result.current).toBe('dark');
 });
 it('updates an already open Chat when the native theme changes',async()=>{
  document.documentElement.dataset.theme='light';
  const {result}=renderHook(useChatTheme);
  expect(result.current).toBe('light');
  act(()=>{document.documentElement.dataset.theme='dark';});
  await waitFor(()=>expect(result.current).toBe('dark'));
  act(()=>{document.documentElement.dataset.theme='light';});
  await waitFor(()=>expect(result.current).toBe('light'));
 });
 it('prefers the native attribute and retains class-only embedding support',async()=>{
  document.documentElement.classList.add('dark');
  document.documentElement.dataset.theme='light';
  const {result}=renderHook(useChatTheme);
  expect(result.current).toBe('light');
  act(()=>{delete document.documentElement.dataset.theme;});
  await waitFor(()=>expect(result.current).toBe('dark'));
 });
});
