//! Geometry, clocks, bounded ownership and VFR decisions are independent of
//! Windows. Tests exercise these contracts without capturing the user's screen.
use crossbeam_channel::{Receiver, Sender, bounded};
use serde::{Deserialize, Serialize};
use std::time::Duration;

pub const QUEUE_CAPACITY: usize = 8;
pub const POOL_CAPACITY: usize = QUEUE_CAPACITY + 2; // producer + pending duration
pub const DISK_RESERVE: u64 = 200 * 1024 * 1024;
pub const KEEPALIVE: Duration = Duration::from_secs(1);

#[derive(Clone, Copy, Debug, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Format { Mp4, Gif }
#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Codec { #[default] H264, Hevc }
#[derive(Clone, Copy, Debug, Default, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub enum Coordinates { #[default] PhysicalDesktop, LogicalMonitor }

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Config {
    pub x: f64, pub y: f64, pub width: f64, pub height: f64,
    pub monitor: String,
    pub output: String,
    pub format: Format,
    #[serde(default)] pub codec: Codec,
    #[serde(default)] pub coordinates: Coordinates,
    #[serde(default = "scale")] pub scale_factor: f64,
    #[serde(default)] pub fps: Option<u32>,
    #[serde(default = "quality")] pub quality: u8,
    #[serde(default = "gif_seconds")] pub gif_max_seconds: u32,
    #[serde(default = "gif_frames")] pub gif_max_frames: u64,
    #[serde(default)] pub exclude_windows: Vec<u64>,
    #[serde(default = "yes")] pub cursor: bool,
}
fn scale() -> f64 { 1.0 }
fn quality() -> u8 { 85 }
fn gif_seconds() -> u32 { 120 }
fn gif_frames() -> u64 { 1800 }
fn yes() -> bool { true }
impl Config {
    pub fn frame_rate(&self) -> u32 { self.fps.unwrap_or(if self.format == Format::Mp4 { 30 } else { 15 }) }
    pub fn validate(&self) -> Result<(), String> {
        if self.monitor.is_empty() || self.output.is_empty() || self.output.contains('\0') { return Err("显示器或输出位置无效".into()); }
        if !(1..=60).contains(&self.frame_rate()) || self.format == Format::Gif && self.frame_rate() > 30 { return Err("视频帧率应为 1–60，GIF 帧率应为 1–30".into()); }
        if !(1..=100).contains(&self.quality) || !(1..=600).contains(&self.gif_max_seconds) || !(2..=18_000).contains(&self.gif_max_frames) { return Err("GIF 质量或录制保护设置无效".into()); }
        if !self.scale_factor.is_finite() || !(0.5..=8.).contains(&self.scale_factor) { return Err("显示缩放比例无效，请重新框选".into()); }
        if self.exclude_windows.len() > 32 { return Err("需排除的窗口过多".into()); }
        let extension = std::path::Path::new(&self.output).extension().and_then(|s| s.to_str()).unwrap_or("");
        if !extension.eq_ignore_ascii_case(if self.format == Format::Mp4 { "mp4" } else { "gif" }) { return Err("文件扩展名与录制格式不一致".into()); }
        Ok(())
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize)]
pub struct Region { pub x: u32, pub y: u32, pub width: u32, pub height: u32 }
impl Region {
    /// Logical coordinates are explicitly monitor-relative. Native capture
    /// already supplies physical desktop pixels, so those must NOT be scaled twice.
    pub fn resolve(config: &Config, origin: (i32, i32), monitor: (u32, u32)) -> Result<Self, String> {
        config.validate()?;
        let values = [config.x, config.y, config.width, config.height];
        if values.iter().any(|v| !v.is_finite() || v.abs() > i32::MAX as f64) { return Err("录制选区坐标无效".into()); }
        let (x, y, w, h) = match config.coordinates {
            Coordinates::PhysicalDesktop => (config.x - origin.0 as f64, config.y - origin.1 as f64, config.width, config.height),
            Coordinates::LogicalMonitor => (config.x * config.scale_factor, config.y * config.scale_factor, config.width * config.scale_factor, config.height * config.scale_factor),
        };
        if x < 0. || y < 0. || w < 16. || h < 16. || x + w > monitor.0 as f64 || y + h > monitor.1 as f64 { return Err("选区必须位于同一显示器内，且不小于 16×16 像素；请重新框选".into()); }
        let region = Self { x: x.floor() as u32, y: y.floor() as u32, width: (w.floor() as u32) & !1, height: (h.floor() as u32) & !1 };
        if region.width < 16 || region.height < 16 { return Err("对齐后的录制选区小于 16×16 像素".into()); }
        Ok(region)
    }
    pub fn bytes(self) -> Result<usize, String> { (self.width as usize).checked_mul(self.height as usize).and_then(|n| n.checked_mul(4)).ok_or_else(|| "录制选区过大".into()) }
}

