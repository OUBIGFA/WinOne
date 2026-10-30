// @vitest-environment jsdom
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {TransferPanel} from './interconnect-transfer';
import {interconnectMessages} from '../locales/interconnect';
import {i18n,setApplicationLanguage} from '../i18n';

const ports=vi.hoisted(()=>({call:vi.fn(),files:vi.fn(),directory:vi.fn(),clipboard:vi.fn(),drop:vi.fn()}));
vi.mock('../bridge',()=>({interconnectCall:ports.call,pickFiles:ports.files,pickDirectory:ports.directory,readClipboardFiles:ports.clipboard,setFileDropTarget:ports.drop}));
let tasks:any[];
const result=()=>({id:'cloud-folder',service:'gofile',link:'https://gofile.io/d/Ab12Cd34',qr:'data:image/png;base64,test',expiresAt:new Date(Date.now()+86400000).toISOString()});
beforeEach(async()=>{
 i18n.addResourceBundle('zh-CN','translation',{interconnect:interconnectMessages.zh},true,true);await setApplicationLanguage('zh-CN');
 tasks=[];ports.files.mockResolvedValue(['C:\\isolated\\资料.txt']);ports.drop.mockResolvedValue(undefined);
 Element.prototype.scrollIntoView=vi.fn();
 ports.call.mockImplementation(async(method:string,spec:any)=>{
  if(method==='transfer_list')return structuredClone(tasks);
  if(method==='transfer_start'){tasks.unshift({id:'cloud-task',spec,state:{status:'completed',result:result()}});return {id:'cloud-task'};}
  if(method==='share_revoke')tasks[0].state.result.revoked=true;
 });
});
afterEach(()=>{cleanup();Object.values(ports).forEach(port=>port.mockReset());});

it('defaults to GoFile, uploads in two actions and exposes the result without opening details',async()=>{
 render(<TransferPanel stored active onActivity={()=>{}}/>);const user=userEvent.setup();
 expect(screen.getByRole('tab',{name:'GoFile'}).getAttribute('aria-selected')).toBe('true');
 await user.click(screen.getByRole('button',{name:'选择文件'}));
 await user.click(screen.getByRole('button',{name:'上传并分享'}));
 await screen.findByRole('img',{name:'云端分享接收二维码'});
 expect(ports.call).toHaveBeenCalledWith('transfer_start',expect.objectContaining({mode:'store_send',storeOrigin:'https://gofile.io',paths:['C:\\isolated\\资料.txt']}));
 expect(screen.getByText('https://gofile.io/d/Ab12Cd34')).toBeTruthy();
 expect(screen.getByRole('button',{name:'复制分享链接'})).toBeTruthy();
 expect(Element.prototype.scrollIntoView).toHaveBeenCalled();
 await user.click(screen.getByRole('button',{name:'打开下载页'}));
 expect(ports.call).toHaveBeenCalledWith('transfer_open_cloud',{url:'https://gofile.io/d/Ab12Cd34'});
 await user.click(screen.getByRole('button',{name:'撤销分享'}));
 await screen.findByText('已撤销');expect(screen.queryByRole('img',{name:'云端分享接收二维码'})).toBeNull();
});

it('keeps file selection and custom croc settings while switching cloud services',async()=>{
 render(<TransferPanel stored active onActivity={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole('button',{name:'选择文件'}));
 await user.click(screen.getByRole('tab',{name:'croc 加密分享'}));
 await user.click(screen.getByText('分享设置',{selector:'summary'}));
 fireEvent.change(screen.getByRole('textbox',{name:'云端服务地址'}),{target:{value:'https://storage.example.test'}});
 fireEvent.change(screen.getByRole('spinbutton',{name:'允许完整下载次数'}),{target:{value:'3'}});
 await user.click(screen.getByRole('tab',{name:'GoFile'}));
 expect(screen.queryByRole('spinbutton',{name:'允许完整下载次数'})).toBeNull();
 await user.click(screen.getByRole('tab',{name:'croc 加密分享'}));
 expect((screen.getByRole('textbox',{name:'云端服务地址'}) as HTMLInputElement).value).toBe('https://storage.example.test');
 await user.click(screen.getByRole('button',{name:'上传并分享'}));
 expect(ports.call).toHaveBeenCalledWith('transfer_start',expect.objectContaining({storeOrigin:'https://storage.example.test',downloads:3,paths:['C:\\isolated\\资料.txt']}));
});

it('opens GoFile receive in the browser and automatically selects croc for a pasted croc code',async()=>{
 render(<TransferPanel stored active onActivity={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole('radio',{name:'接收'}));
 fireEvent.change(screen.getByRole('textbox',{name:'口令或分享链接'}),{target:{value:'https://gofile.io/d/Ab12Cd34'}});
 await user.click(screen.getByRole('button',{name:'打开下载页'}));
 expect(ports.call).toHaveBeenCalledWith('transfer_open_cloud',{url:'https://gofile.io/d/Ab12Cd34'});
 expect(ports.call.mock.calls.some(([method])=>method==='transfer_start')).toBe(false);
 fireEvent.change(screen.getByRole('textbox',{name:'口令或分享链接'}),{target:{value:'croc-store-v1.example'}});
 await waitFor(()=>expect(screen.getByRole('tab',{name:'croc 加密分享'}).getAttribute('aria-selected')).toBe('true'));
 expect(screen.getByRole('button',{name:'查看分享内容'})).toBeTruthy();
});

it('retains the draft on start failure and offers revocation for a partial upload',async()=>{
 render(<TransferPanel stored active onActivity={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole('button',{name:'选择文件'}));
 ports.call.mockImplementation(async(method:string)=>{if(method==='transfer_list')return structuredClone(tasks);if(method==='transfer_start')throw new Error('offline');});
 await user.click(screen.getByRole('button',{name:'上传并分享'}));await screen.findByText('Error: offline');
 expect(screen.getByText('C:\\isolated\\资料.txt')).toBeTruthy();
 tasks.push({id:'partial',spec:{mode:'store_send',storeOrigin:'https://gofile.io'},state:{status:'failed',error:'upload failed',result:{id:'folder',service:'gofile'}}});
 await screen.findByRole('button',{name:'撤销分享'});
 expect(screen.queryByRole('img',{name:'云端分享接收二维码'})).toBeNull();
});

it('preserves text and file drafts across payload switches in the primary composer',async()=>{
 render(<TransferPanel stored={false} active onActivity={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole('button',{name:'选择文件'}));
 await user.click(screen.getByRole('radio',{name:'文本'}));
 await user.type(screen.getByRole('textbox',{name:'文本'}),'待发送文本');
 await user.click(screen.getByRole('radio',{name:'文件与文件夹'}));expect(screen.getByText('C:\\isolated\\资料.txt')).toBeTruthy();
 await user.click(screen.getByRole('radio',{name:'文本'}));
 expect((screen.getByRole('textbox',{name:'文本'}) as HTMLTextAreaElement).value).toBe('待发送文本');
 expect(within(screen.getByRole('region',{name:'发送与接收'})).getByRole('button',{name:'发送'})).toBeTruthy();
});
