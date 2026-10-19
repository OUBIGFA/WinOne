//! WGC is reused from windows-capture; only the crop-to-pool adapter is ours.
//! Its stock buffer_crop and VideoEncoder allocate per frame / queue without a
//! bound, so this path reuses one staging texture and the fixed ownership pool.
use crate::{core::{BufferPool, Config, Frame as OwnedFrame, Packet, Region, VfrFilter, fingerprint}, session::Shared};
use crossbeam_channel::Sender;
use std::{sync::{Arc, atomic::Ordering}, time::{Duration, Instant}};
use windows::Win32::{Foundation::HWND, Graphics::{Gdi::{GetMonitorInfoW, HMONITOR, MONITORINFO}, Direct3D11::{D3D11_BOX, D3D11_MAP_READ, D3D11_MAPPED_SUBRESOURCE, ID3D11DeviceContext, ID3D11Texture2D}}, UI::WindowsAndMessaging::{GetWindowDisplayAffinity, GetWindowThreadProcessId, SetWindowDisplayAffinity, WDA_EXCLUDEFROMCAPTURE, WINDOW_DISPLAY_AFFINITY}};
use windows_capture::{capture::{CaptureControl, Context, GraphicsCaptureApiHandler}, d3d11::{SendDirectX, StagingTexture}, frame::Frame, graphics_capture_api::InternalCaptureControl, monitor::Monitor, settings::{ColorFormat, CursorCaptureSettings, DirtyRegionSettings, DrawBorderSettings, MinimumUpdateIntervalSettings, SecondaryWindowSettings, Settings}};

pub fn resolve_monitor(config:&Config)->Result<(Monitor,Region),String>{
    config.validate()?;
    for monitor in Monitor::enumerate().map_err(|e|e.to_string())? {
        if monitor.device_name().map_err(|e|e.to_string())?.eq_ignore_ascii_case(&config.monitor) {
            let mut info=MONITORINFO{cbSize:std::mem::size_of::<MONITORINFO>() as u32,..Default::default()};
            if !unsafe{GetMonitorInfoW(HMONITOR(monitor.as_raw_hmonitor()),&mut info)}.as_bool(){return Err("显示器信息读取失败，请重新框选".into());}
            let rect=info.rcMonitor;
            let region=Region::resolve(config,(rect.left,rect.top),((rect.right-rect.left) as u32,(rect.bottom-rect.top) as u32))?;
            return Ok((monitor,region));
        }
    }
    Err("选区所属显示器已断开，请重新框选".into())
}
pub struct Flags {region:Region,frames:Sender<Packet>,pool:BufferPool,shared:Arc<Shared>,config:Config}
pub struct Capture {flags:Flags, staging:SendDirectX<StagingTexture>,filter:VfrFilter,last_arrival:Option<Duration>,_excluded:ExcludedWindows}
pub fn start(monitor:Monitor,region:Region,config:&Config,frames:Sender<Packet>,pool:BufferPool,shared:Arc<Shared>)->Result<CaptureControl<Capture,String>,String>{
    // Frame-rate throttling also lives in the callback so Windows versions
    // without the newer MinimumUpdateInterval API retain identical semantics.
    Capture::start_free_threaded(Settings::new(monitor,
        if config.cursor{CursorCaptureSettings::WithCursor}else{CursorCaptureSettings::WithoutCursor},
        DrawBorderSettings::Default, SecondaryWindowSettings::Default,MinimumUpdateIntervalSettings::Default,DirtyRegionSettings::Default,ColorFormat::Bgra8,
        Flags{region,frames,pool,shared,config:config.clone()})).map_err(|e|e.to_string())
}
impl GraphicsCaptureApiHandler for Capture {
    type Flags=Flags;type Error=String;
    fn new(ctx:Context<Flags>)->Result<Self,String>{
        let region=ctx.flags.region;
        let excluded=ExcludedWindows::new(&ctx.flags.config.exclude_windows)?;
        let staging=StagingTexture::new(&ctx.device,region.width,region.height,windows::Win32::Graphics::Dxgi::Common::DXGI_FORMAT_B8G8R8A8_UNORM).map_err(|e|e.to_string())?;
        Ok(Self{flags:ctx.flags,staging:SendDirectX::new(staging),filter:VfrFilter::default(),last_arrival:None,_excluded:excluded})
    }
    fn on_frame_arrived(&mut self,frame:&mut Frame,control:InternalCaptureControl)->Result<(),String>{
        let state=&self.flags.shared;
        state.captured.fetch_add(1,Ordering::Relaxed);
        let clock=state.clock.lock();
        if clock.stopped(){control.stop();return Ok(());}
        if clock.paused(){return Ok(());}
        let pts=clock.pts(state.start.elapsed());drop(clock);
        let interval=Duration::from_secs_f64(1./self.flags.config.frame_rate() as f64);
        if self.last_arrival.is_some_and(|last|pts.saturating_sub(last)<interval){return Ok(());}
        self.last_arrival=Some(pts);
        let r=self.flags.region;
        if r.x+r.width>frame.width() || r.y+r.height>frame.height(){return Err("显示分辨率已改变，已停止录制以免选区错位".into());}
        let source=D3D11_BOX{left:r.x,top:r.y,front:0,right:r.x+r.width,bottom:r.y+r.height,back:1};
        unsafe{frame.device_context().CopySubresourceRegion(self.staging.0.texture(),0,0,0,0,frame.as_raw_texture(),0,Some(&source));}
        let mapped=Mapped::new(frame.device_context(),self.staging.0.texture(),r.height)?;
        let before=Instant::now();
        let hash=fingerprint(mapped.bytes(),r.width as usize,r.height as usize,mapped.info.RowPitch as usize);
        state.filter_max_ns.fetch_max(before.elapsed().as_nanos().min(u64::MAX as u128) as u64,Ordering::Relaxed);
        if self.filter.is_static(hash,pts){state.skipped.fetch_add(1,Ordering::Relaxed);return Ok(());}
        let Some(mut buffer)=self.flags.pool.take() else{state.dropped.fetch_add(1,Ordering::Relaxed);return Ok(());};
        let row_bytes=r.width as usize*4;
        for (row,destination) in buffer.bytes.chunks_exact_mut(row_bytes).enumerate(){let offset=row*mapped.info.RowPitch as usize;destination.copy_from_slice(&mapped.bytes()[offset..offset+row_bytes]);}
        match self.flags.frames.try_send(Packet::Frame(OwnedFrame{buffer,pts})) {
            Ok(())=>{self.filter.accepted(hash,pts);state.submitted.fetch_add(1,Ordering::Relaxed);state.max_queue.fetch_max(self.flags.frames.len() as u64,Ordering::Relaxed);},
            Err(crossbeam_channel::TrySendError::Full(_))=>{state.dropped.fetch_add(1,Ordering::Relaxed);},
            Err(crossbeam_channel::TrySendError::Disconnected(_))=>control.stop(),
        }
        Ok(())
    }
    fn on_closed(&mut self)->Result<(),String>{Ok(())}
}
struct Mapped<'a>{context:&'a ID3D11DeviceContext,texture:&'a ID3D11Texture2D,info:D3D11_MAPPED_SUBRESOURCE,rows:u32}
impl<'a> Mapped<'a>{
    fn new(context:&'a ID3D11DeviceContext,texture:&'a ID3D11Texture2D,rows:u32)->Result<Self,String>{let mut info=D3D11_MAPPED_SUBRESOURCE::default();unsafe{context.Map(texture,0,D3D11_MAP_READ,0,Some(&mut info))}.map_err(|e|e.to_string())?;Ok(Self{context,texture,info,rows})}
    fn bytes(&self)->&[u8]{unsafe{std::slice::from_raw_parts(self.info.pData.cast(),self.info.RowPitch as usize*self.rows as usize)}}
}
impl Drop for Mapped<'_>{fn drop(&mut self){unsafe{self.context.Unmap(self.texture,0)}}}

