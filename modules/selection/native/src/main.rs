//! One-shot, read-only UIA selection retrieval. The caller owns the timeout and
//! process job; a hung external provider never occupies the Host's hook thread.
#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]
use serde::{Deserialize, Serialize};
use std::io::{Read, Write};
use std::sync::mpsc::{sync_channel, SyncSender};
use std::time::{Duration, Instant};
use windows::Win32::{
    Foundation::{HWND, POINT},
    System::{Com::*, Ole::*, Variant::VT_R8},
    UI::{Accessibility::*, WindowsAndMessaging::*},
};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Request {
    window: u64,
    pid: u32,
    #[serde(default)]
    point: Option<[f64; 2]>,
    #[serde(default)]
    origin: Option<[f64; 2]>,
}
#[derive(Serialize)]
struct Reply {
    status: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    text: Option<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    rectangles: Vec<[f64; 4]>,
}
impl Reply {
    fn status(status: &'static str) -> Self {
        Self { status, text: None, rectangles: vec![] }
    }
}
struct Apartment;
impl Drop for Apartment {
    fn drop(&mut self) { unsafe { CoUninitialize() } }
}
struct Array(*mut SAFEARRAY);
impl Drop for Array {
    fn drop(&mut self) { unsafe { let _ = SafeArrayDestroy(self.0); } }
}

#[windows_core::implement(IUIAutomationStructureChangedEventHandler)]
struct StructureChanged(SyncSender<()>);
impl IUIAutomationStructureChangedEventHandler_Impl for StructureChanged_Impl {
    fn HandleStructureChangedEvent(&self, _: windows_core::Ref<IUIAutomationElement>, _: StructureChangeType, _: *const SAFEARRAY) -> windows_core::Result<()> {
        let _ = self.0.try_send(());
        Ok(())
    }
}
struct Subscription {
    automation: IUIAutomation,
    root: IUIAutomationElement,
    handler: IUIAutomationStructureChangedEventHandler,
}
impl Drop for Subscription {
    fn drop(&mut self) {
        unsafe { let _ = self.automation.RemoveStructureChangedEventHandler(&self.root, &self.handler); }
    }
}

fn source_current(request: &Request) -> bool {
    unsafe {
        let window = HWND(request.window as *mut _);
        let mut pid = 0;
        request.pid != 0 && request.window != 0 && IsWindow(Some(window)).as_bool()
            && GetForegroundWindow() == window
            && GetWindowThreadProcessId(window, Some(&mut pid)) != 0 && pid == request.pid
    }
}

// UIA returns physical screen coordinates. Reject malformed or huge arrays
// before copying them, and release even when a provider returns invalid data.
fn rectangles(range: &IUIAutomationTextRange) -> Result<Vec<[f64; 4]>, &'static str> {
    unsafe {
        let array = Array(range.GetBoundingRectangles().map_err(|_| "unavailable")?);
        if array.0.is_null() { return Ok(vec![]) }
        if SafeArrayGetDim(array.0) != 1 || SafeArrayGetVartype(array.0).map_err(|_| "unavailable")? != VT_R8 {
            return Err("unavailable");
        }
        let first = SafeArrayGetLBound(array.0, 1).map_err(|_| "unavailable")?;
        let last = SafeArrayGetUBound(array.0, 1).map_err(|_| "unavailable")?;
        let count = i64::from(last) - i64::from(first) + 1;
        if !(0..=4096).contains(&count) || count % 4 != 0 { return Err("too_large") }
        let mut data = Vec::with_capacity(count as usize);
        for offset in 0..count as i32 {
            let index = first + offset;
            let mut value = 0f64;
            SafeArrayGetElement(array.0, &index, (&mut value as *mut f64).cast()).map_err(|_| "unavailable")?;
            if !value.is_finite() || value.abs() > 10_000_000.0 { return Err("unavailable") }
            data.push(value);
        }
        Ok(data.chunks_exact(4).filter(|r| r[2] > 0.0 && r[3] > 0.0).map(|r| [r[0], r[1], r[2], r[3]]).collect())
    }
}

