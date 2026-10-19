// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIUsage} from "./ai-usage";

it("filters requests by gateway key, including calls made without a key",async()=>{
 call.mockResolvedValue({...empty,callerKeys:[{id:'public-hash',callerKeyId:'public-hash',callerKeyName:'Fixture gateway'}]});
 const user=userEvent.setup();render(<AIUsage active providers={[]}/>);
 await user.click(screen.getByRole('tab',{name:'请求明细'}));
 await user.click(await screen.findByRole('combobox',{name:'网关密钥'}));
 await user.click(screen.getByRole('option',{name:'Fixture gateway'}));
 await waitFor(()=>expect(call).toHaveBeenLastCalledWith('ai.usage.get',expect.objectContaining({callerKey:'public-hash',offset:0})));
 // Wait for the popup to return focus before opening it again. An immediate
 // second click races Base UI's deferred focus restoration in jsdom.
 const selector=screen.getByRole('combobox',{name:'网关密钥'});
 await waitFor(()=>expect(document.activeElement).toBe(selector));
 await user.click(selector);await user.click(await screen.findByRole('option',{name:'未使用网关密钥'}));
 await waitFor(()=>expect(call).toHaveBeenLastCalledWith('ai.usage.get',expect.objectContaining({callerKey:'none'})));
});

it("shows reported plan credits including zero and a separate first-token column",async()=>{
 call.mockResolvedValue({...empty,sum:{...empty.sum,credits:0,credited:1},rows:[{t:'2026-10-17T00:00:00Z',provider:'one',agent:'winone',model:'fixture',in:3,out:2,ms:1500,ttft_ms:125,credits:0,status:200,priced:false,cost:0}]});
 const user=userEvent.setup();render(<AIUsage active providers={[]}/>);
 await screen.findByText('套餐积分');
 await user.click(screen.getByRole('tab',{name:'请求明细'}));
 const table=await screen.findByRole('table');
 expect(within(table).getByRole('columnheader',{name:'首字延迟'})).toBeTruthy();
 expect(within(table).getByText('0.13 s',{exact:true})).toBeTruthy();
 expect(within(table).getByText('0 套餐积分',{exact:true})).toBeTruthy();
});
const call=vi.hoisted(()=>vi.fn());vi.mock("../bridge",()=>({captureCall:call,chatCall:(method:string,params:unknown)=>call("chat."+method,params)}));
const empty={rows:[],sum:{calls:0,errors:0,input:0,output:0,cache_read:0,cache_write:0,cost:0,unpriced:0},offset:0,limit:15,total:0,by:{}};
it('reads saved API-key balances and preserves named parts, zero and failed components',async()=>{
 call.mockImplementation(async(method)=>method==='ai.accounts.quota'?{quotas:[{provider:'deepseek',name:'DeepSeek',windows:[],balance:'¥12.00',balanceSplit:[{label:'Granted',text:'¥0.00'},{label:'Topped up',text:'¥12.00'}],balanceParts:[{label:'Wallet',text:'¥12.00'},{label:'Credits',text:'—',error:'Unavailable'}]}]}:empty);
 render(<AIUsage active providers={[{id:'deepseek',name:'DeepSeek',authMode:'api-key',baseUrl:'https://api.deepseek.com/v1',protocol:'chat',models:[],hasKey:true}]}/>);
 await screen.findByText('¥0.00',{exact:true});
 expect(screen.getByText('赠送',{exact:true})).toBeTruthy();expect(screen.getByText('充值',{exact:true})).toBeTruthy();
 expect(screen.getByText('Wallet',{exact:true})).toBeTruthy();expect(screen.getByText('Credits',{exact:true})).toBeTruthy();
 expect(call).toHaveBeenCalledWith('ai.accounts.quota',{provider:'deepseek',agent:undefined});
});
afterEach(()=>{cleanup();call.mockReset();});

