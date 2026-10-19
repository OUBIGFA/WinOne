//! For adding frames to the encoder
//!
//! [`gifski::new()`][crate::new] returns the [`Collector`] that collects animation frames,
//! and a [`Writer`][crate::Writer] that performs compression and I/O.

pub use imgref::ImgVec;
pub use rgb::{RGB8, RGBA8};

use crate::error::GifResult;
use crossbeam_channel::Sender;
use std::sync::{Arc, atomic::{AtomicU64, Ordering}};

// WinOne: optional ownership return after the denoiser no longer needs the
// original input. Existing public collection methods retain their behavior.
pub(crate) type Recycle = Box<dyn FnOnce(Vec<RGBA8>) + Send>;

#[cfg(feature = "png")]
use std::path::PathBuf;

pub(crate) enum FrameSource {
    Pixels(ImgVec<RGBA8>),
    #[cfg(feature = "png")]
    PngData(Vec<u8>),
    #[cfg(all(feature = "png", not(target_arch = "wasm32")))]
    Path(PathBuf),
}

pub(crate) struct InputFrame {
    /// The pixels to resize and encode
    pub frame: FrameSource,
    /// Time in seconds when to display the frame. First frame should start at 0.
    pub presentation_timestamp: f64,
    pub frame_index: usize,
    pub recycle: Option<Recycle>,
}

pub(crate) struct InputFrameResized {
    /// The pixels to encode
    pub frame: ImgVec<RGBA8>,
    /// The same as above, but with smart blur applied (for denoiser)
    pub frame_blurred: ImgVec<RGB8>,
    /// Time in seconds when to display the frame. First frame should start at 0.
    pub presentation_timestamp: f64,

    /// Debugging bad inputs
    pub original_index: usize,
    pub recycle: Option<Recycle>,
}

/// Collect frames that will be encoded
///
/// Note that writing will finish only when the collector is dropped.
/// Collect frames on another thread, or call `drop(collector)` before calling `writer.write()`!
pub struct Collector {
    pub(crate) queue: Sender<InputFrame>,
    pub(crate) final_timestamp: Arc<AtomicU64>,
}

impl Collector {
    /// WinOne: fixed-size streaming input with ownership returned after use.
    /// The callback is for buffer reuse, not a completion notification. If the
    /// pipeline fails, input allocations are dropped normally instead.
    pub fn add_frame_rgba_recyclable(&self, frame_index: usize, frame: ImgVec<RGBA8>, presentation_timestamp: f64, recycle: impl FnOnce(Vec<RGBA8>) + Send + 'static) -> GifResult<()> {
        self.queue.send(InputFrame {frame: FrameSource::Pixels(frame), frame_index, presentation_timestamp, recycle: Some(Box::new(recycle))})?;
        Ok(())
    }

    /// WinOne: the real end PTS of a live capture, set before dropping the
    /// collector. No terminal duplicate or guessed average frame duration.
    pub fn set_final_timestamp(&self, timestamp: f64) -> GifResult<()> {
        if !timestamp.is_finite() || timestamp < 0. { return Err(crate::Error::WrongSize("invalid final timestamp".into())); }
        self.final_timestamp.store(timestamp.to_bits(), Ordering::Release);
        Ok(())
    }

    /// Frame index starts at 0.
    ///
    /// Set each frame (index) only once, but you can set them in any order. However, out-of-order frames
    /// will be buffered in RAM, and big gaps in frame indices will cause high memory usage.
    ///
    /// Presentation timestamp is time in seconds (since file start at 0) when this frame is to be displayed.
    ///
    /// If the first frame doesn't start at pts=0, the delay will be used for the last frame.
    ///
    /// If this function appears to be stuck after a few frames, it's because [`crate::Writer::write()`] is not running.
    #[cfg_attr(debug_assertions, track_caller)]
    pub fn add_frame_rgba(&self, frame_index: usize, frame: ImgVec<RGBA8>, presentation_timestamp: f64) -> GifResult<()> {
        debug_assert!(frame_index == 0 || presentation_timestamp > 0.);
        self.queue.send(InputFrame {
            frame_index,
            frame: FrameSource::Pixels(frame),
            presentation_timestamp,
            recycle: None,
        })?;
        Ok(())
    }

    /// Decode a frame from in-memory PNG-compressed data.
    ///
    /// Frame index starts at 0.
    /// Set each frame (index) only once, but you can set them in any order. However, out-of-order frames
    /// will be buffered in RAM, and big gaps in frame indices will cause high memory usage.
    ///
    /// Presentation timestamp is time in seconds (since file start at 0) when this frame is to be displayed.
    ///
    /// If the first frame doesn't start at pts=0, the delay will be used for the last frame.
    ///
    /// If this function appears to be stuck after a few frames, it's because [`crate::Writer::write()`] is not running.
    #[cfg(feature = "png")]
    #[inline]
    pub fn add_frame_png_data(&self, frame_index: usize, png_data: Vec<u8>, presentation_timestamp: f64) -> GifResult<()> {
        self.queue.send(InputFrame {
            frame: FrameSource::PngData(png_data),
            presentation_timestamp,
            frame_index,
            recycle: None,
        })?;
        Ok(())
    }

    /// Read and decode a PNG file from disk.
    ///
    /// Frame index starts at 0.
    /// Set each frame (index) only once, but you can set them in any order.
    ///
    /// Presentation timestamp is time in seconds (since file start at 0) when this frame is to be displayed.
    ///
    /// If the first frame doesn't start at pts=0, the delay will be used for the last frame.
    ///
    /// If this function appears to be stuck after a few frames, it's because [`crate::Writer::write()`] is not running.
    #[cfg(feature = "png")]
    pub fn add_frame_png_file(&self, frame_index: usize, path: PathBuf, presentation_timestamp: f64) -> GifResult<()> {
        self.queue.send(InputFrame {
            frame: FrameSource::Path(path),
            presentation_timestamp,
            frame_index,
            recycle: None,
        })?;
        Ok(())
    }
}