// Some WebView text providers expose a selection before its line rectangles.
// Hit-test the visible text control and compare text endpoints in that case.
// RangeFromPoint alone returns the nearest character even outside the control,
// so bounds, visibility and the already-verified foreground ancestry are required.
fn point_in_selection(element: &IUIAutomationElement, pattern: &IUIAutomationTextPattern, ranges: &IUIAutomationTextRangeArray, point: [f64; 2]) -> bool {
    let [x, y] = point;
    if !x.is_finite() || !y.is_finite() { return false }
    unsafe {
        if element.CurrentIsOffscreen().map_or(true, |value| value.as_bool()) { return false }
        let Ok(bounds) = element.CurrentBoundingRectangle() else { return false };
        if x < f64::from(bounds.left) || x >= f64::from(bounds.right) || y < f64::from(bounds.top) || y >= f64::from(bounds.bottom) { return false }
        let Ok(hit) = pattern.RangeFromPoint(POINT { x: x as i32, y: y as i32 }) else { return false };
        let Ok(count) = ranges.Length() else { return false };
        (0..count).any(|index| {
            let Ok(range) = ranges.GetElement(index) else { return false };
            matches!(hit.CompareEndpoints(TextPatternRangeEndpoint_Start, &range, TextPatternRangeEndpoint_Start), Ok(value) if value >= 0)
                && matches!(hit.CompareEndpoints(TextPatternRangeEndpoint_Start, &range, TextPatternRangeEndpoint_End), Ok(value) if value < 0)
        })
    }
}

fn retrieve(request: &Request) -> Result<Reply, &'static str> {
    if !source_current(request) { return Err("stale") }
    unsafe {
        CoInitializeEx(None, COINIT_MULTITHREADED).ok().map_err(|_| "unavailable")?;
        let _apartment = Apartment;
        let automation: IUIAutomation = CoCreateInstance(&CUIAutomation, None, CLSCTX_INPROC_SERVER).map_err(|_| "unavailable")?;
        let root = automation.ElementFromHandle(HWND(request.window as *mut _)).map_err(|_| "stale")?;
        let (sender, changes) = sync_channel(1);
        let handler: IUIAutomationStructureChangedEventHandler = StructureChanged(sender).into();
        automation.AddStructureChangedEventHandler(&root, TreeScope_Subtree, None, &handler).map_err(|_| "unavailable")?;
        let _subscription = Subscription { automation: automation.clone(), root: root.clone(), handler };
        // Chromium creates the accessible document asynchronously on first use.
        // Re-query only after its structural event, never after an arbitrary
        // delay or a full-document fallback. A caret/sensitive/stale result ends
        // immediately. Parent process cancellation also bounds external COM calls.
        let deadline = Instant::now() + Duration::from_millis(600);
        loop {
            let result = read_selection(request, &automation, &root);
            if result.as_ref().err() != Some(&"unsupported") { return result }
            if changes.recv_timeout(deadline.saturating_duration_since(Instant::now())).is_err() { return result }
            if !source_current(request) { return Err("stale") }
        }
    }
}

