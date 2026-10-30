// @vitest-environment jsdom
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, render, renderHook, screen, within} from '@testing-library/react';
import {ModelRouteSelector} from './onebotModelRouteSelector';
import userEvent from '@testing-library/user-event';
import type {AIProvider} from '../../ai-configuration';
import {providerViews} from './modelProviders';
import {publishCindyHost} from './store';
import {useAgentCapabilities} from './agentCapabilities';
import {ModelSelectorContent} from './modelSelector';
import '../../../i18n';

const providers = (): AIProvider[] => [{id:'vendor',name:'Vendor',authMode:'api-key',hasKey:true,
  models:['custom','limited','plain','unknown'],modelMetadata:{
    custom:{levels:['light','balanced','deep'],reasoningEffort:'balanced',contextWindow:128000},
    limited:{levels:['light','balanced','deep'],efforts:['balanced','deep']},
    plain:{reasoning:false},
  }}];
afterEach(()=>{cleanup();publishCindyHost({providers:null});});

it('offers the model’s own levels and respects the narrowed offer without inventing levels',()=>{
  const models=providerViews(providers())[0].models.pi!;
  expect(models[0]).toMatchObject({efforts:['light','balanced','deep'],defaultEffort:'balanced',contextWindow:128000});
  expect(models[1].efforts).toEqual(['balanced','deep']);
  expect(models[2].efforts).toEqual([]);
  expect(models[3].efforts).toEqual([]);
});

it('passes raw vendor effort names and context limits through the shared capability hook',()=>{
  publishCindyHost({providers:providers()});
  const {result}=renderHook(()=>useAgentCapabilities('pi'));
  expect(result.current.capabilities?.availableModels[0].contextWindow).toBe(128000);
  expect(result.current.capabilities?.effortLevels).toEqual([
    {id:'light',displayName:'light'},{id:'balanced',displayName:'balanced'},{id:'deep',displayName:'deep'},
  ]);
});

it('shows source-specific levels and reconciles an unsupported effort when selecting another model',async()=>{
  publishCindyHost({providers:providers()});
  const select=vi.fn(),change=vi.fn();
  render(<ModelSelectorContent vendorKey="pi" modelId="custom" currentProviderId="vendor" effort="light"
    onModelChange={()=>{}} onEffortChange={change} onDismiss={()=>{}} onUnifiedSelect={select}/>);
  const levels=within(screen.getByRole('group'));
  await userEvent.click(levels.getByRole('button',{name:'deep'}));
  expect(change).toHaveBeenCalledWith('deep');
  expect(levels.queryByRole('button',{name:'high'})).toBeNull();
  await userEvent.click(screen.getByRole('option',{name:'limited'}));
  expect(select.mock.calls[0][0]).toMatchObject({providerId:'vendor',modelId:'limited',effort:undefined});
});

it('shows the provider and raw effort in the OneBot route trigger',()=>{
  publishCindyHost({providers:providers()});
  render(<ModelRouteSelector vendorKey="pi" modelId="custom" currentProviderId="vendor" effort="balanced"
    onModelChange={()=>{}} onEffortChange={()=>{}}/>);
  expect(screen.getByRole('button').getAttribute('title')).toBe('Vendor · custom · balanced');
});

it('marks only the resolved source when a legacy route has no provider',()=>{
  const first=providers()[0];
  publishCindyHost({providers:[first,{...first,id:'second',name:'Second vendor'}]});
  render(<ModelSelectorContent vendorKey="pi" modelId="custom" currentProviderId={null} effort="light"
    onModelChange={()=>{}} onEffortChange={()=>{}} onDismiss={()=>{}}/>);
  expect(screen.getAllByRole('option',{selected:true})).toHaveLength(1);
});

it('emits one reconciled route through the non-unified callback before dismissal',async()=>{
  publishCindyHost({providers:providers()});
  const provider=vi.fn(),model=vi.fn(),effort=vi.fn(),dismiss=vi.fn();
  render(<ModelSelectorContent vendorKey="pi" modelId="custom" currentProviderId="vendor" effort="light" fastMode
    onModelChange={model} onEffortChange={effort} onProviderChange={provider} onDismiss={dismiss}/>);
  await userEvent.click(screen.getByRole('option',{name:'plain'}));
  expect(provider).toHaveBeenCalledExactlyOnceWith('vendor','plain','',true);
  expect(model).not.toHaveBeenCalled();expect(effort).not.toHaveBeenCalled();
  expect(dismiss).toHaveBeenCalledTimes(1);
});
