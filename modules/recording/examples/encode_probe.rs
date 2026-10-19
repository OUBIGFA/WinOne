//! Synthetic frames only. Run through build/test-recording.ps1; this probe
//! never captures the desktop, changes settings or invokes an external encoder.
use std::{fs::{self,File},path::{Path,PathBuf},sync::atomic::Ordering,thread,time::Duration};
use winone_recording::{core::{BufferPool,Config,Frame,Packet,Region,QUEUE_CAPACITY},encoder,session::Shared};
use windows::{core::HSTRING,Win32::{Media::MediaFoundation::*,System::Com::{CoInitializeEx,CoUninitialize,COINIT_MULTITHREADED}}};
fn main()->Result<(),Box<dyn std::error::Error>>{
    let output=PathBuf::from(std::env::args_os().nth(1).ok_or("Pass an isolated output directory")?);fs::create_dir_all(&output)?;
    for format in ["gif","mp4"] {
        let path=output.join(format!("synthetic.{format}"));if path.exists(){return Err("Choose a fresh output directory".into());}File::create(&path)?;
        let c:Config=serde_json::from_value(serde_json::json!({"x":0,"y":0,"width":640,"height":360,"monitor":"fixture","output":path,"format":format}))?;
        let r=Region{x:0,y:0,width:640,height:360};let pool=BufferPool::new(r.bytes()?)?;let state=Shared::new(&c,r,pool.bytes);let stats=state.clone();
        let (tx,rx)=crossbeam_channel::bounded(QUEUE_CAPACITY);let (ready_tx,ready_rx)=crossbeam_channel::bounded(1);
        let target=path.clone();let worker=thread::spawn(move||encoder::encode(&c,r,&target,rx,state,ready_tx));
        ready_rx.recv_timeout(Duration::from_secs(20))??;
        // WGC may deliver its first frame after encoder/capture startup. That
        // latency must not shorten the saved timeline or disagree with status.
        for (index,time) in [250,400,1700].into_iter().enumerate(){
            let mut buffer=pool.take().ok_or("Unexpected buffer starvation")?;
            for y in 0..r.height as usize {for x in 0..r.width as usize {let p=&mut buffer.bytes[(y*r.width as usize+x)*4..][..4];p.copy_from_slice(if y<r.height as usize/2 {if x<r.width as usize/2 {&[0,0,240,255]}else{&[0,240,0,255]}}else{if x<r.width as usize/2{&[240,0,0,255]}else{&[220,220,220,255]}});}}
            // Change a central square while keeping orientation corner markers.
            for y in 36..60 {for x in 52..78 {buffer.bytes[(y*r.width as usize+x)*4..][..3].fill((index*90) as u8);}}
            tx.send(Packet::Frame(Frame{buffer,pts:Duration::from_millis(time)}))?;
        }
        tx.send(Packet::End(Duration::from_secs(6)))?;drop(tx);
        let encoded=worker.join().map_err(|_|"Encoder panicked")?;
        if let Err(error)=&encoded {eprintln!("{format}: {error}; {} samples accepted",stats.encoded.load(Ordering::Relaxed));}
        encoded?;
        assert_eq!(stats.encoded.load(Ordering::Relaxed),3);
        let evidence=if format=="gif"{read_gif(&path)?}else{read_mp4(&path)?};
        println!("{}",serde_json::json!({"format":format,"file":path,"bytes":fs::metadata(&path)?.len(),"evidence":evidence}));
    }
    Ok(())
}
fn read_gif(path:&Path)->Result<serde_json::Value,Box<dyn std::error::Error>>{
    let mut options=gif::DecodeOptions::new();options.set_color_output(gif::ColorOutput::RGBA);let mut reader=options.read_info(File::open(path)?)?;
    assert_eq!((reader.width(),reader.height()),(640,360));let mut frames=0;let mut ticks=0u64;
    while let Some(frame)=reader.read_next_frame()?{frames+=1;ticks+=frame.delay as u64;}
    assert!((598..=602).contains(&ticks),"GIF static tail was truncated or extended: {ticks}");
    Ok(serde_json::json!({"frames":frames,"durationMs":ticks*10}))
}
fn read_mp4(path:&Path)->Result<serde_json::Value,Box<dyn std::error::Error>>{
    unsafe {CoInitializeEx(None,COINIT_MULTITHREADED).ok()?;MFStartup(MF_VERSION,MFSTARTUP_FULL)?;}
    let result=(||->Result<serde_json::Value,Box<dyn std::error::Error>>{
        let reader=unsafe{MFCreateSourceReaderFromURL(&HSTRING::from(path.as_os_str()),None)}?;
        let mut samples=Vec::new();
        loop {let mut flags=0;let mut timestamp=0;let mut sample=None;unsafe{reader.ReadSample(MF_SOURCE_READER_FIRST_VIDEO_STREAM.0 as u32,0,None,Some(&mut flags),Some(&mut timestamp),Some(&mut sample))}?;
            if let Some(sample)=sample{samples.push((timestamp,unsafe{sample.GetSampleDuration()}?));}
            if flags & MF_SOURCE_READERF_ENDOFSTREAM.0 as u32 != 0{break;}
        }
        let end=samples.last().map(|(pts,duration)|(pts+duration)/10_000).unwrap_or(0);
        assert!((5980..=6020).contains(&end),"MP4 static tail was truncated: {samples:?}");
        assert!(samples.len()<=4,"MP4 unexpectedly expanded VFR into CFR: {} samples",samples.len());
        Ok(serde_json::json!({"samples":samples,"durationMs":end}))
    })();
    unsafe {MFShutdown()?;CoUninitialize();}result
}