/// All time comes from Instant::elapsed in production. No frame-count clock;
/// this value-based core makes multi-hour pause compensation testable instantly.
#[derive(Debug, Default)]
pub struct Timeline { paused_at: Option<Duration>, paused_total: Duration, stopped: Option<Duration> }
impl Timeline {
    pub fn pts(&self, elapsed: Duration) -> Duration { self.stopped.or(self.paused_at).unwrap_or(elapsed).saturating_sub(self.paused_total) }
    pub fn pause(&mut self, elapsed: Duration) { if self.paused_at.is_none() && self.stopped.is_none() { self.paused_at = Some(elapsed); } }
    pub fn resume(&mut self, elapsed: Duration) { if self.stopped.is_none() { if let Some(start) = self.paused_at.take() { self.paused_total += elapsed.saturating_sub(start); } } }
    pub fn stop(&mut self, elapsed: Duration) -> Duration { if self.stopped.is_none() { self.stopped = Some(self.paused_at.unwrap_or(elapsed)); } self.pts(elapsed) }
    pub fn paused(&self) -> bool { self.paused_at.is_some() }
    pub fn stopped(&self) -> bool { self.stopped.is_some() }
}

/// A bounded sampling grid reads small horizontal blocks, including both edges.
/// Unlike a cryptographic full-frame hash, its cost does not grow with 4K area.
/// Approximate dedup is deliberately paired with a one-second keepalive.
pub fn fingerprint(bytes: &[u8], width: usize, height: usize, stride: usize) -> u64 {
    let mut hash = 0xcbf29ce484222325u64;
    let rows = height.min(64); let cols = width.div_ceil(4).min(128);
    for row in 0..rows {
        let y = row * (height - 1) / (rows - 1).max(1);
        for col in 0..cols {
            let x = col * width.saturating_sub(4) / (cols - 1).max(1);
            let start = y * stride + x * 4;
            for chunk in bytes[start..start + width.min(4) * 4].chunks_exact(4) {
                hash ^= u32::from_le_bytes(chunk.try_into().unwrap()) as u64;
                hash = hash.wrapping_mul(0x100000001b3);
            }
        }
    }
    hash
}
#[derive(Default)]
pub struct VfrFilter { last: Option<(u64, Duration)> }
impl VfrFilter {
    pub fn is_static(&self, hash: u64, pts: Duration) -> bool { self.last.is_some_and(|(previous, time)| previous == hash && pts.saturating_sub(time) < KEEPALIVE) }
    /// Commit only after successful enqueue. A full queue must not cause a
    /// changed frame to be mistaken for one already encoded.
    pub fn accepted(&mut self, hash: u64, pts: Duration) { self.last = Some((hash, pts)); }
}

