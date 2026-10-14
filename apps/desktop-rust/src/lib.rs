//! Shared desktop state and the existing Host protocol; neither owns module engines.
pub mod protocol;
pub mod state;
pub mod programs;
#[cfg(windows)]
pub mod transport;
