//! Program icons are read from file resources in the UI process; a program is
//! never started for its icon. Extraction runs on the worker thread and yields
//! straight-alpha pixels; display resources are created on the UI thread.
use serde_json::Value;
use std::path::{Path, PathBuf};
use windows_sys::core::PCWSTR;
use windows_sys::Win32::{
    Graphics::Gdi::*,
    Storage::FileSystem::{MoveFileExW, MOVEFILE_REPLACE_EXISTING},
    System::Com::*,
    UI::{Shell::*, WindowsAndMessaging::*},
};

const SIZE: usize = 32;
const PIXELS: usize = SIZE * SIZE * 4;
const RPC_E_CHANGED_MODE: i32 = 0x80010106u32 as i32;

const BASE64: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

pub fn base64_encode(data: &[u8]) -> String {
    let mut out = String::with_capacity(data.len().div_ceil(3) * 4);
    for chunk in data.chunks(3) {
        let bytes = [chunk[0], *chunk.get(1).unwrap_or(&0), *chunk.get(2).unwrap_or(&0)];
        let group = (bytes[0] as u32) << 16 | (bytes[1] as u32) << 8 | bytes[2] as u32;
        out.push(BASE64[(group >> 18) as usize & 63] as char);
        out.push(BASE64[(group >> 12) as usize & 63] as char);
        out.push(if chunk.len() > 1 { BASE64[(group >> 6) as usize & 63] as char } else { '=' });
        out.push(if chunk.len() > 2 { BASE64[group as usize & 63] as char } else { '=' });
    }
    out
}

pub fn base64_decode(text: &str) -> Option<Vec<u8>> {
    let bytes = text.as_bytes();
    if bytes.is_empty() || bytes.len() % 4 != 0 { return None; }
    let chunks = bytes.len() / 4;
    let mut out = Vec::with_capacity(chunks * 3);
    for (index, chunk) in bytes.chunks(4).enumerate() {
        let last = index + 1 == chunks;
        let mut values = [0u8; 4];
        let mut padding = 0;
        for (position, byte) in chunk.iter().enumerate() {
            if *byte == b'=' {
                if !last || position < 2 { return None; }
                padding += 1;
                continue;
            }
            if padding > 0 { return None; }
            values[position] = BASE64.iter().position(|candidate| candidate == byte)? as u8;
        }
        let group = (values[0] as u32) << 18 | (values[1] as u32) << 12 | (values[2] as u32) << 6 | values[3] as u32;
        out.push((group >> 16) as u8);
        if padding < 2 { out.push((group >> 8) as u8); }
        if padding < 1 { out.push(group as u8); }
    }
    Some(out)
}

const K: [u32; 64] = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
];

fn sha256(data: &[u8]) -> [u8; 32] {
    let mut state: [u32; 8] = [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19];
    let mut message = data.to_vec();
    let length = (data.len() as u64) * 8;
    message.push(0x80);
    while message.len() % 64 != 56 { message.push(0); }
    message.extend_from_slice(&length.to_be_bytes());
    for block in message.chunks(64) {
        let mut w = [0u32; 64];
        for index in 0..16 {
            w[index] = u32::from_be_bytes([block[index * 4], block[index * 4 + 1], block[index * 4 + 2], block[index * 4 + 3]]);
        }
        for index in 16..64 {
            let s0 = w[index - 15].rotate_right(7) ^ w[index - 15].rotate_right(18) ^ (w[index - 15] >> 3);
            let s1 = w[index - 2].rotate_right(17) ^ w[index - 2].rotate_right(19) ^ (w[index - 2] >> 10);
            w[index] = w[index - 16].wrapping_add(s0).wrapping_add(w[index - 7]).wrapping_add(s1);
        }
        let mut v = state;
        for index in 0..64 {
            let s1 = v[4].rotate_right(6) ^ v[4].rotate_right(11) ^ v[4].rotate_right(25);
            let choose = (v[4] & v[5]) ^ (!v[4] & v[6]);
            let t1 = v[7].wrapping_add(s1).wrapping_add(choose).wrapping_add(K[index]).wrapping_add(w[index]);
            let s0 = v[0].rotate_right(2) ^ v[0].rotate_right(13) ^ v[0].rotate_right(22);
            let majority = (v[0] & v[1]) ^ (v[0] & v[2]) ^ (v[1] & v[2]);
            let t2 = s0.wrapping_add(majority);
            v = [t1.wrapping_add(t2), v[0], v[1], v[2], v[3].wrapping_add(t1), v[4], v[5], v[6]];
        }
        for index in 0..8 { state[index] = state[index].wrapping_add(v[index]); }
    }
    let mut digest = [0u8; 32];
    for index in 0..8 { digest[index * 4..index * 4 + 4].copy_from_slice(&state[index].to_be_bytes()); }
    digest
}