struct ExcludedWindows(Vec<(u64,u32,u32)>);
impl ExcludedWindows {
    fn new(handles:&[u64])->Result<Self,String>{
        let mut owned=Self(Vec::new());
        for &handle in handles {
            let hwnd=HWND(handle as *mut _);let mut process=0;let thread=unsafe{GetWindowThreadProcessId(hwnd,Some(&mut process))};
            if process!=std::process::id() || thread==0 {return Err("只能排除当前截图工具自己的窗口".into());}
            let mut previous=0;
            unsafe{GetWindowDisplayAffinity(hwnd,&mut previous)}.map_err(|e|format!("无法读取录制工具栏的捕获保护：{e}"))?;
            unsafe{SetWindowDisplayAffinity(hwnd,WDA_EXCLUDEFROMCAPTURE)}.map_err(|e|format!("无法从录制中排除工具栏，请检查 Windows 捕获支持：{e}"))?;
            owned.0.push((handle,previous,thread));
        }
        // Display affinity is composited asynchronously. Fence those changes
        // before WGC allocates its first capture frame; a stale first surface
        // can otherwise contain the protected window's black placeholder.
        if !handles.is_empty() { unsafe { windows::Win32::Graphics::Dwm::DwmFlush() }.map_err(|e|format!("无法同步录制窗口的捕获保护：{e}"))?; }
        Ok(owned)
    }
}
impl Drop for ExcludedWindows {
    fn drop(&mut self){for &(handle,previous,thread) in self.0.iter().rev(){let hwnd=HWND(handle as *mut _);let mut pid=0;if unsafe{GetWindowThreadProcessId(hwnd,Some(&mut pid))}==thread && pid==std::process::id(){if let Err(error)=unsafe{SetWindowDisplayAffinity(hwnd,WINDOW_DISPLAY_AFFINITY(previous))}{eprintln!("restore recording window affinity: {error}");}}}}
}