pub struct BufferPool { returned: Receiver<Vec<u8>>, recycle: Sender<Vec<u8>>, pub bytes: usize }
impl BufferPool {
    pub fn new(frame_bytes: usize) -> Result<Self, String> {
        let (recycle, returned) = bounded(POOL_CAPACITY);
        for _ in 0..POOL_CAPACITY {
            let mut buffer = Vec::new();
            buffer.try_reserve_exact(frame_bytes).map_err(|_| "内存不足，无法准备录制缓冲区".to_string())?;
            buffer.resize(frame_bytes, 0);
            recycle.send(buffer).map_err(|e| e.to_string())?;
        }
        Ok(Self { returned, recycle, bytes: frame_bytes.checked_mul(POOL_CAPACITY).ok_or("缓冲区大小溢出")? })
    }
    pub fn take(&self) -> Option<FrameBuffer> { self.returned.try_recv().ok().map(|bytes| FrameBuffer { bytes, recycle: self.recycle.clone() }) }
}
pub struct FrameBuffer { pub bytes: Vec<u8>, recycle: Sender<Vec<u8>> }
impl FrameBuffer {
    pub(crate) fn into_parts(mut self) -> (Vec<u8>, Sender<Vec<u8>>) { (std::mem::take(&mut self.bytes), self.recycle.clone()) }
}
impl Drop for FrameBuffer {
    fn drop(&mut self) { if !self.bytes.is_empty() { let _ = self.recycle.try_send(std::mem::take(&mut self.bytes)); } }
}
pub struct Frame { pub buffer: FrameBuffer, pub pts: Duration }
pub enum Packet { Frame(Frame), End(Duration) }

#[cfg(test)]
mod tests {
    use super::*;
    fn config() -> Config { serde_json::from_str(r#"{"x":-1900,"y":10,"width":101,"height":99,"monitor":"display","output":"capture.mp4","format":"mp4","scaleFactor":1.5}"#).unwrap() }
    #[test] fn physical_pixels_are_not_scaled_twice() { assert_eq!(Region::resolve(&config(), (-1920, 0), (1920, 1080)).unwrap(), Region{x:20,y:10,width:100,height:98}); }
    #[test] fn monitor_relative_logical_coordinates_scale_once() { let mut c=config();c.coordinates=Coordinates::LogicalMonitor;c.x=10.;assert_eq!(Region::resolve(&c,(-1920,0),(1920,1080)).unwrap(),Region{x:15,y:15,width:150,height:148}); }
    #[test] fn bad_geometry_is_rejected_not_clamped() { let mut c=config();for v in [0.,15.,f64::NAN,5000.] { c.width=v;assert!(Region::resolve(&c,(-1920,0),(1920,1080)).is_err()); } }
    #[test] fn two_hours_and_repeated_pause_have_no_frame_drift() { let mut c=Timeline::default();c.pause(Duration::from_secs(10));c.pause(Duration::from_secs(11));c.resume(Duration::from_secs(35));c.resume(Duration::from_secs(36));assert_eq!(c.pts(Duration::from_secs(7235)),Duration::from_secs(7210));c.pause(Duration::from_secs(7235));assert_eq!(c.stop(Duration::from_secs(7240)),Duration::from_secs(7210));assert_eq!(c.pts(Duration::from_secs(9000)),Duration::from_secs(7210)); }
    #[test] fn static_keepalive_and_failed_enqueue_do_not_lose_changes() { let mut f=VfrFilter::default();f.accepted(1,Duration::ZERO);assert!(f.is_static(1,Duration::from_millis(999)));assert!(!f.is_static(1,KEEPALIVE));assert!(!f.is_static(2,Duration::from_millis(2)));assert!(!f.is_static(2,Duration::from_millis(3))); }
    #[test] fn pool_is_bounded_and_reuses_the_same_allocations() { let p=BufferPool::new(1024).unwrap();let frames=(0..POOL_CAPACITY).map(|_|p.take().unwrap()).collect::<Vec<_>>();let mut addresses=frames.iter().map(|f|f.buffer_address()).collect::<Vec<_>>();assert!(p.take().is_none());drop(frames);for _ in 0..10_000 { let b=p.take().unwrap();assert!(addresses.contains(&b.buffer_address())); } addresses.sort();addresses.dedup();assert_eq!(addresses.len(),POOL_CAPACITY); }
    impl FrameBuffer { fn buffer_address(&self)->usize {self.bytes.as_ptr() as usize} }
    #[test] fn padded_rows_hash_like_packed_rows() { let a=vec![127u8;16*16*4];let mut b=vec![0u8;16*80];for y in 0..16 {b[y*80..y*80+64].copy_from_slice(&a[y*64..(y+1)*64]);}assert_eq!(fingerprint(&a,16,16,64),fingerprint(&b,16,16,80));b[8*80+8*4]=3;assert_ne!(fingerprint(&a,16,16,64),fingerprint(&b,16,16,80)); }
}
