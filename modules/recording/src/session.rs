use crate::core::{BufferPool, Config, Format, Packet, Region, Timeline, DISK_RESERVE, QUEUE_CAPACITY};
use crossbeam_channel::{Sender, bounded};
use parking_lot::Mutex;
use serde::Serialize;
use std::{fs::{self, OpenOptions}, path::{Path, PathBuf}, sync::{Arc, atomic::{AtomicBool, AtomicU64, Ordering}}, thread::{self, JoinHandle}, time::{Duration, Instant, SystemTime, UNIX_EPOCH}};
use windows::{Storage::Streams::IRandomAccessStream, Win32::Storage::FileSystem::{GetDiskFreeSpaceExW,MoveFileW}, core::{HSTRING,AgileReference}};

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Status {
    pub phase: String, pub format: Format, pub width: u32, pub height: u32,
    pub duration_ms: u64, pub encoded_frames: u64, pub captured_frames: u64,
    pub skipped_static: u64, pub dropped_backpressure: u64, pub pool_bytes: usize,
    pub max_queue: u64, pub filter_max_us: f64, pub bytes: u64,
    pub path: Option<String>, pub reason: Option<String>, pub error: Option<String>,
}
pub struct Shared {
    pub start: Instant, pub clock: Mutex<Timeline>, pub status: Mutex<Status>,
    pub encoded: AtomicU64, pub submitted: AtomicU64, pub captured: AtomicU64, pub skipped: AtomicU64, pub dropped: AtomicU64,
    pub max_queue: AtomicU64, pub filter_max_ns: AtomicU64, pub cancel: AtomicBool,
    pub stream: Mutex<Option<AgileReference<IRandomAccessStream>>>,
}
impl Shared {
    pub fn new(config: &Config, region: Region, pool_bytes: usize) -> Arc<Self> {
        Arc::new(Self {
            start: Instant::now(), clock: Mutex::new(Timeline::default()),
            status: Mutex::new(Status {phase:"starting".into(),format:config.format,width:region.width,height:region.height,duration_ms:0,encoded_frames:0,captured_frames:0,skipped_static:0,dropped_backpressure:0,pool_bytes,max_queue:0,filter_max_us:0.,bytes:0,path:None,reason:None,error:None}),
            encoded: AtomicU64::new(0), submitted:AtomicU64::new(0), captured:AtomicU64::new(0),skipped:AtomicU64::new(0),dropped:AtomicU64::new(0),max_queue:AtomicU64::new(0),filter_max_ns:AtomicU64::new(0),cancel:AtomicBool::new(false),stream:Mutex::new(None),
        })
    }
    pub fn snapshot(&self) -> Status {
        let pts = self.clock.lock().pts(self.start.elapsed());
        let mut s = self.status.lock().clone();
        s.duration_ms = pts.as_millis().min(u64::MAX as u128) as u64;
        s.encoded_frames = self.encoded.load(Ordering::Relaxed);s.captured_frames=self.captured.load(Ordering::Relaxed);
        s.skipped_static=self.skipped.load(Ordering::Relaxed);s.dropped_backpressure=self.dropped.load(Ordering::Relaxed);
        s.max_queue=self.max_queue.load(Ordering::Relaxed);s.filter_max_us=self.filter_max_ns.load(Ordering::Relaxed) as f64/1000.;s
    }
    pub fn phase(&self, phase: &str) { self.status.lock().phase = phase.into(); }
}
#[derive(Clone, Copy)]
pub enum Control { Stop, Cancel }
pub struct Session { pub shared: Arc<Shared>, controls: Sender<Control>, worker: Option<JoinHandle<()>> }
impl Session {
    pub fn start(config: Config) -> Result<Self, String> {
        let (monitor, region) = crate::capture::resolve_monitor(&config)?;
        let output = Output::reserve(&config)?;
        let pool = match BufferPool::new(region.bytes()?) { Ok(pool)=>pool,Err(e)=>{let _=fs::remove_file(&output.working);return Err(e);} };
        let shared=Shared::new(&config,region,pool.bytes);
        let (controls, commands)=bounded(1);let (ready_tx,ready_rx)=bounded(1);let state=shared.clone();
        let worker=thread::Builder::new().name("winone-recording".into()).spawn(move||{
            // One owner stops capture, drains encoding, finalizes and publishes.
            // Auto-stop works even if WGC sends no events for a static desktop.
            let result=(|| -> Result<String,String> {
                let _apartment=crate::encoder::Apartment::new()?;
                let (frames, receiver)=bounded(QUEUE_CAPACITY);
                let (encoding_ready, encoder_ready)=bounded(1);
                let encoding_state=state.clone();let encoding_config=config.clone();let working=output.working.clone();
                let encoder=thread::Builder::new().name("winone-encoder".into()).spawn(move||crate::encoder::encode(&encoding_config,region,&working,receiver,encoding_state,encoding_ready)).map_err(|e|e.to_string())?;
                match encoder_ready.recv() {
                    Ok(Ok(()))=>(),
                    error=>{drop(frames);let _=encoder.join();return Err(match error {Ok(Err(e))=>e,_=>"编码线程初始化失败".into()});}
                }
                let capture=match crate::capture::start(monitor,region,&config,frames.clone(),pool,state.clone()) {
                    Ok(capture)=>capture,
                    Err(error)=>{drop(frames);let _=encoder.join();return Err(error);}
                };
                state.phase("recording");let _=ready_tx.send(Ok(()));
                let mut disk_check=Instant::now();let mut flush_check=Instant::now();
                let mut fault=None;
                let reason=loop {
                    match commands.recv_timeout(Duration::from_millis(100)) {
                        Ok(Control::Cancel)=>{state.cancel.store(true,Ordering::Release);break "cancelled";},
                        Ok(Control::Stop)=>break "user",Err(crossbeam_channel::RecvTimeoutError::Disconnected)=>break "shutdown",Err(_)=>()
                    }
                    if encoder.is_finished() {fault=Some("编码器提前退出，录制已停止".to_string());break "encoderFailed";}
                    if capture.is_finished() {break "sourceClosed";}
                    let pts=state.clock.lock().pts(state.start.elapsed());
                    if config.format==Format::Gif && (pts.as_secs()>=config.gif_max_seconds as u64 || state.submitted.load(Ordering::Relaxed)>=config.gif_max_frames) {break "gifLimit";}
                    if state.captured.load(Ordering::Relaxed)==0 && state.start.elapsed()>Duration::from_secs(15) {fault=Some("未收到屏幕画面，请检查屏幕是否锁定后重试".into());break "noFrames";}
                    if disk_check.elapsed()>=Duration::from_secs(1) {
                        disk_check=Instant::now();
                        match free_space(&output.working) {Ok(bytes) if bytes<DISK_RESERVE=>break "diskLow",Err(error)=>{fault=Some(error);break "diskCheckFailed";},_=>()}
                    }
                    if flush_check.elapsed()>=Duration::from_secs(2) {
                        flush_check=Instant::now();
                        if let Some(stream)=state.stream.lock().clone() {
                            if let Err(error)=stream.resolve().and_then(|stream|stream.FlushAsync()).and_then(|op|op.join()) {fault=Some(format!("写入录制文件失败：{error}"));break "writeFailed";}
                        }
                    }
                };
                state.phase("stopping");
                let end=state.clock.lock().stop(state.start.elapsed());
                let capture_error=capture.stop().err().map(|e|e.to_string());
                // Packet::End is ordered after all accepted frames. It supplies
                // the final static duration, including a completely idle screen.
                let _=frames.send(Packet::End(end));drop(frames);
                let encoded=encoder.join().map_err(|_|"编码线程意外退出".to_string())?;
                state.stream.lock().take();
                if state.cancel.load(Ordering::Acquire) {return Ok("cancelled".into());}
                encoded?;
                if let Some(error)=capture_error.or(fault) {return Err(error);}
                Ok(reason.into())
            })();
            state.clock.lock().stop(state.start.elapsed());
            state.stream.lock().take();
            let result=match result {
                Ok(reason) if reason=="cancelled"=>fs::remove_file(&output.working).map(|_|(reason,None)).map_err(|e|format!("取消后删除未完成文件失败：{}：{e}",output.working.display())),
                Ok(reason)=>output.publish().map(|path|(reason,Some(path))),
                Err(error)=>Err(error),
            };
            match result {
                Ok((reason,path))=>{
                    let mut s=state.status.lock();s.reason=Some(reason);s.phase=if path.is_some(){"completed"}else{"cancelled"}.into();
                    if let Some(path)=path {s.bytes=fs::metadata(&path).map(|m|m.len()).unwrap_or(0);s.path=Some(path.to_string_lossy().into());}
                },
                Err(error)=>{let _=ready_tx.send(Err(error.clone()));let mut s=state.status.lock();s.phase="failed".into();s.error=Some(error);s.path=Some(output.working.to_string_lossy().into());}
            }
        }).map_err(|e|e.to_string())?;
        let mut session=Self{shared,controls,worker:Some(worker)};
        match ready_rx.recv() {Ok(Ok(()))=>Ok(session),Ok(Err(error))=>{session.join();Err(error)},Err(_)=>{session.join();Err("录制启动线程提前退出".into())}}
    }
    pub fn active(&self)->bool {self.worker.as_ref().is_some_and(|w|!w.is_finished())}
    pub fn pause(&self)->Result<Status,String> {let mut c=self.shared.clock.lock();if c.stopped(){return Err("录制已经结束".into());}c.pause(self.shared.start.elapsed());drop(c);self.shared.phase("paused");Ok(self.shared.snapshot())}
    pub fn resume(&self)->Result<Status,String> {let mut c=self.shared.clock.lock();if c.stopped(){return Err("录制已经结束".into());}c.resume(self.shared.start.elapsed());drop(c);self.shared.phase("recording");Ok(self.shared.snapshot())}
    pub fn finish(&mut self,cancel:bool)->Status {if self.active(){let _=self.controls.send(if cancel{Control::Cancel}else{Control::Stop});}self.join();self.shared.snapshot()}
    fn join(&mut self){if let Some(worker)=self.worker.take(){if worker.join().is_err(){self.shared.phase("failed");self.shared.status.lock().error=Some("录制线程意外退出".into());}}}
}
impl Drop for Session {fn drop(&mut self){if self.active(){let _=self.controls.send(Control::Stop);}self.join();}}

