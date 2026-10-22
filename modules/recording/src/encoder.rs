//! A bounded adapter around Windows Media Foundation/MediaTranscoder and
//! statically linked gifski. No command-line encoder or sequence images exist.
//! The MSS event/deferral pattern follows windows-capture 2.0.1 (MIT), but uses
//! our bounded packet receiver, true durations, cancellation and pooled memory.
use crate::{core::{Codec, Config, Format, Frame, FrameBuffer, Packet, Region}, session::Shared};
use crossbeam_channel::{Receiver, Sender};
use parking_lot::Mutex;
use std::{cell::UnsafeCell, fs::{File, OpenOptions}, io::{self, Write}, path::Path, sync::{Arc, atomic::Ordering}, thread, time::{Duration, Instant}};
use windows::{Foundation::{TimeSpan, TypedEventHandler}, Media::{Core::{MediaStreamSample,MediaStreamSource,MediaStreamSourceSampleRequestedEventArgs,MediaStreamSourceStartingEventArgs,VideoStreamDescriptor}, MediaProperties::{MediaEncodingProfile,MediaEncodingSubtypes,VideoEncodingProperties,ContainerEncodingProperties}, Transcoding::MediaTranscoder}, Storage::{FileAccessMode,StorageFile,Streams::{IBuffer,IBuffer_Impl}}, System::Threading::{ThreadPool,WorkItemHandler,WorkItemOptions,WorkItemPriority}, Win32::{Foundation::{E_INVALIDARG,E_FAIL}, Media::MediaFoundation::*, System::{Com::CoTaskMemFree,WinRT::{IBufferByteAccess,IBufferByteAccess_Impl,RoInitialize,RoUninitialize,RO_INIT_MULTITHREADED}}}, core::{HSTRING,implement}};

pub fn encode(config:&Config,region:Region,path:&Path,frames:Receiver<Packet>,state:Arc<Shared>,ready:Sender<Result<(),String>>)->Result<(),String>{
    let result=match config.format {Format::Mp4=>mp4(config,region,path,frames,state.clone(),ready.clone()),Format::Gif=>gif(config,region,path,frames,state.clone(),ready.clone())};
    if let Err(error)=&result {let _=ready.try_send(Err(error.clone()));}
    result
}

