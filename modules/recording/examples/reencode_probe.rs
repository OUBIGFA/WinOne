//! Test-only BGRA stream → the production Media Foundation encoder. The caller
//! decodes an explicitly selected sample to stdin; there is no product FFmpeg
//! dependency, desktop capture, frame sequence or extra full-video allocation.
use std::{fs::OpenOptions, io::{self, Read}, path::PathBuf, thread, time::Duration};
use winone_recording::{core::{BufferPool, Config, Packet, Frame, Region, QUEUE_CAPACITY}, encoder, session::Shared};

#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct SampleTimes { pts_seconds: Vec<f64>, duration_seconds: f64 }

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = std::env::args().skip(1).collect::<Vec<_>>();
    if !(4..=5).contains(&args.len()) { return Err("Pass output.mp4 width height fps [timestamps.json]; stream BGRA frames on stdin".into()); }
    let times = args.get(4).map(|file| -> Result<SampleTimes, Box<dyn std::error::Error>> {
        let times: SampleTimes = serde_json::from_slice(&std::fs::read(file)?)?;
        if times.pts_seconds.is_empty() || times.pts_seconds.iter().any(|time| !time.is_finite() || *time < 0.)
            || times.pts_seconds.windows(2).any(|pair| pair[1] <= pair[0])
            || !times.duration_seconds.is_finite() || times.duration_seconds <= *times.pts_seconds.last().unwrap() {
            return Err("Invalid source sample timeline".into());
        }
        Ok(times)
    }).transpose()?;
    let output = PathBuf::from(&args[0]);
    let width: u32 = args[1].parse()?; let height: u32 = args[2].parse()?; let fps: u32 = args[3].parse()?;
    let config: Config = serde_json::from_value(serde_json::json!({"x":0,"y":0,"width":width,"height":height,"monitor":"sample-probe","output":output,"format":"mp4","fps":fps}))?;
    let region = Region::resolve(&config, (0, 0), (width, height))?;
    if region.width != width || region.height != height { return Err("The sample probe requires even dimensions; it must not crop the source".into()); }
    let pool = BufferPool::new(region.bytes()?)?;
    OpenOptions::new().write(true).create_new(true).open(&output)?;
    let state = Shared::new(&config, region, pool.bytes); let evidence = state.clone();
    let (send, receive) = crossbeam_channel::bounded(QUEUE_CAPACITY);
    let (ready_send, ready_receive) = crossbeam_channel::bounded(1);
    let target = output.clone();
    let worker = thread::spawn(move || encoder::encode(&config, region, &target, receive, state, ready_send));
    ready_receive.recv_timeout(Duration::from_secs(20))??;
    let mut input = io::stdin().lock(); let mut count = 0_u64;
    loop {
        let mut buffer = loop {
            if let Some(buffer) = pool.take() { break buffer; }
            if worker.is_finished() { return Err("Sample encoder exited while waiting for its pooled input".into()); }
            thread::sleep(Duration::from_millis(1));
        };
        if input.read(&mut buffer.bytes[..1])? == 0 { break; }
        input.read_exact(&mut buffer.bytes[1..])?;
        let pts = match &times {
            Some(times) => *times.pts_seconds.get(count as usize).ok_or("Decoder emitted more frames than the sample timeline")?,
            None => count as f64 / fps as f64,
        };
        send.send(Packet::Frame(Frame { buffer, pts: Duration::from_secs_f64(pts) }))?;
        count += 1;
    }
    if times.as_ref().is_some_and(|times| times.pts_seconds.len() != count as usize) { return Err("Decoder lost frames from the source sample".into()); }
    let duration = times.as_ref().map_or(count as f64 / fps as f64, |times| times.duration_seconds);
    send.send(Packet::End(Duration::from_secs_f64(duration)))?; drop(send);
    worker.join().map_err(|_| "Sample encoder panicked")??;
    println!("{}", serde_json::json!({"file":output,"width":width,"height":height,"fps":fps,"frames":count,"bytes":std::fs::metadata(&output)?.len(),"poolBytes":evidence.snapshot().pool_bytes}));
    Ok(())
}