pub fn free_space(path:&Path)->Result<u64,String>{
    let directory=path.parent().ok_or("输出目录无效")?;let mut available=0;
    unsafe{GetDiskFreeSpaceExW(&HSTRING::from(directory.as_os_str()),Some(&mut available),None,None)}.map_err(|e|format!("无法检查输出磁盘：{e}"))?;Ok(available)
}
struct Output {working:PathBuf,final_path:PathBuf}
impl Output {
    fn reserve(config:&Config)->Result<Self,String>{
        config.validate()?;let path=PathBuf::from(&config.output);
        if !path.is_absolute(){return Err("请选择绝对输出路径".into());}
        if path.exists(){return Err("目标文件已存在，请选择其他名称；原文件未改动".into());}
        let directory=path.parent().ok_or("输出目录无效")?;
        if !directory.is_dir(){return Err("保存目录不存在，请重新选择".into());}
        if free_space(&path)?<DISK_RESERVE{return Err("磁盘剩余空间不足 200 MB，请更换保存位置".into());}
        let nonce=SystemTime::now().duration_since(UNIX_EPOCH).map_err(|e|e.to_string())?.as_nanos();
        let working=directory.join(format!(".winone-recording-{}-{nonce}.partial",std::process::id()));
        OpenOptions::new().write(true).create_new(true).open(&working).map_err(|e|format!("无法创建录制文件：{e}"))?;
        Ok(Self{working,final_path:path})
    }
    fn publish(&self)->Result<PathBuf,String>{
        // FlushFileBuffers on Windows requires a writable handle, even after
        // the encoder stream has been finalized and closed.
        OpenOptions::new().write(true).open(&self.working).and_then(|f|f.sync_all()).map_err(|e|format!("保存录制文件失败：{e}"))?;
        // MoveFileW is an atomic, non-replacing rename on this same volume.
        // std::fs::rename would replace a destination created during recording.
        unsafe { MoveFileW(&HSTRING::from(self.working.as_os_str()),&HSTRING::from(self.final_path.as_os_str())) }.map_err(|e|format!("录制已收尾，但无法改为目标名称；文件保留在 {}：{e}",self.working.display()))?;
        Ok(self.final_path.clone())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn output() -> Output {
        let root=PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../_temp/claude-recording/rust-tests").join(format!("{}-{}",std::process::id(),SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_nanos()));
        fs::create_dir_all(&root).unwrap();
        Output{working:root.join("recording.partial"),final_path:root.join("recording.mp4")}
    }
    #[test] fn finalization_flushes_and_publishes_the_owned_file() {
        let output=output();fs::write(&output.working,b"encoded video").unwrap();
        assert_eq!(output.publish().unwrap(),output.final_path);
        assert_eq!(fs::read(&output.final_path).unwrap(),b"encoded video");assert!(!output.working.exists());
    }
    #[test] fn finalization_does_not_replace_a_later_created_destination() {
        let output=output();fs::write(&output.working,b"new recording").unwrap();fs::write(&output.final_path,b"user file").unwrap();
        assert!(output.publish().is_err());assert_eq!(fs::read(&output.final_path).unwrap(),b"user file");assert_eq!(fs::read(&output.working).unwrap(),b"new recording");
    }
}