fn hex(bytes: &[u8]) -> String { bytes.iter().map(|byte| format!("{byte:02x}")).collect() }

/// Shared digest for local integrity checks (profile preview transfer).
pub fn sha256_hex(data: &[u8]) -> String { hex(&sha256(data)) }

/// Read the cached icon or extract it from the file. `None` settles the caller
/// state without writing a cache entry, matching the previous desktop host.
pub fn load(cache_directory: &str, path: &str) -> Option<String> {
    let absolute = std::path::absolute(path).ok()?;
    let metadata = std::fs::metadata(&absolute).ok()?;
    if metadata.is_dir() { return None; }
    let file = PathBuf::from(cache_directory).join(format!("{}.json", cache_key(&absolute)));
    if let Some(pixels) = read_entry(&file, &metadata) { return Some(base64_encode(&pixels)); }
    let pixels = extract(&absolute)?;
    write_entry(&file, &metadata, &pixels);
    Some(base64_encode(&pixels))
}

fn cache_key(absolute: &Path) -> String {
    let normalized = absolute.to_string_lossy().replace('/', "\\").to_lowercase();
    hex(&sha256(normalized.as_bytes()))
}

fn modification_time(metadata: &std::fs::Metadata) -> Option<u64> {
    metadata.modified().ok()?.duration_since(std::time::UNIX_EPOCH).ok().map(|span| span.as_nanos() as u64)
}

fn read_entry(file: &Path, metadata: &std::fs::Metadata) -> Option<Vec<u8>> {
    let data = std::fs::read(file).ok()?;
    if data.len() > 1 << 20 { return None; }
    let entry: Value = serde_json::from_slice(&data).ok()?;
    if entry["version"].as_u64() != Some(1) { return None; }
    if entry["size"].as_u64() != Some(metadata.len()) { return None; }
    if entry["modified"].as_u64() != modification_time(metadata) { return None; }
    let pixels = base64_decode(entry["icon"].as_str()?)?;
    (pixels.len() == PIXELS).then_some(pixels)
}

fn write_entry(file: &Path, metadata: &std::fs::Metadata, pixels: &[u8]) {
    // Cache writes are best effort; a lost entry only costs one re-extraction.
    let Some(modified) = modification_time(metadata) else { return };
    let entry = serde_json::json!({"version":1, "size":metadata.len(), "modified":modified, "icon":base64_encode(pixels)});
    let Some(directory) = file.parent() else { return };
    if std::fs::create_dir_all(directory).is_err() { return; }
    let name = file.file_name().unwrap_or_default().to_string_lossy().to_string();
    let temporary = file.with_file_name(format!("{name}.tmp{}", std::process::id()));
    if std::fs::write(&temporary, entry.to_string().as_bytes()).is_err() { return; }
    if replace_file(&temporary, file).is_err() { let _ = std::fs::remove_file(&temporary); }
}

fn replace_file(from: &Path, to: &Path) -> Result<(), ()> {
    let source = crate::windows_files::wide(&from.to_string_lossy());
    let target = crate::windows_files::wide(&to.to_string_lossy());
    unsafe {
        if MoveFileExW(source.as_ptr() as PCWSTR, target.as_ptr() as PCWSTR, MOVEFILE_REPLACE_EXISTING) != 0 { Ok(()) } else { Err(()) }
    }
}

fn extract(path: &Path) -> Option<Vec<u8>> {
    unsafe {
        // SHGetFileInfoW follows the shell's icon choice for the file; COM is
        // initialized per call exactly like the previous WebView2 host did.
        let com = CoInitializeEx(std::ptr::null(), COINIT_APARTMENTTHREADED as u32);
        if com < 0 && com != RPC_E_CHANGED_MODE { return None; }
        let pixels = file_icon(path);
        if com >= 0 { CoUninitialize(); }
        pixels
    }
}