it("uses the saved allowance display and account masking choices",async()=>{
 call.mockImplementation(async(method)=>method==="ai.accounts.quota"?{quotas:[{provider:"codex",name:"Codex",user:"fixture@example.invalid",windows:[{name:"Five hours",used:25}]}]}:empty);
 render(<AIUsage active preferences={{quotaLeft:false,hideAccounts:false}} providers={[{id:"saved-account",name:"Configured account",authMode:"account",agent:"codex",models:[],hasKey:false}]}/>);
 expect(await screen.findByText("25.0% 已用")).toBeTruthy();expect(screen.getByText(/fixture@example.invalid/)).toBeTruthy();
});

it("renders Magpie's request heatmap and its recorded daily totals",async()=>{
 call.mockResolvedValue({...empty,heatmap:{from:"2026-10-05",to:"2026-10-11",days:[{date:"2026-10-11",calls:2,tokens:1250,cost:0.03,unpriced:0}]}});
 render(<AIUsage active providers={[]}/>);
 await userEvent.setup().click(screen.getByRole("tab",{name:"请求明细"}));
 const map=await screen.findByRole("img",{name:"请求活动热图"});
 expect(map.querySelectorAll("i[data-date]")).toHaveLength(7);
 await userEvent.setup().hover(map.querySelector<HTMLElement>('i[data-date="2026-10-11"]')!);
 expect(await screen.findByText(/2 次请求/)).toBeTruthy();
});

it("keeps the All projects filter when an empty catalog serializes as null",async()=>{
 // Go marshals an empty slice as null. The Sessions page must treat that as a
 // valid zero-project state instead of reading projects.map on null.
 const counts={sessions:0,messages:0,calls:0,input:0,output:0,cache_read:0,cache_write:0,reasoning:0,errors:0,compactions:0,activeMs:0,cost:0,unpriced:0};
 // The real Host encodes an empty catalog as null for every list field.
 call.mockImplementation(async(method)=>method==="chat.catalog"?{projects:null,conversations:[]}:method==="chat.usage"?{rows:null,sum:counts,series:null,by:null,total:0,offset:0,limit:15,median:0,p90:0,top:null,tools:null,skills:null,warnings:null,shape:null}:empty);
 const user=userEvent.setup();render(<AIUsage active providers={[]}/>);
 await user.click(screen.getByRole("tab",{name:"会话"}));
 const project=await screen.findByRole("combobox",{name:"项目"});
 expect(project.textContent).toContain("全部项目");
 expect(await screen.findByText("此范围内还没有会话。")).toBeTruthy();
});

it("applies an explicit inclusive date range to usage reads",async()=>{
 call.mockResolvedValue(empty);const user=userEvent.setup();render(<AIUsage active providers={[]}/>);
 await user.click(screen.getByRole("combobox",{name:"时间范围"}));
 await user.click(await screen.findByRole("option",{name:"自选日期"}));
 fireEvent.change(screen.getByLabelText("起始日期"),{target:{value:"2026-10-08"}});
 fireEvent.change(screen.getByLabelText("截止日期"),{target:{value:"2026-10-09"}});
 await user.click(screen.getByRole("button",{name:"应用日期"}));
 await waitFor(()=>expect(call).toHaveBeenLastCalledWith("ai.usage.get",expect.objectContaining({period:"2026-10-08..2026-10-09",offset:0})));
});
it("loads only while active and does not let a late old filter replace the current one",async()=>{
 let first!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(resolve=>{first=resolve})).mockResolvedValue({...empty,total:1,sum:{...empty.sum,calls:1,errors:1}});
 const view=render(<AIUsage active={false} providers={[]}/>);expect(call).not.toHaveBeenCalled();
 view.rerender(<AIUsage active providers={[]}/>);await waitFor(()=>expect(call).toHaveBeenCalledOnce());
 await userEvent.setup().click(screen.getByRole("checkbox",{name:"仅显示失败请求"}));
 await waitFor(()=>expect(call).toHaveBeenLastCalledWith("ai.usage.get",expect.objectContaining({failed:true,offset:0})));
 await screen.findByText("失败 · 1");await act(async()=>first(empty));
 expect(screen.getByText("失败 · 1")).toBeTruthy();
});
it("shows read failures and allows an explicit retry without saving settings",async()=>{
 call.mockRejectedValueOnce(Error("无法读取统计")).mockResolvedValue(empty);
 render(<AIUsage active providers={[]}/>);await screen.findByRole("alert");
 await userEvent.setup().click(screen.getByRole("button",{name:"重试"}));
 await screen.findAllByText("此范围内还没有请求记录。");expect(screen.queryByRole("alert")).toBeNull();
 expect(call.mock.calls.every(([method])=>method==="ai.usage.get")).toBe(true);
});
it("keeps usage totals visible when model prices cannot be updated",async()=>{
 call.mockResolvedValue({...empty,priceWarning:true,sum:{...empty.sum,calls:2,input:40,output:10,unpriced:2}});
 render(<AIUsage active providers={[]}/>);
 await screen.findByText("模型价格更新失败。已有价格继续用于估算，未知价格不计入费用。请刷新重试。");
 expect(screen.getByText("40",{exact:true})).toBeTruthy();
 expect(screen.getByText("10",{exact:true})).toBeTruthy();
});

