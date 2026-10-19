// Source editor diagnostics feed one visible status surface in its host.
import {useEffect,useState} from 'react';
const eventName='winone:document-feedback';
export const toast={error:(message:unknown)=>window.dispatchEvent(new CustomEvent(eventName,{detail:String(message)}))};
export function createLogger(name:string){return {warn:(...args:unknown[])=>console.warn(name,...args),error:(...args:unknown[])=>console.error(name,...args)};}
export function DocumentFeedback(){const [message,setMessage]=useState('');useEffect(()=>{const receive=(event:Event)=>setMessage((event as CustomEvent<string>).detail);window.addEventListener(eventName,receive);return()=>window.removeEventListener(eventName,receive);},[]);return message?<p role="alert" className="chat-preview-feedback">{message}</p>:null;}
