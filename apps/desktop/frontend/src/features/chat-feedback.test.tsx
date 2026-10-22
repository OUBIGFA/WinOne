// @vitest-environment jsdom
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {afterEach,expect,it} from 'vitest';
import {ChatFeedback} from './chat-feedback';
import {chatErrorKind,chatErrorSummary} from './chat-error';
import {chatMessages} from '../locales/chat';
afterEach(cleanup);
const socket='The socket connection was closed unexpectedly. For more information, pass `verbose: true` in the second argument to fetch()';
it.each([
 [socket,'network'],['API Error: upstream stream error: terminated','network'],['ECONNRESET','network'],['503 Service Unavailable','network'],
 ['Request timed out','timeout'],['401 Unauthorized: fetch failed','auth'],['429 rate limit: network error','rateLimit'],['AbortError: operation aborted','cancelled'],['不支持的模块操作 (unsupported)','unsupported'],['app_session_terminated','unknown'],
])('classifies %s without confusing unrelated failures', (message,kind)=>{expect(chatErrorKind(message)).toBe(kind);});
it('shows a useful summary, preserves original diagnostics collapsed, and keeps a repeated error dismissed',()=>{
 const view=render(<ChatFeedback message={socket} role="alert"/>);
 expect(screen.getByText(chatMessages.zh.error_network)).toBeTruthy();
 const details=screen.getByText(socket).closest('details')!;expect(details.open).toBe(false);
 fireEvent.click(screen.getByText('查看原始错误'));expect(details.open).toBe(true);
 fireEvent.click(screen.getByRole('button',{name:'关闭'}));view.rerender(<ChatFeedback message={socket} role="alert"/>);expect(screen.queryByRole('alert')).toBeNull();
 view.rerender(<ChatFeedback message="ETIMEDOUT" role="alert"/>);expect(screen.getByText(chatMessages.zh.error_timeout)).toBeTruthy();
});
it('localizes both languages without promising an automatic retry and preserves specific product instructions',()=>{
 for(const language of ['zh','en'] as const){
  const translate=(key:string)=>(chatMessages[language] as Record<string,string>)[key.replace('chat.','')];
  const hint=chatMessages[language].saveFileBeforeNavigate;
  expect(chatErrorSummary(hint,translate)).toBe(hint);
  expect(chatErrorSummary(chatMessages[language].draftLoadFailed+': '+socket,translate)).toBe(chatMessages[language].draftLoadFailed);
  const text=chatErrorSummary(socket,translate);
  expect(text).toBe(chatMessages[language].error_network);expect(text).not.toContain('verbose');expect(text).not.toMatch(/正在重试|automatically retrying/i);
 }
 expect(chatErrorSummary('文件已在外部修改，请重新读取。',key=>key)).toBe('文件已在外部修改，请重新读取。');
});
it('does not reinterpret normal status messages as errors',()=>{render(<ChatFeedback message="Request timed out is quoted here"/>);expect(screen.getByText('Request timed out is quoted here')).toBeTruthy();expect(screen.queryByText('查看原始错误')).toBeNull();});
