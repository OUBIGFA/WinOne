//! Diagnostic comparison against the unmodified upstream constructor.
use windows_capture::encoder::{AudioSettingsBuilder,ContainerSettingsBuilder,VideoEncoder,VideoSettingsBuilder,VideoSettingsSubType};
use windows::Win32::System::WinRT::{RoInitialize,RoUninitialize,RO_INIT_MULTITHREADED};
fn main()->Result<(),Box<dyn std::error::Error>>{
    let path=std::env::args_os().nth(1).ok_or("Pass an isolated output file")?;
    unsafe{RoInitialize(RO_INIT_MULTITHREADED)}?;
    let mut encoder=VideoEncoder::new(VideoSettingsBuilder::new(640,360).frame_rate(30).sub_type(VideoSettingsSubType::H264),AudioSettingsBuilder::default().disabled(true),ContainerSettingsBuilder::default(),path)?;
    let frame=vec![200;640*360*4];
    for time in [0,4_000_000,17_000_000,60_000_000]{encoder.send_frame_buffer(&frame,time)?;}
    encoder.finish()?;unsafe{RoUninitialize()};Ok(())
}