fn file_icon(path: &Path) -> Option<Vec<u8>> {
    unsafe {
        let wide = crate::windows_files::wide(&path.to_string_lossy());
        let mut info: SHFILEINFOW = std::mem::zeroed();
        let found = SHGetFileInfoW(wide.as_ptr(), 0, &mut info, std::mem::size_of::<SHFILEINFOW>() as u32, SHGFI_ICON);
        if found == 0 || info.hIcon.is_null() { return None; }
        let pixels = recover_pixels(info.hIcon);
        DestroyIcon(info.hIcon);
        pixels
    }
}

fn dib_header() -> BITMAPINFO {
    let mut info: BITMAPINFO = unsafe { std::mem::zeroed() };
    info.bmiHeader.biSize = std::mem::size_of::<BITMAPINFOHEADER>() as u32;
    info.bmiHeader.biWidth = SIZE as i32;
    info.bmiHeader.biHeight = -(SIZE as i32);
    info.bmiHeader.biPlanes = 1;
    info.bmiHeader.biBitCount = 32;
    info
}

fn render_once(icon: HICON, background: u8) -> Option<Vec<u8>> {
    unsafe {
        let dc = CreateCompatibleDC(std::ptr::null_mut());
        if dc.is_null() { return None; }
        let info = dib_header();
        let mut bits: *mut core::ffi::c_void = std::ptr::null_mut();
        let bitmap = CreateDIBSection(dc, &info, DIB_RGB_COLORS, &mut bits, std::ptr::null_mut(), 0);
        if bitmap.is_null() || bits.is_null() { DeleteDC(dc); return None; }
        let previous = SelectObject(dc, bitmap);
        let data = std::slice::from_raw_parts_mut(bits as *mut u8, PIXELS);
        data.fill(background);
        let drawn = DrawIconEx(dc, 0, 0, icon, SIZE as i32, SIZE as i32, 0, std::ptr::null_mut(), DI_NORMAL);
        GdiFlush();
        let copy = if drawn != 0 { Some(data.to_vec()) } else { None };
        SelectObject(dc, previous);
        DeleteObject(bitmap);
        DeleteDC(dc);
        copy
    }
}

/// Render on black and white to recover transparency for both modern alpha
/// icons and legacy masked icons, as the previous desktop host did.
fn recover_pixels(icon: HICON) -> Option<Vec<u8>> {
    let black = render_once(icon, 0)?;
    let white = render_once(icon, 255)?;
    let mut pixels = vec![0u8; PIXELS];
    for index in (0..PIXELS).step_by(4) {
        let difference = (white[index] as i32 + white[index + 1] as i32 + white[index + 2] as i32
            - black[index] as i32 - black[index + 1] as i32 - black[index + 2] as i32) / 3;
        let alpha = (255 - difference).clamp(0, 255) as u8;
        pixels[index + 3] = alpha;
        if alpha == 0 { continue; }
        for channel in 0..3 {
            pixels[index + channel] = ((black[index + channel] as u32 * 255) / alpha as u32).min(255) as u8;
        }
    }
    Some(pixels)
}

/// Straight-alpha BGRA becomes a display icon on the UI thread. The caller owns
/// the returned icon and must release it with `destroy`.
pub fn create_icon(pixels: &[u8]) -> Option<HICON> {
    if pixels.len() != PIXELS { return None; }
    unsafe {
        let info = dib_header();
        let mut bits: *mut core::ffi::c_void = std::ptr::null_mut();
        let color = CreateDIBSection(std::ptr::null_mut(), &info, DIB_RGB_COLORS, &mut bits, std::ptr::null_mut(), 0);
        if color.is_null() || bits.is_null() { return None; }
        let data = std::slice::from_raw_parts_mut(bits as *mut u8, PIXELS);
        for index in (0..PIXELS).step_by(4) {
            let alpha = pixels[index + 3];
            for channel in 0..3 {
                data[index + channel] = ((pixels[index + channel] as u32 * alpha as u32) / 255).min(255) as u8;
            }
            data[index + 3] = alpha;
        }
        let mask = CreateBitmap(SIZE as i32, SIZE as i32, 1, 1, [0u8; 128].as_ptr() as *const core::ffi::c_void);
        let mut icon: HICON = std::ptr::null_mut();
        if !mask.is_null() {
            let details = ICONINFO { fIcon: 1, xHotspot: 0, yHotspot: 0, hbmMask: mask, hbmColor: color };
            icon = CreateIconIndirect(&details);
        }
        DeleteObject(color);
        if !mask.is_null() { DeleteObject(mask); }
        (!icon.is_null()).then_some(icon)
    }
}

pub fn destroy(icon: HICON) { unsafe { DestroyIcon(icon); } }

