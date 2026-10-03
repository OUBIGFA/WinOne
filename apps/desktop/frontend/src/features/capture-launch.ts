export interface CaptureLaunchRequest {readonly id:string;readonly draftId:string}
export interface CaptureLaunchPort {
  read:()=>Promise<CaptureLaunchRequest|null>;
  acknowledge:(id:string)=>Promise<boolean>;
  subscribe:(listener:()=>void)=>()=>void;
}

// Subscribe before reading the retained native queue. One receiver operation
// runs at a time; duplicate notifications cannot open multiple editor views.
export function startCaptureLaunchReceiver(port:CaptureLaunchPort,accept:(request:CaptureLaunchRequest)=>Promise<boolean>,onError:(error:unknown)=>void) {
  let stopped=false,running=false,notified=false;
  async function drain() {
    if(stopped)return;
    if(running){notified=true;return;}
    running=true;
    try {
      for(let count=0;count<8&&!stopped;count++) {
        notified=false;
        const request=await port.read();
        if(stopped)return;
        if(!request){if(notified)continue;return;}
        if(!request.id||!/^[a-f0-9]{32}$/.test(request.draftId))throw new Error("Invalid capture launch request");
        if(!await accept(request)){notified=false;return;}
        if(stopped)return;
        if(!await port.acknowledge(request.id))return;
      }
    } catch(error) {
      notified=false;
      if(!stopped)onError(error);
    } finally {
      running=false;
      if(notified&&!stopped)void drain();
    }
  }
  const unsubscribe=port.subscribe(()=>void drain());
  void drain();
  return {retry:()=>void drain(),stop:()=>{stopped=true;unsubscribe();}};
}
