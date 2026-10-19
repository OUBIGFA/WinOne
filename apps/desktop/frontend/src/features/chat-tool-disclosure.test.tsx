// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {AgentActionRow} from './cindy/AgentActionRow';
import {__test_internals} from './cindy/useExpandedBlockMemory';
afterEach(()=>{cleanup();__test_internals.reset();});

it('keeps repeated terminal rows independent across work groups',()=>{
 const message={clientId:'same-terminal-task',content:'',toolName:'powershell',toolInput:{command:'Get-Date'}};
 render(<><section><AgentActionRow message={message} status="done" toolResult="First output"/></section><section><AgentActionRow message={message} status="done" toolResult="Second output"/></section></>);
 const rows=screen.getAllByRole('button',{name:'powershell Get-Date'});
 fireEvent.click(rows[0]);
 expect(rows[0].getAttribute('aria-expanded')).toBe('true');
 expect(rows[1].getAttribute('aria-expanded')).toBe('false');
 expect(screen.queryByText('Second output')).toBeNull();
});