it("defaults to seven days and keeps request pages to fifteen with complete export",async()=>{
 const rows=Array.from({length:15},(_,index)=>({t:new Date(2026,9,10,0,index).toISOString(),provider:"fixture",agent:"winone",model:"model-"+index,in:10,out:2,ms:1000,status:200,priced:true,cost:0.001}));
 call.mockImplementation(async(method,params)=>method==="ai.usage.export"?{state:"saved",path:"chosen.csv",rows:16}:{...empty,rows:params.offset?rows.slice(0,1):rows,total:16,offset:params.offset||0,sum:{...empty.sum,calls:16}});
 const user=userEvent.setup();render(<AIUsage active providers={[]}/>);
 await waitFor(()=>expect(call).toHaveBeenCalledWith("ai.usage.get",expect.objectContaining({period:"7d",offset:0})));
 await user.click(screen.getByRole("tab",{name:"请求明细"}));
 const table=await screen.findByRole("table");expect(within(table).getAllByRole("row")).toHaveLength(16);
 await user.click(screen.getByRole("button",{name:"下一页"}));
 await waitFor(()=>expect(call).toHaveBeenLastCalledWith("ai.usage.get",expect.objectContaining({offset:15})));
 expect(await screen.findByText("第 16–16 条，共 16 条")).toBeTruthy();
 await user.click(screen.getByRole("button",{name:"导出 CSV"}));
 expect(await screen.findByText("已将 16 条请求保存到 chosen.csv")).toBeTruthy();
});

it("does not report a cancelled save or a raw CSV response as a saved export",async()=>{
 let exportResult:unknown={state:"cancelled"};
 call.mockImplementation(async(method)=>method==="ai.usage.export"?exportResult:{...empty,total:1});
 const user=userEvent.setup();render(<AIUsage active providers={[]}/>);await user.click(screen.getByRole("tab",{name:"请求明细"}));
 const save=await screen.findByRole("button",{name:"导出 CSV"});await user.click(save);
 expect(screen.queryByText(/已将 .* 条请求保存到/)).toBeNull();expect(screen.queryByRole("alert")).toBeNull();
 exportResult={csv:"data without a saved file"};await user.click(save);
 expect(await screen.findByRole("alert")).toBeTruthy();expect(screen.queryByText(/已将 .* 条请求保存到/)).toBeNull();
});

it("reads configured quota accounts and never invents a zero allowance on failure",async()=>{
 call.mockImplementation(async(method)=>{if(method==="ai.accounts.quota")throw Error("额度接口离线");return empty;});
 render(<AIUsage active providers={[{id:"saved-account",name:"Configured account",authMode:"account",agent:"codex",models:[],hasKey:false}]}/>);
 await screen.findByText(/额度接口离线/);
 expect(call).toHaveBeenCalledWith("ai.accounts.quota",{provider:"saved-account",agent:"codex"});
 expect(screen.queryByRole("progressbar")).toBeNull();
});

