import {useCallback,useEffect,useRef,useState} from 'react';

type Preferences={nickname:string};
type Native=<T>(method:string,params?:unknown)=>Promise<T>;

// Persist through Host, not localStorage: the desktop document's origin may
// change across launches. Serialize blur/join writes so an old request cannot
// replace a later edit. Read failure leaves the saved value untouched.
export function useChatNickname(native:Native,onError:(message:string)=>void){
 const [nickname,setNickname]=useState(''),[ready,setReady]=useState(false);
 const saved=useRef(''),queue=useRef(Promise.resolve()),mounted=useRef(true);
 const load=useCallback(async()=>{
  const value=await native<Preferences>('preferences.get');
  if(mounted.current){saved.current=value.nickname;setNickname(value.nickname);setReady(true);}
 },[native]);
 useEffect(()=>{
  mounted.current=true;
  void load().catch(error=>{if(mounted.current)onError(String(error));});
  return()=>{mounted.current=false;};
 },[load]);
 const save=(value=nickname)=>{
  const normalized=value.trim();
  const work=queue.current.then(async()=>{
   if(!ready)throw new Error('个人昵称尚未读取，请重新打开闪聊后重试');
   if(saved.current===normalized)return;
   const result=await native<Preferences>('preferences.save',{nickname:normalized});
   saved.current=result.nickname;
  });
  // A failed save is reported to its caller, but does not poison later retries.
  queue.current=work.catch(()=>{});
  return work;
 };
 return {nickname,setNickname,ready,save,load};
}
