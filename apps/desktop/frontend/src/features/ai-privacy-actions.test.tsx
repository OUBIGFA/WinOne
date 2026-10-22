// @vitest-environment jsdom
import {useState} from 'react';
import {afterEach,expect,it} from 'vitest';
import {cleanup,render,screen,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {AIPrivacyActions,AIPrivacyRules} from './ai-privacy-actions';
import type {MagpieSettings} from './ai-configuration';
afterEach(cleanup);
it('edits existing rules and retains per-kind actions and allowed values',async()=>{
 let current:MagpieSettings={};
 function Fixture(){const [settings,set]=useState<MagpieSettings>({redact:true,redactRules:[{kind:'TOKEN_X',prefix:'before_'}]});current=settings;const onChange=(patch:Partial<MagpieSettings>)=>set(s=>({...s,...patch}));return <><AIPrivacyRules settings={settings} onChange={onChange} disabled={false}/><AIPrivacyActions settings={settings} onChange={onChange} disabled={false}/><button>Other</button></>;}
 const user=userEvent.setup();render(<Fixture/>);
 await user.click(screen.getByRole('button',{name:'编辑脱敏规则 1'}));
 await user.clear(screen.getByLabelText('匹配内容'));await user.type(screen.getByLabelText('匹配内容'),'after_');
 await user.click(screen.getByRole('button',{name:'应用规则编辑'}));expect(current.redactRules).toEqual([{kind:'TOKEN_X',prefix:'after_'}]);
 await user.click(screen.getByText('按类型处理敏感信息',{selector:'summary'}));
 const group=screen.getByRole('radiogroup',{name:'API 密钥 · API_KEY'});
 await user.click(within(group).getByRole('radio',{name:'阻止'}));
 await user.type(screen.getByLabelText('始终原样发送'),'ops@example.invalid, test-key');await user.click(screen.getByText('Other'));
 expect(current.redactActions).toEqual({API_KEY:'block'});expect(current.redactAllow).toEqual(['ops@example.invalid','test-key']);
 await user.click(within(group).getByRole('radio',{name:'脱敏'}));expect(current.redactActions).toEqual({});
});
