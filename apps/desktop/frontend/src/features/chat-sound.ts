// Completion feedback stays local to this window: a short two-tone WebAudio
// chime, remembered per profile like the other composer preferences.
const soundKey="winone.chat.sound.v1";
let audioContext:AudioContext|undefined;
function audio():AudioContext|undefined{
 if(typeof window==="undefined")return undefined;
 const Ctor=window.AudioContext;
 if(typeof Ctor!=="function")return undefined;
 audioContext??=new Ctor();
 if(audioContext.state==="suspended")void audioContext.resume();
 return audioContext;
}
export function readChatSoundEnabled(){try{const raw=localStorage.getItem(soundKey);return raw===null?true:raw!=="false";}catch{return true;}}
export function writeChatSoundEnabled(value:boolean){try{localStorage.setItem(soundKey,value?"true":"false");}catch{}}
export function unlockChatAudio(){try{audio();}catch{}}
export function playChatDone(){
 try{
  const context=audio();if(!context)return;
  const start=context.currentTime;
  for(const [offset,frequency] of [[0,523.25],[0.18,659.25]] as const){
   const oscillator=context.createOscillator(),gain=context.createGain();
   oscillator.type="sine";oscillator.frequency.value=frequency;
   gain.gain.setValueAtTime(0.18,start+offset);
   gain.gain.exponentialRampToValueAtTime(0.001,start+offset+0.45);
   oscillator.connect(gain).connect(context.destination);
   oscillator.start(start+offset);oscillator.stop(start+offset+0.5);
  }
 }catch{}
}
