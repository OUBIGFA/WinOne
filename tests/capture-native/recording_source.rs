//! Test-only frame acquisition. Session, ABI, clocks, output protection and
//! MP4/GIF encoding are compiled from the unchanged production Rust sources.
use crate::{core::{BufferPool,Config,Frame,Packet,Region},session::Shared};
use crossbeam_channel::Sender;
use std::{sync::{Arc,atomic::{AtomicBool,Ordering}},thread::{self,JoinHandle},time::Duration};

pub fn resolve_monitor(config:&Config)->Result<((),Region),String>{
    config.validate()?;
    let region=Region::resolve(config,(0,0),(32768,32768))?;
    Ok(((),region))
}
pub struct CaptureControl{stop:Arc<AtomicBool>,thread:JoinHandle<()>}
impl CaptureControl{
    pub fn is_finished(&self)->bool{self.thread.is_finished()}
    pub fn stop(self)->Result<(),String>{self.stop.store(true,Ordering::Release);self.thread.join().map_err(|_|"Generated frame worker panicked".into())}
}
pub fn start(_:(),region:Region,config:&Config,frames:Sender<Packet>,pool:BufferPool,shared:Arc<Shared>)->Result<CaptureControl,String>{
    let stop=Arc::new(AtomicBool::new(false));let stopping=stop.clone();let fps=config.frame_rate();
    let worker=thread::spawn(move||{
        let mut index=0u64;
        while !stopping.load(Ordering::Acquire){
            let clock=shared.clock.lock();
            if clock.stopped(){break;}
            let paused=clock.paused();let pts=clock.pts(shared.start.elapsed());drop(clock);
            if !paused{
                shared.captured.fetch_add(1,Ordering::Relaxed);
                if let Some(mut buffer)=pool.take(){
                    for y in 0..region.height as usize{for x in 0..region.width as usize{
                        let color=if y<region.height as usize/2{[20,40,220,255]}else{[220,60,20,255]};
                        buffer.bytes[(y*region.width as usize+x)*4..][..4].copy_from_slice(&color);
                    }}
                    for y in 30..60.min(region.height as usize){for x in 30..60.min(region.width as usize){buffer.bytes[(y*region.width as usize+x)*4]=((index*29)%255) as u8;}}
                    if frames.try_send(Packet::Frame(Frame{buffer,pts})).is_ok(){shared.submitted.fetch_add(1,Ordering::Relaxed);}
                    index+=1;
                }
            }
            thread::sleep(Duration::from_millis(1000/fps as u64));
        }
    });
    Ok(CaptureControl{stop,thread:worker})
}
