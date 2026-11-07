import test from 'node:test';
import assert from 'node:assert/strict';
import {guardedOutboundFetch} from './seams/outbound-fetch.ts';

// These checks stop before network dispatch; no fixture exception relaxes the
// public production policy. IPv6-wrapped and legacy IPv4 inputs share the guard.
test('plugin public fetch rejects local and special-use addresses before authorization or traffic', async()=>{
 for(const url of ['https://127.0.0.1/','https://169.254.169.254/','https://[::ffff:7f00:1]/','https://[::1]/','https://10.0.0.1/','https://localhost/','http://example.com/']){
  let authorized=false;
  await assert.rejects(async()=>guardedOutboundFetch(url,{signal:AbortSignal.timeout(1000)},()=>{authorized=true}));
  assert.equal(authorized,false,url);
 }
});

test('plugin request rechecks its live authority after resolution and before dispatch', async()=>{
 const denied=new Error('call already settled');let checked=false;
 await assert.rejects(()=>guardedOutboundFetch('https://1.1.1.1/',{signal:AbortSignal.timeout(1000)},()=>{checked=true;throw denied}),error=>error===denied);
 assert.equal(checked,true);
});