it("labels a retained allowance as a failed refresh instead of a new live reading",async()=>{
 call.mockImplementation(async(method)=>method==="ai.accounts.quota"?{quotas:[{provider:"codex",name:"Codex",asOf:"2026-10-10T10:00:00Z",windows:[{name:"Five hours",used:25}]}]}:empty);
 render(<AIUsage active providers={[{id:"saved-account",name:"Configured account",authMode:"account",agent:"codex",models:[],hasKey:false}]}/>);
 expect(await screen.findByRole("alert")).toBeTruthy();
 expect(screen.getByText(/本次额度读取失败，以下为/)).toBeTruthy();
 expect(screen.getByText("75.0% 剩余")).toBeTruthy();
});

it("keeps one in-flight allowance read while ledger filters reload",async()=>{
 let finish!:(value:unknown)=>void;let quotaReads=0;
 call.mockImplementation(async(method)=>{
  if(method!=="ai.accounts.quota")return empty;
  quotaReads++;
  if(quotaReads>1)throw Error("账号正在登录或使用，请稍后再试");
  return new Promise(resolve=>{finish=resolve;});
 });
 render(<AIUsage active providers={[{id:"saved-account",name:"Configured account",authMode:"account",agent:"codex",models:[],hasKey:false}]}/>);
 await screen.findByText("正在读取额度与余额…");
 await userEvent.setup().click(screen.getByRole("checkbox",{name:"仅显示失败请求"}));
 await act(async()=>finish({quotas:[{provider:"codex",name:"Codex",user:"fixture@example.invalid",windows:[{name:"Five hours",used:25}]}]}));
 expect(await screen.findByRole("progressbar")).toBeTruthy();
 expect(screen.getByText("75.0% 剩余")).toBeTruthy();
 expect(screen.queryByRole("alert")).toBeNull();
 expect(quotaReads).toBe(1);
});

it("opens Context and Sessions using WinOne data and renders classified request parts",async()=>{
 const prompt={window:1000,tokens:100,counted:true,parts:[{kind:"system",tokens:40,items:[{name:"prompt",tokens:40}]},{kind:"chat",tokens:60,items:[{name:"1",tag:"turn",tokens:60}]}]};
 const counts={sessions:1,messages:2,calls:1,input:100,output:20,cache_read:0,cache_write:0,reasoning:0,errors:0,compactions:0,activeMs:1000,cost:0.02,unpriced:0};
 const session={...counts,id:"registered",title:"My chat",provider:"fixture",model:"model",project:"",createdAt:"2026-10-10T01:00:00Z",updatedAt:"2026-10-10T01:00:01Z",archived:false,context:[],tools:{read:1},skills:{},roles:{user:1,assistant:1}};
 call.mockImplementation(async(method)=>{
  if(method==="chat.catalog")return {projects:[],conversations:[{id:"registered",title:"My chat"}]};
  if(method==="chat.usage")return {rows:[session],sum:counts,series:[],by:{model:{model:counts}},total:1,offset:0,limit:15,median:120,p90:120,top:[],tools:{read:1},skills:{},warnings:[],shape:{}};
  return {...empty,context:{agents:[],sessions:[{agent:"fixture",key:"registered",model:"model",requests:1,peak:100,window:1000,latest:prompt,latestId:1,points:[{id:1,time:"2026-10-10T01:00:00Z",tokens:100}]}]},contextTotal:1,contextRequests:[{id:1,time:"2026-10-10T01:00:00Z",agent:"fixture",session:"registered",model:"model",prompt}]};
 });
 const user=userEvent.setup();render(<AIUsage active providers={[]}/>);
 await user.click(screen.getByRole("tab",{name:"上下文"}));
 await user.click(await screen.findByRole("button",{name:/My chat/}));
 expect(screen.getByText("系统提示 · 40 Token")).toBeTruthy();
 expect(screen.getByLabelText("上下文组成").children).toHaveLength(400);
 await user.click(screen.getByRole("tab",{name:"会话"}));
 await screen.findByText("最近会话");
 expect(call).toHaveBeenCalledWith("chat.usage",expect.objectContaining({period:"7d",offset:0}));
 expect(screen.getByText("$0.020000")).toBeTruthy();
});
