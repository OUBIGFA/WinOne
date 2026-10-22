// @vitest-environment jsdom
import {act,cleanup,renderHook,waitFor} from '@testing-library/react';
import {afterEach,expect,it} from 'vitest';
import {useIsDarkMode} from './useIsDarkMode';

afterEach(()=>{cleanup();delete document.documentElement.dataset.theme;document.documentElement.classList.remove('dark');});

it('follows the native theme on mount and while an editor stays open',async()=>{
 document.documentElement.dataset.theme='dark';
 const {result}=renderHook(()=>useIsDarkMode());
 expect(result.current).toBe(true);
 act(()=>{document.documentElement.dataset.theme='light';document.documentElement.classList.add('dark');});
 await waitFor(()=>expect(result.current).toBe(false));
 act(()=>{document.documentElement.dataset.theme='dark';});
 await waitFor(()=>expect(result.current).toBe(true));
});

it('keeps class-only Cindy surfaces compatible when no native theme is set',async()=>{
 const {result}=renderHook(()=>useIsDarkMode());
 expect(result.current).toBe(false);
 act(()=>{document.documentElement.classList.add('dark');});
 await waitFor(()=>expect(result.current).toBe(true));
});