fn read_selection(request: &Request, automation: &IUIAutomation, root: &IUIAutomationElement) -> Result<Reply, &'static str> {
    unsafe {
        let focus = automation.GetFocusedElement().map_err(|_| "unavailable")?;
        let diagnostics=std::env::var_os("WINONE_SELECTION_DIAGNOSTICS").is_some();
        if diagnostics {eprintln!("focus metadata: pid={:?}, type={:?}, password={:?}",focus.CurrentProcessId(),focus.CurrentControlType(),focus.CurrentIsPassword());}
        // Unknown sensitivity is not permission to read. Never use ValuePattern,
        // DocumentRange or simulated Copy as an automatic fallback.
        if focus.CurrentIsPassword().map_err(|_| "unavailable")?.as_bool() { return Err("sensitive") }
        let walker = automation.RawViewWalker().map_err(|_| "unavailable")?;
        let mut element = focus.clone();
        let mut path = vec![];
        let mut owned = false;
        // Chromium accessibility nodes can belong to a child process. Verify
        // their actual UIA ancestry to the exact foreground window instead of
        // equating process IDs or accepting every child process of a browser.
        for _ in 0..64 {
            if element.CurrentIsPassword().map_err(|_| "unavailable")?.as_bool() { return Err("sensitive") }
            let native = element.CurrentNativeWindowHandle().map_err(|_| "unavailable")?;
            if diagnostics {eprintln!("selection ancestry: pid={:?}, type={:?}, native={:?}, nativeRoot={:?}, source={:?}, uiaRoot={:?}",element.CurrentProcessId(),element.CurrentControlType(),native,GetAncestor(native,GA_ROOT),HWND(request.window as *mut _),automation.CompareElements(&element,root));}
            if !native.is_invalid() && GetAncestor(native, GA_ROOT) != HWND(request.window as *mut _) { return Err("stale") }
            path.push(element.clone());
            if automation.CompareElements(&element, root).map_err(|_| "stale")?.as_bool() { owned = true; break }
            let Ok(parent) = walker.GetParentElement(&element) else { break };
            element = parent;
        }
        if !owned { return Err("stale") }
        for element in path {
            if std::env::var_os("WINONE_SELECTION_DIAGNOSTICS").is_some() {
                // Opt-in diagnostics include control metadata only, never text,
                // element names, field values or clipboard contents.
                eprintln!("selection control: pid={:?}, type={:?}, textPattern={}", element.CurrentProcessId(), element.CurrentControlType(), element.GetCurrentPatternAs::<IUIAutomationTextPattern>(UIA_TextPatternId).is_ok());
            }
            if let Ok(pattern) = element.GetCurrentPatternAs::<IUIAutomationTextPattern>(UIA_TextPatternId) {
                let ranges = pattern.GetSelection().map_err(|_| "unavailable")?;
                let count = ranges.Length().map_err(|_| "unavailable")?;
                if !(0..=32).contains(&count) { return Err("too_large") }
                let mut text = String::new();
                let mut boxes = vec![];
                for index in 0..count {
                    let range = ranges.GetElement(index).map_err(|_| "unavailable")?;
                    let enclosing = range.GetEnclosingElement().map_err(|_| "unavailable")?;
                    if enclosing.CurrentIsPassword().map_err(|_| "unavailable")?.as_bool() { return Err("sensitive") }
                    let part = range.GetText(65537).map_err(|_| "unavailable")?.to_string();
                    if part.trim().is_empty() { continue }
                    if !text.is_empty() { text.push('\n') }
                    text.push_str(&part);
                    if text.len() > 65536 { return Err("too_large") }
                    boxes.extend(rectangles(&range)?);
                }
                // A caret is an empty selection; walking to a document ancestor
                // at this point would risk returning a different selection.
                if text.trim().is_empty() { return Err("empty") }
                if let Some([x, y]) = request.point {
                    if diagnostics { eprintln!("selection geometry: point={:?}, origin={:?}, boxes={:?}", request.point, request.origin, boxes); }
                    // A text drag often ends beyond the last glyph. Either end
                    // must belong to the selected range, not only mouse-up.
                    let near = |[px, py]: [f64; 2]| px.is_finite() && py.is_finite() && boxes.iter().any(|r| px >= r[0]-24.0 && py >= r[1]-24.0 && px <= r[0]+r[2]+24.0 && py <= r[1]+r[3]+24.0);
                    let matches_range = near([x,y]) || request.origin.is_some_and(near)
                        || boxes.is_empty() && (point_in_selection(&element, &pattern, &ranges, [x,y])
                            || request.origin.is_some_and(|point| point_in_selection(&element, &pattern, &ranges, point)));
                    if !matches_range {
                        return Err("unrelated");
                    }
                }
                let current = automation.GetFocusedElement().map_err(|_| "stale")?;
                if !source_current(request) || !automation.CompareElements(&focus, &current).map_err(|_| "stale")?.as_bool() { return Err("stale") }
                return Ok(Reply { status: "selected", text: Some(text), rectangles: boxes });
            }
        }
        Err("unsupported")
    }
}

fn main() {
    let mut data = vec![];
    let input = std::io::stdin().take(4097).read_to_end(&mut data);
    let reply = if input.is_ok() && data.len() <= 4096 {
        match serde_json::from_slice::<Request>(&data) {
            Ok(request) => retrieve(&request).unwrap_or_else(Reply::status),
            Err(_) => Reply::status("invalid"),
        }
    } else { Reply::status("invalid") };
    // Diagnostics never print selected text. JSON is the sole private pipe output.
    if serde_json::to_writer(std::io::stdout().lock(), &reply).is_err() { std::process::exit(1) }
    let _ = std::io::stdout().flush();
}