#[cfg(test)]
mod tests {
    use super::*;

    /// Concurrent SHGetFileInfoW calls race in the shell icon cache: one caller
    /// receives success with a null hIcon. The product extracts on a single
    /// worker thread; tests that extract must be serialized.
    fn extraction_lock() -> std::sync::MutexGuard<'static, ()> {
        static LOCK: std::sync::OnceLock<std::sync::Mutex<()>> = std::sync::OnceLock::new();
        LOCK.get_or_init(|| std::sync::Mutex::new(())).lock().unwrap_or_else(|poisoned| poisoned.into_inner())
    }

    #[test]
    fn sha256_matches_known_vectors() {
        assert_eq!(hex(&sha256(b"")), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
        assert_eq!(hex(&sha256(b"abc")), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
    }

    #[test]
    fn base64_round_trips_known_values() {
        assert_eq!(base64_encode(b"ABC"), "QUJD");
        assert_eq!(base64_encode(b"A"), "QQ==");
        assert_eq!(base64_encode(b"AB"), "QUI=");
        assert_eq!(base64_decode("QUJD").as_deref(), Some(b"ABC".as_slice()));
        assert_eq!(base64_decode("QQ==").as_deref(), Some(&b"A"[..]));
        let pixels = vec![7u8; PIXELS];
        assert_eq!(base64_decode(&base64_encode(&pixels)), Some(pixels));
        assert!(base64_decode("A===").is_none(), "padding must be trailing only");
        assert!(base64_decode("AB!C").is_none(), "invalid characters must be rejected");
    }

    #[test]
    fn extraction_reads_a_real_executable_without_starting_it() {
        let _guard = extraction_lock();
        let pixels = extract(&std::env::current_exe().unwrap());
        assert!(pixels.as_deref().is_some_and(|p| p.len() == PIXELS));
    }

    #[test]
    fn directories_and_missing_files_settle_without_icons() {
        let root = std::env::temp_dir().join(format!("winone-icon-miss-{}", std::process::id()));
        let directory = root.to_string_lossy().to_string();
        assert!(load(&directory, std::env::temp_dir().to_str().unwrap()).is_none());
        assert!(load(&directory, r"Z:\winone-missing\program.exe").is_none());
        assert!(!root.exists(), "icon-less results must not be cached");
    }

    #[test]
    fn cache_hits_and_invalidates_by_file_state() {
        let _guard = extraction_lock();
        let root = std::env::temp_dir().join(format!("winone-icon-cache-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        let target = std::env::current_exe().unwrap();
        let path = target.to_string_lossy().to_string();
        let directory = root.to_string_lossy().to_string();
        assert!(load(&directory, &path).is_some(), "a real executable must yield an icon");
        // Overwrite the entry with a distinctive marker while keeping the correct
        // size and modification time: the next load must be served from cache.
        let file = root.join(format!("{}.json", cache_key(&target)));
        let entry: Value = serde_json::from_slice(&std::fs::read(&file).unwrap()).unwrap();
        let marker = base64_encode(&vec![9u8; PIXELS]);
        let rewritten = serde_json::json!({"version":1, "size":entry["size"], "modified":entry["modified"], "icon":marker});
        std::fs::write(&file, rewritten.to_string()).unwrap();
        let cached = load(&directory, &path).unwrap();
        assert_eq!(base64_decode(&cached), Some(vec![9u8; PIXELS]), "an intact entry must be served from the cache");
        // A size change invalidates the entry and re-extracts the real icon.
        let stale = serde_json::json!({"version":1, "size":entry["size"].as_u64().unwrap() + 1, "modified":entry["modified"], "icon":marker});
        std::fs::write(&file, stale.to_string()).unwrap();
        let refreshed = load(&directory, &path).unwrap();
        assert_ne!(base64_decode(&refreshed), Some(vec![9u8; PIXELS]), "changed files must re-extract");
        let _ = std::fs::remove_dir_all(&root);
    }

    #[test]
    fn create_icon_builds_and_releases_a_valid_display_icon() {
        let mut pixels = vec![0u8; PIXELS];
        for index in (0..PIXELS).step_by(4) { pixels[index + 2] = 255; pixels[index + 3] = 255; }
        let icon = create_icon(&pixels).expect("icon creation must succeed");
        assert!(!icon.is_null());
        assert!(create_icon(&pixels[..4095]).is_none(), "partial pixel buffers must be rejected");
        destroy(icon);
    }
}
