//! Shortcut recording for capture and always-on-top. The Host releases its
//! global registrations while the recorder is active; the UI commits a chord
//! only after every modifier is released so restoring the registration cannot
//! immediately retrigger a WinOne action.
use super::*;
use serde_json::json;
use std::time::{Duration, Instant};
use windows_sys::Win32::UI::Input::KeyboardAndMouse::*;

pub(super) const SHORTCUT_FIELD: usize = 310;
pub(super) const SHORTCUT_CLEAR: usize = 311;
pub(super) const SHORTCUT_SAVE: usize = 312;
pub(super) const SHORTCUT_HINT: usize = 313;
pub(super) const SHORTCUT_TIMER: usize = 900;
const RECORD_SECONDS: Duration = Duration::from_secs(20);
const EM_SETCUEBANNER: u32 = 0x1501;
const MOD_CTRL: u8 = 1;
const MOD_ALT: u8 = 2;
const MOD_SHIFT: u8 = 4;
const MOD_WIN: u8 = 8;
struct ShortcutLayout{field:[i32;4],clear:[i32;4],save:[i32;4],hint:[i32;4]}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Default)]
pub(super) enum RecordPhase { #[default] Idle, Preparing, Recording, Stopping }

pub(super) struct ShortcutState {
    pub(super) draft: Value, saved: Value,
    pub(super) phase: RecordPhase,
    pub(super) token: String,
    pub(super) preview: String,
    pub(super) candidate: Option<String>,
    pub(super) deadline: Instant,
    pub(super) notice: String,
    pub(super) error: String,
    pub(super) status: Value,
    pub(super) saving: bool,
    // Modifiers observed since recording began; live key state covers chords
    // that were already held when the recording started.
    pub(super) mods: u8,
}
impl Default for ShortcutState {
    fn default() -> Self {
        Self { draft: Value::Null, saved: Value::Null, phase: RecordPhase::Idle, token: String::new(),
            preview: String::new(), candidate: None, deadline: Instant::now(), notice: String::new(),
            error: String::new(), status: Value::Null, saving: false, mods: 0 }
    }
}
impl ShortcutState {
    pub(super) fn dirty(&self) -> bool { self.draft != self.saved }
    fn reset(&mut self, settings: &Value) {
        let base = if settings.is_object() { settings.clone() } else { json!({}) };
        self.draft = base.clone(); self.saved = base;
    }
    fn clear(&mut self) {
        self.draft = Value::Null; self.saved = Value::Null; self.status = Value::Null;
        self.notice.clear(); self.error.clear();
    }
}

impl App {
    pub(super) fn shortcut_page(&self) -> bool { matches!(self.navigation.page(), "capture" | "always-on-top") }
    fn shortcut_key(&self) -> &'static str {
        if self.navigation.page() == "capture" { "captureShortcut" } else { "pinShortcut" }
    }
    fn shortcut_action(&self) -> &'static str {
        if self.navigation.page() == "capture" { "capture" } else { "pin" }
    }
    pub(super) fn shortcut_dirty(&self) -> bool { self.shortcut_page() && self.shortcut.dirty() }
    pub(super) fn recording(&self) -> bool { self.shortcut.phase != RecordPhase::Idle }
    pub(super) fn reset_shortcut(&mut self) {
        if self.shortcut_page() { self.shortcut.reset(&self.snapshot["settings"]); }
        else { self.shortcut.clear(); }
    }

    fn shortcut_layout(&self)->Result<ShortcutLayout,String>{
        let palette=self.palette.as_ref().ok_or("界面未初始化")?;
        let available=widgets::content_width(self.window,palette.scale);
        let compact=available<576;
        let buttons_y=if compact{126}else{78};
        let height=widgets::wrapped_height(self.window,palette.font,&self.shortcut_hint(),available,palette.scale).max(24);
        Ok(ShortcutLayout{field:[104,78,if compact{available-104}else{212},40],clear:[if compact{104}else{328},buttons_y,88,40],
            save:[if compact{204}else{428},buttons_y,148,40],hint:[0,buttons_y+48,0,height]})
    }
    pub(super) fn reposition_shortcut(&mut self)->Result<i32,String>{
        let layout=self.shortcut_layout()?;
        for control in &mut self.controls{
            let rect=match control.id{SHORTCUT_FIELD=>layout.field,SHORTCUT_CLEAR=>layout.clear,SHORTCUT_SAVE=>layout.save,SHORTCUT_HINT=>layout.hint,_=>continue};
            control.x=rect[0];control.y=rect[1];control.width=rect[2];control.height=rect[3];
        }
        Ok(layout.hint[1]+layout.hint[3])
    }
    pub(super) fn render_shortcut(&mut self) -> Result<i32, String> {
        let layout=self.shortcut_layout()?;
        let value = self.shortcut.draft[self.shortcut_key()].as_str().unwrap_or("").to_string();
        self.add("STATIC", &self.tr("快捷键", "Shortcut"), 0, SS_LEFT as u32, Area::Content, [0,82,96,30], false)?;
        let text = match self.shortcut.phase {
            RecordPhase::Recording => self.shortcut.preview.clone(),
            RecordPhase::Preparing => String::new(),
            _ => value,
        };
        let field = self.add("EDIT", &text, SHORTCUT_FIELD, WS_TABSTOP | WS_BORDER | ES_READONLY as u32,
            Area::Content, layout.field, false)?;
        unsafe { SendMessageW(field, EM_SETCUEBANNER, 1, wide(&self.shortcut_cue()).as_ptr() as isize); }
        self.button(&self.tr("清除", "Clear"), SHORTCUT_CLEAR, Area::Content, layout.clear)?;
        self.button(&self.tr("保存快捷键", "Save shortcut"), SHORTCUT_SAVE, Area::Content, layout.save)?;
        self.add("STATIC", &self.shortcut_hint(), SHORTCUT_HINT, 0, Area::Content, layout.hint, false)?;
        Ok(layout.hint[1]+layout.hint[3])
    }

    fn shortcut_cue(&self) -> String {
        match self.shortcut.phase {
            RecordPhase::Recording => self.tr("请按组合键", "Press a key combination"),
            RecordPhase::Preparing => String::new(),
            _ => self.tr("点击录入快捷键", "Click to record a shortcut"),
        }
    }
    fn shortcut_hint(&self) -> String {
        match self.shortcut.phase {
            RecordPhase::Preparing => self.tr("正在准备录入…", "Preparing to record…"),
            RecordPhase::Recording if !self.shortcut.error.is_empty() => self.shortcut.error.clone(),
            RecordPhase::Recording => self.tr("请按下组合键，Esc 取消，Delete 清除。", "Press your shortcut. Esc cancels; Delete clears."),
            _ if !self.shortcut.notice.is_empty() => self.shortcut.notice.clone(),
            _ => self.registration_hint(),
        }
    }
    fn registration_hint(&self) -> String {
        if !self.shortcut.status.is_object() {
            return self.tr("暂时无法确认是否生效", "Unable to confirm whether the shortcut is active");
        }
        let action = self.shortcut_action();
        if let Some(error) = self.shortcut.status["actionErrors"][action].as_str().filter(|e| !e.is_empty()) {
            return self.tr("最近一次快捷键操作失败：{error}", "Last shortcut action failed: {error}").replace("{error}", error);
        }
        if let Some(binding) = self.shortcut.status["bindings"].as_array()
            .and_then(|items| items.iter().find(|b| b["id"] == action)) {
            if let Some(error) = binding["error"].as_str().filter(|e| !e.is_empty()) { return error.into(); }
            if binding["registered"] == true {
                let key = binding["shortcut"].as_str().unwrap_or("");
                return self.tr("当前生效：{key}", "Active: {key}").replace("{key}", key);
            }
        }
        self.tr("未启用", "Not enabled")
    }
    fn update_shortcut_controls(&mut self) {
        let value = self.shortcut.draft[self.shortcut_key()].as_str().unwrap_or("").to_string();
        let text = match self.shortcut.phase {
            RecordPhase::Recording => self.shortcut.preview.clone(),
            RecordPhase::Preparing => String::new(),
            _ => value,
        };
        for control in &self.controls {
            if control.id == SHORTCUT_FIELD {
                widgets::set_text(control.window, &text);
                unsafe { SendMessageW(control.window, EM_SETCUEBANNER, 1, wide(&self.shortcut_cue()).as_ptr() as isize); }
            } else if control.id == SHORTCUT_HINT { widgets::set_text(control.window, &self.shortcut_hint()); }
        }
        if let Err(error)=self.resized(){self.status=error;}
    }
    pub(super) fn refresh_shortcut_buttons(&self) {
        let value = self.shortcut.draft[self.shortcut_key()].as_str().unwrap_or("").to_string();
        for control in &self.controls {
            match control.id {
                SHORTCUT_FIELD => widgets::enable(control.window, !self.saving() && !self.shortcut.saving),
                SHORTCUT_CLEAR => widgets::enable(control.window, self.shortcut.phase == RecordPhase::Idle
                    && !self.saving() && !self.shortcut.saving && !value.is_empty()),
                SHORTCUT_SAVE => widgets::enable(control.window, self.shortcut.phase == RecordPhase::Idle
                    && !self.saving() && !self.shortcut.saving && self.shortcut.dirty()),
                SHORTCUT_HINT => widgets::set_text(control.window, &self.shortcut_hint()),
                _ => {}
            }
        }
    }

    pub(super) fn shortcut_command(&mut self, id: usize, notification: u32) -> Result<(), String> {
        if id == SHORTCUT_FIELD && notification == EN_SETFOCUS { self.start_recording(); return Ok(()); }
        if notification != BN_CLICKED { return Ok(()); }
        match id {
            SHORTCUT_CLEAR => {
                if self.shortcut.phase == RecordPhase::Idle && !self.saving() && self.shortcut.draft.is_object()
                    && self.shortcut.draft[self.shortcut_key()].as_str().is_some_and(|v| !v.is_empty()) {
                    let key = self.shortcut_key();
                    self.shortcut.draft[key] = json!("");
                    self.update_shortcut_controls();
                }
                Ok(())
            }
            SHORTCUT_SAVE => { self.save_shortcut(); Ok(()) }
            _ => Ok(()),
        }
    }
    fn save_shortcut(&mut self) {
        if self.shortcut.phase != RecordPhase::Idle || self.saving() || !self.shortcut.dirty() { return; }
        self.shortcut.saving = true;
        if let Err(error) = self.submit(Task::ShortcutSave { value: self.shortcut.draft.clone() }) {
            self.shortcut.saving = false;
            self.status = error;
        } else { self.status = self.tr("正在保存…", "Saving…"); }
        self.refresh_buttons();
    }

    fn start_recording(&mut self) {
        if self.shortcut.phase != RecordPhase::Idle || self.saving() || !self.shortcut_page() { return; }
        self.shortcut.phase = RecordPhase::Preparing;
        self.shortcut.preview.clear();
        self.shortcut.candidate = None;
        self.shortcut.error.clear();
        self.shortcut.notice.clear();
        self.shortcut.mods = self.live_modifiers();
        if let Err(error) = self.submit(Task::ShortcutRecord) {
            self.shortcut.phase = RecordPhase::Idle;
            self.shortcut.notice = error;
        }
        self.update_shortcut_controls();
    }

    pub(super) fn finish_recording(&mut self, next: Option<&str>) {
        if !matches!(self.shortcut.phase, RecordPhase::Preparing | RecordPhase::Recording) { return; }
        if let Some(value) = next {
            if self.shortcut.draft.is_object() {
                let key = self.shortcut_key();
                self.shortcut.draft[key] = json!(value);
            }
        }
        let token = self.shortcut.token.clone();
        self.shortcut.phase = RecordPhase::Stopping;
        self.shortcut.candidate = None;
        self.shortcut.mods = 0;
        unsafe { KillTimer(self.window, SHORTCUT_TIMER); }
        if !token.is_empty() {
            if let Err(error) = self.submit(Task::ShortcutStop { token }) {
                // The Host recording watchdog (deadline or process exit) still
                // restores registrations; surface the failure without wedging.
                self.shortcut.phase = RecordPhase::Idle;
                self.shortcut.notice = error;
                self.recording_destination = None;
            }
        }
        self.update_shortcut_controls();
    }

    /// Consume a keyboard message while a recording is active. Returns false in
    /// idle state so normal dialog navigation and typing stay untouched.
    pub(super) fn record_key(&mut self, message: u32, wparam: WPARAM) -> bool {
        if self.shortcut.phase == RecordPhase::Idle { return false; }
        let vk = wparam as u16;
        match message {
            WM_KEYDOWN | WM_SYSKEYDOWN => self.recording_key_down(vk),
            WM_KEYUP | WM_SYSKEYUP => self.recording_key_up(vk),
            _ => {} // Character and dead-key translations stay swallowed.
        }
        true
    }
    fn recording_key_down(&mut self, vk: u16) {
        if self.track_modifier(vk, true) {
            self.shortcut.preview = self.modifier_labels();
            self.update_shortcut_controls();
            return;
        }
        if vk == VK_ESCAPE { self.finish_recording(None); return; }
        let modifiers = self.modifier_labels();
        if (vk == VK_DELETE || vk == VK_BACK) && modifiers.is_empty() { self.finish_recording(Some("")); return; }
        if self.alt_graph() || key_label(vk).is_none() {
            self.record_error(self.tr("请使用字母、数字或功能键。", "Use a letter, number, or function key."));
            return;
        }
        if vk == VK_F12 {
            self.record_error(self.tr("F12 为系统保留键，请换一个组合。", "F12 is reserved by Windows. Choose another key."));
            return;
        }
        if modifiers.is_empty() {
            self.record_error(self.tr("请同时按下 Ctrl、Alt、Shift 或 Win。", "Hold Ctrl, Alt, Shift, or Win with the key."));
            return;
        }
        let value = format!("{modifiers}+{}", key_label(vk).unwrap());
        self.shortcut.candidate = Some(value.clone());
        self.shortcut.preview = value;
        self.shortcut.error.clear();
        self.update_shortcut_controls();
    }
    fn recording_key_up(&mut self, vk: u16) {
        self.track_modifier(vk, false);
        // Commit only once every modifier is released, so the Host can safely
        // restore its registrations without the held chord firing an action.
        if let Some(candidate) = self.shortcut.candidate.clone().filter(|_| self.modifier_labels().is_empty()) {
            self.finish_recording(Some(&candidate));
        }
    }
    fn record_error(&mut self, message: String) {
        self.shortcut.error = message;
        self.update_shortcut_controls();
    }
    fn track_modifier(&mut self, vk: u16, pressed: bool) -> bool {
        let bit = match vk {
            VK_CONTROL | VK_LCONTROL | VK_RCONTROL => MOD_CTRL,
            VK_MENU | VK_LMENU | VK_RMENU => MOD_ALT,
            VK_SHIFT | VK_LSHIFT | VK_RSHIFT => MOD_SHIFT,
            VK_LWIN | VK_RWIN => MOD_WIN,
            _ => return false,
        };
        if pressed { self.shortcut.mods |= bit; } else { self.shortcut.mods &= !bit; }
        true
    }
    fn live_modifiers(&self) -> u8 {
        let pressed = |vk: u16| (unsafe { GetKeyState(vk as i32) } as u16) & 0x8000 != 0;
        let mut bits = 0;
        if pressed(VK_CONTROL) { bits |= MOD_CTRL; }
        if pressed(VK_MENU) { bits |= MOD_ALT; }
        if pressed(VK_SHIFT) { bits |= MOD_SHIFT; }
        if pressed(VK_LWIN) || pressed(VK_RWIN) { bits |= MOD_WIN; }
        bits
    }
    fn modifier_labels(&self) -> String {
        let bits = self.shortcut.mods | self.live_modifiers();
        let mut parts = Vec::new();
        if bits & MOD_CTRL != 0 { parts.push("Ctrl"); }
        if bits & MOD_ALT != 0 { parts.push("Alt"); }
        if bits & MOD_SHIFT != 0 { parts.push("Shift"); }
        if bits & MOD_WIN != 0 { parts.push("Win"); }
        parts.join("+")
    }
    // Ctrl+RightAlt is AltGr on many layouts: the chord would collide with
    // normal typing instead of a distinct hotkey, so reject it like the web UI.
    fn alt_graph(&self) -> bool {
        let pressed = |vk: u16| (unsafe { GetKeyState(vk as i32) } as u16) & 0x8000 != 0;
        pressed(VK_RMENU) && pressed(VK_LCONTROL) && !pressed(VK_LMENU)
    }

    pub(super) fn recording_tick(&mut self) {
        if self.shortcut.phase != RecordPhase::Recording {
            unsafe { KillTimer(self.window, SHORTCUT_TIMER); }
            return;
        }
        if Instant::now() >= self.shortcut.deadline {
            self.shortcut.notice = self.tr("录入已结束，请点击输入框重试。", "Recording ended. Click the field to try again.");
            self.finish_recording(None);
        }
    }

    pub(super) fn receive_shortcut(&mut self, reply: &worker::Reply) -> Result<bool, String> {
        let handled = match &reply.task {
            Task::ShortcutRecord => {
                match &reply.result {
                    Ok(value) => {
                        let token = value["token"].as_str().unwrap_or("").to_string();
                        if self.shortcut.phase == RecordPhase::Preparing && !token.is_empty() {
                            self.shortcut.phase = RecordPhase::Recording;
                            self.shortcut.token = token;
                            self.shortcut.deadline = Instant::now() + RECORD_SECONDS;
                            self.shortcut.preview.clear();
                            self.shortcut.candidate = None;
                            unsafe { SetTimer(self.window, SHORTCUT_TIMER, 1000, None); }
                        } else if !token.is_empty() {
                            // The recording was abandoned before the Host answered;
                            // release the token it just created.
                            self.shortcut.token = token.clone();
                            if let Err(error) = self.submit(Task::ShortcutStop { token }) {
                                self.recording_destination = None;
                                self.shortcut.phase = RecordPhase::Idle;
                                self.shortcut.notice = error;
                            }
                        } else {
                            self.recording_destination = None;
                            self.shortcut.phase = RecordPhase::Idle;
                            self.shortcut.notice = self.tr("无法开始录入，请重试。", "Unable to start recording. Retry.");
                        }
                    }
                    Err(error) if self.shortcut.phase == RecordPhase::Preparing => {
                        self.shortcut.phase = RecordPhase::Idle;
                        self.shortcut.notice = error.to_string();
                    }
                    Err(error) => {
                        self.shortcut.phase = RecordPhase::Idle;
                        self.shortcut.notice = error.to_string();
                        self.recording_destination = None;
                    }
                }
                true
            }
            Task::ShortcutStop { .. } => {
                let restored = reply.result.as_ref().is_ok_and(|value| value["stopped"] == true);
                if !restored { self.shortcut.notice = match &reply.result {
                    Err(error) => error.to_string(),
                    Ok(_) => self.tr("无法确认快捷键已恢复，请重试。", "Cannot confirm shortcut restoration. Retry."),
                }; }
                self.shortcut.phase = RecordPhase::Idle;
                unsafe { KillTimer(self.window, SHORTCUT_TIMER); }
                // Re-read registrations so conflicts and errors surface after restore.
                if let Err(error) = self.submit(Task::ShortcutStatus) { self.shortcut.notice = error; }
                let destination = self.recording_destination.take();
                if restored {
                    if let Some(destination) = destination { self.navigate(destination)?; }
                }
                true
            }
            Task::ShortcutStatus => {
                self.shortcut.status = match &reply.result { Ok(value) => value.clone(), Err(_) => Value::Null };
                true
            }
            Task::ShortcutSnapshot => {
                if let Ok(value) = &reply.result {
                    self.snapshot = value.clone();
                    if self.shortcut.phase == RecordPhase::Idle && !self.shortcut.saving && !self.shortcut.dirty() {
                        self.shortcut.reset(&self.snapshot["settings"]);
                    }
                }
                true
            }
            Task::ShortcutSave { .. } => {
                self.shortcut.saving = false;
                match &reply.result {
                    Ok(value) if value.is_object() => {
                        self.snapshot["settings"] = value.clone();
                        self.shortcut.saved = value.clone();
                        self.shortcut.draft = value.clone();
                        self.status = self.tr("已保存", "Saved");
                        let _ = self.submit(Task::ShortcutStatus);
                    }
                    Ok(_) => self.status = self.tr("无法确认保存结果，请重新读取设置。", "Cannot confirm the save. Reload settings."),
                    Err(error) => self.status = error.to_string(),
                }
                true
            }
            _ => return Ok(false),
        };
        if handled {
            self.update_shortcut_controls();
            self.refresh_buttons();
        }
        Ok(handled)
    }
}

/// The recorded vocabulary matches internal/hotkeys: letters, digits, F1–F24
/// and numpad keys, with one display name shared by the settings value.
fn key_label(vk: u16) -> Option<String> {
    match vk {
        0x41..=0x5A => Some((((vk - 0x41) as u8 + b'A') as char).to_string()),
        0x30..=0x39 => Some((((vk - 0x30) as u8 + b'0') as char).to_string()),
        0x60..=0x69 => Some(format!("Num{}", vk - 0x60)),
        0x70..=0x87 => Some(format!("F{}", vk - 0x6F)),
        _ => None,
    }
}