// Holding a COM sample holds the pool lease. Windows reads the buffer directly;
// it is returned only when the last COM reference disappears, never immediately
// after WriteSample. This avoids both use-after-recycle and raw-frame copies.
#[implement(IBuffer, IBufferByteAccess)]
struct PooledBuffer { frame: UnsafeCell<FrameBuffer>, length:u32 }
// The only exposed operation is a read-only media source. No Rust reference to
// these bytes survives construction; neither capture nor the pool can touch
// them until COM releases this object. SetLength cannot resize the allocation.
unsafe impl Send for PooledBuffer {}
unsafe impl Sync for PooledBuffer {}
impl IBuffer_Impl for PooledBuffer_Impl {
    fn Capacity(&self)->windows::core::Result<u32>{Ok(self.length)}
    fn Length(&self)->windows::core::Result<u32>{Ok(self.length)}
    fn SetLength(&self,value:u32)->windows::core::Result<()>{if value==self.length{Ok(())}else{Err(E_INVALIDARG.into())}}
}
impl IBufferByteAccess_Impl for PooledBuffer_Impl {
    fn Buffer(&self)->windows::core::Result<*mut u8>{Ok(unsafe{(*self.frame.get()).bytes.as_ptr().cast_mut()})}
}
fn ticks(duration:Duration)->TimeSpan{TimeSpan{Duration:(duration.as_nanos()/100).min(i64::MAX as u128) as i64}}
struct Samples {frames:Receiver<Packet>,pending:Option<Frame>,ended:bool,state:Arc<Shared>,region:Region}
impl Samples {
    fn next(&mut self)->windows::core::Result<Option<MediaStreamSample>>{
        loop {
            if self.ended{return Ok(None);}
            let next=self.frames.recv();
            if self.state.cancel.load(Ordering::Acquire){self.pending.take();if next.is_err() || matches!(next,Ok(Packet::End(_))){self.ended=true;}continue;}
            let (mut frame,end)=match next {
                Ok(Packet::Frame(mut frame))=>{
                    // Hold the first available picture from zero, as the GIF
                    // path does. MSS normalizes a nonzero first PTS and would
                    // otherwise cut startup latency from the reported duration.
                    if self.pending.is_none(){frame.pts=Duration::ZERO;}
                    let time=frame.pts;let previous=self.pending.replace(frame);if let Some(previous)=previous{(previous,time)}else{continue;}
                },
                Ok(Packet::End(end))=>{self.ended=true;if let Some(frame)=self.pending.take(){(frame,end)}else{return Ok(None);}},
                Err(_)=>{self.ended=true;self.pending.take();return Ok(None);}
            };
            // MF's packed BGRA buffer descriptor expects bottom-up rows. The
            // source pool is top-down; flip in place without another big buffer.
            let row=self.region.width as usize*4;let height=self.region.height as usize;
            for y in 0..height/2 {let (top,bottom)=frame.buffer.bytes.split_at_mut((height-1-y)*row);top[y*row..(y+1)*row].swap_with_slice(&mut bottom[..row]);}
            let length=frame.buffer.bytes.len() as u32;
            let bytes:IBuffer=PooledBuffer{frame:UnsafeCell::new(frame.buffer),length}.into();
            let sample=MediaStreamSample::CreateFromBuffer(&bytes,ticks(frame.pts))?;
            sample.SetDuration(ticks(end.saturating_sub(frame.pts).max(Duration::from_millis(1))))?;
            self.state.encoded.fetch_add(1,Ordering::Relaxed);
            return Ok(Some(sample));
        }
    }
}
// Screen recordings retain physical dimensions and frame timing. Allocate a
// measured 0.10 bits/pixel/frame instead of the former 0.25 video budget; MF
// still controls quantization and VFR sample durations, with no frame scaling.
fn mp4_bitrate(region:Region,fps:u32)->u32 {
    (u64::from(region.width).saturating_mul(u64::from(region.height)).saturating_mul(u64::from(fps))/10).clamp(1_000_000,80_000_000) as u32
}
fn mp4(config:&Config,region:Region,path:&Path,frames:Receiver<Packet>,state:Arc<Shared>,ready:Sender<Result<(),String>>)->Result<(),String>{
    let _apartment=Apartment::new()?;
    let _mf=MediaFoundation::new()?;
    require_hardware(config.codec)?;
    let result=(||->windows::core::Result<()>{
        // Build a fresh profile as windows-capture does. CreateMp4(HD1080p)
        // carries preset transform properties that can conflict with a replaced
        // video descriptor and fail type negotiation before the first sample.
        let profile=MediaEncodingProfile::new()?;
        let container=ContainerEncodingProperties::new()?;
        container.SetSubtype(&MediaEncodingSubtypes::Mpeg4()?)?;
        profile.SetContainer(&container)?;
        let video=VideoEncodingProperties::new()?;
        video.SetSubtype(&match config.codec {Codec::H264=>MediaEncodingSubtypes::H264()?,Codec::Hevc=>MediaEncodingSubtypes::Hevc()?})?;
        video.SetWidth(region.width)?;video.SetHeight(region.height)?;
        video.SetBitrate(mp4_bitrate(region,config.frame_rate()))?;
        video.FrameRate()?.SetNumerator(config.frame_rate())?;video.FrameRate()?.SetDenominator(1)?;
        video.PixelAspectRatio()?.SetNumerator(1)?;video.PixelAspectRatio()?.SetDenominator(1)?;
        profile.SetVideo(&video)?;
        let input=VideoEncodingProperties::CreateUncompressed(&MediaEncodingSubtypes::Bgra8()?,region.width,region.height)?;
        input.FrameRate()?.SetNumerator(config.frame_rate())?;input.FrameRate()?.SetDenominator(1)?;
        let descriptor=VideoStreamDescriptor::Create(&input)?;
        let source=MediaStreamSource::CreateFromDescriptor(&descriptor)?;
        source.SetBufferTime(ticks(Duration::ZERO))?;source.SetCanSeek(false)?;
        let starting=source.Starting(&TypedEventHandler::<MediaStreamSource,MediaStreamSourceStartingEventArgs>::new(|_,args|{
            args.as_ref().ok_or_else(||windows::core::Error::from(E_INVALIDARG))?.Request()?.SetActualStartPosition(ticks(Duration::ZERO))?;Ok(())
        }))?;
        let samples=Arc::new(Mutex::new(Samples{frames,pending:None,ended:false,state:state.clone(),region}));
        let sample_token=source.SampleRequested(&TypedEventHandler::<MediaStreamSource,MediaStreamSourceSampleRequestedEventArgs>::new(move|_,args|{
            let request=args.as_ref().ok_or_else(||windows::core::Error::from(E_INVALIDARG))?.Request()?;
            let deferral=request.GetDeferral()?;let samples=samples.clone();
            ThreadPool::RunWithPriorityAndOptionsAsync(&WorkItemHandler::new(move |_|{
                let result=samples.lock().next().and_then(|sample|request.SetSample(sample.as_ref()));
                let completed=deferral.Complete();result?;completed?;Ok(())
            }),WorkItemPriority::Normal,WorkItemOptions::None)?;Ok(())
        }))?;
        let stream_result=(||->windows::core::Result<()>{
            let absolute=std::fs::canonicalize(path).map_err(|e|windows::core::Error::new(E_FAIL,e.to_string()))?;
            let name=absolute.to_string_lossy();
            let name=if let Some(unc)=name.strip_prefix(r"\\?\UNC\"){format!(r"\\{unc}")}else{name.strip_prefix(r"\\?\").unwrap_or(&name).to_string()};
            let file=StorageFile::GetFileFromPathAsync(&HSTRING::from(name))?.join()?;
            let stream=file.OpenAsync(FileAccessMode::ReadWrite)?.join()?;
            *state.stream.lock()=Some(windows::core::AgileReference::new(&stream)?);
            let transcoder=MediaTranscoder::new()?;transcoder.SetHardwareAccelerationEnabled(true)?;
            // Publish readiness before Prepare asks for its first input sample;
            // capture must be running to satisfy that asynchronous request.
            let _=ready.send(Ok(()));
            let transcode=transcoder.PrepareMediaStreamSourceTranscodeAsync(&source,&stream,&profile)?.join().map_err(|e|windows::core::Error::new(e.code(),format!("准备编码：{e}")))?;
            if !transcode.CanTranscode()? {return Err(windows::core::Error::new(E_INVALIDARG,format!("系统编码器无法处理所选格式：{:?}",transcode.FailureReason()?)));}
            transcode.TranscodeAsync()?.join().map_err(|e|windows::core::Error::new(e.code(),format!("流式编码：{e}")))?;
            stream.FlushAsync()?.join()?;stream.Close()?;state.stream.lock().take();Ok(())
        })();
        // Break the event ownership cycle even on initialization/codec failure.
        let starting_result=source.RemoveStarting(starting);let sample_result=source.RemoveSampleRequested(sample_token);
        stream_result?;starting_result?;sample_result?;Ok(())
    })();
    result.map_err(|e|format!("MP4 编码失败（{:?}）：{e}。请尝试 H.264 或检查显卡驱动。",config.codec))
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn video_budget_preserves_resolution_and_scales_with_selected_fps() {
        let region=Region{x:0,y:0,width:1920,height:1080};
        assert_eq!(mp4_bitrate(region,30),6_220_800);
        assert_eq!(mp4_bitrate(region,60),12_441_600);
        assert_eq!(mp4_bitrate(Region{width:320,height:200,..region},1),1_000_000);
        assert_eq!(mp4_bitrate(Region{width:u32::MAX,height:u32::MAX,..region},60),80_000_000);
    }
}

pub(crate) struct Apartment;
impl Apartment {pub(crate) fn new()->Result<Self,String>{unsafe{RoInitialize(RO_INIT_MULTITHREADED)}.map_err(|e|e.to_string())?;Ok(Self)}}
impl Drop for Apartment {fn drop(&mut self){unsafe{RoUninitialize()}}}
struct MediaFoundation;
impl MediaFoundation {fn new()->Result<Self,String>{unsafe{MFStartup(MF_VERSION,MFSTARTUP_FULL)}.map_err(|e|e.to_string())?;Ok(Self)}}
impl Drop for MediaFoundation {fn drop(&mut self){let _=unsafe{MFShutdown()};}}
fn require_hardware(codec:Codec)->Result<(),String>{
    let kind=MFT_REGISTER_TYPE_INFO{guidMajorType:MFMediaType_Video,guidSubtype:match codec{Codec::H264=>MFVideoFormat_H264,Codec::Hevc=>MFVideoFormat_HEVC}};
    let mut activations=std::ptr::null_mut();let mut count=0;
    unsafe{MFTEnumEx(MFT_CATEGORY_VIDEO_ENCODER,MFT_ENUM_FLAG_HARDWARE|MFT_ENUM_FLAG_SORTANDFILTER,None,Some(&kind),&mut activations,&mut count)}.map_err(|e|e.to_string())?;
    if !activations.is_null(){for item in unsafe{std::slice::from_raw_parts_mut(activations,count as usize)}{item.take();}unsafe{CoTaskMemFree(Some(activations.cast()))};}
    if count==0{Err(format!("当前系统没有可用的 {codec:?} 硬件编码器；请检查驱动或更换编码格式"))}else{Ok(())}
}

struct GifProgress(Arc<Shared>);
impl gifski::progress::ProgressReporter for GifProgress {
    fn increase(&mut self)->bool{!self.0.cancel.load(Ordering::Acquire)}
    // gifski calls written_bytes once per emitted GIF frame; increase also
    // counts its internally merged inputs, so it is not an encoded-frame count.
    fn written_bytes(&mut self,bytes:u64){self.0.encoded.fetch_add(1,Ordering::Relaxed);self.0.status.lock().bytes=bytes;}
    fn error(&mut self,message:String){self.0.status.lock().error=Some(message);}
}
struct StreamingFile{file:File,last_flush:Instant}
impl Write for StreamingFile {
    fn write(&mut self,bytes:&[u8])->io::Result<usize>{let n=self.file.write(bytes)?;if self.last_flush.elapsed()>=Duration::from_secs(2){self.file.flush()?;self.file.sync_data()?;self.last_flush=Instant::now();}Ok(n)}
    fn flush(&mut self)->io::Result<()>{self.file.flush()?;self.file.sync_data()}
}
fn gif(config:&Config,region:Region,path:&Path,frames:Receiver<Packet>,state:Arc<Shared>,ready:Sender<Result<(),String>>)->Result<(),String>{
    let (collector,writer)=gifski::new(gifski::Settings{width:Some(region.width),height:Some(region.height),quality:config.quality,fast:false,repeat:gifski::Repeat::Infinite}).map_err(|e|e.to_string())?;
    let file=OpenOptions::new().write(true).truncate(true).open(path).map_err(|e|e.to_string())?;
    let writer_state=state.clone();
    let writing=thread::Builder::new().name("winone-gifski-writer".into()).spawn(move||{
        let mut stream=StreamingFile{file,last_flush:Instant::now()};
        writer.write(&mut stream,&mut GifProgress(writer_state)).map_err(|e|e.to_string())?;
        stream.flush().map_err(|e|e.to_string())
    }).map_err(|e|e.to_string())?;
    let _=ready.send(Ok(()));
    let result=(||->Result<(),String>{
        let mut index=0;
        while let Ok(packet)=frames.recv(){
            if state.cancel.load(Ordering::Acquire){if matches!(packet,Packet::End(_)){break;}continue;}
            match packet {
                Packet::Frame(mut frame)=>{
                    if index==0{frame.pts=Duration::ZERO;}
                    // Convert channels in the same input allocation. The small
                    // gifski adapter returns it after denoising; no per-frame
                    // raw RGBA clone or sequence image is needed.
                    for pixel in frame.buffer.bytes.chunks_exact_mut(4){pixel.swap(0,2);pixel[3]=255;}
                    let (bytes,recycle)=frame.buffer.into_parts();
                    let pixels=rgba_allocation(bytes);
                    collector.add_frame_rgba_recyclable(index,gifski::collector::ImgVec::new(pixels,region.width as usize,region.height as usize),frame.pts.as_secs_f64(),move|pixels|{
                        let _=recycle.try_send(byte_allocation(pixels));
                    }).map_err(|e|e.to_string())?;
                    index+=1;
                },
                Packet::End(end)=>{collector.set_final_timestamp(end.as_secs_f64()).map_err(|e|e.to_string())?;break;}
            }
        }
        if index==0 && !state.cancel.load(Ordering::Acquire){return Err("没有可保存的录制画面".into());}Ok(())
    })();
    drop(collector);let written=writing.join().map_err(|_|"GIF 写入线程意外退出".to_string())?;
    if state.cancel.load(Ordering::Acquire){return Ok(());}
    result?;written
}

// RGBA8 consists of four u8 values with byte alignment. Transferring the same
// allocation between the pool and gifski is safe only with this exact layout;
// lengths/capacities are converted in bytes, and ownership is moved, not aliased.
const _: () = assert!(std::mem::size_of::<gifski::collector::RGBA8>()==4 && std::mem::align_of::<gifski::collector::RGBA8>()==1);
fn rgba_allocation(bytes:Vec<u8>)->Vec<gifski::collector::RGBA8>{
    assert!(bytes.len().is_multiple_of(4) && bytes.capacity().is_multiple_of(4));
    let mut bytes=std::mem::ManuallyDrop::new(bytes);
    unsafe{Vec::from_raw_parts(bytes.as_mut_ptr().cast(),bytes.len()/4,bytes.capacity()/4)}
}
fn byte_allocation(pixels:Vec<gifski::collector::RGBA8>)->Vec<u8>{
    let mut pixels=std::mem::ManuallyDrop::new(pixels);
    unsafe{Vec::from_raw_parts(pixels.as_mut_ptr().cast(),pixels.len()*4,pixels.capacity()*4)}
}
