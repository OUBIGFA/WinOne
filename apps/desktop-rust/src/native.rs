//! Native settings shell. The Host continues to own configuration and module lifetimes.
use crate::{widgets::{self, Area, Control, Palette, wide}, worker::{self, Task, Worker}};
use serde_json::{Value, json};
use std::{cell::RefCell, collections::HashMap, ptr::{null, null_mut}};
use winone_desktop::state::{Draft, Destination, Navigation, NavigationResult, Page, build_navigation};
// windows-rs places static-control styles in SystemServices, not UI::Controls.
use windows_sys::Win32::System::SystemServices::{SS_ENDELLIPSIS, SS_ICON, SS_LEFT};
use windows_sys::{core::w, Win32::{Foundation::*, Graphics::Gdi::*, System::LibraryLoader::GetModuleHandleW, UI::{Controls::*, HiDpi::*, WindowsAndMessaging::*}}};

#[cfg(test)]
#[path = "native_tests.rs"]
mod tests;
#[path = "native_programs.rs"]
mod program_view;
#[path = "native_logs.rs"]
mod log_view;
#[path = "native_shortcut.rs"]
mod shortcut_view;
#[path = "native_management.rs"]
mod management_view;
#[path = "native_profiles.rs"]
mod profile_view;
#[path = "native_capture.rs"]
mod capture_view;
#[path = "native_permissions.rs"]
mod permission_view;
#[path = "native_updates.rs"]
mod updates_view;

const NAV: usize = 100;
const TAB: usize = 200;
const SAVE: usize = 300;
const RELOAD: usize = 301;
const STAY: usize = 302;
const DISCARD: usize = 303;
const STATUS: usize = 400;
const FIELD: usize = 500;
const COLOR_DIALOG_MESSAGE:u32=WM_APP+4;
// Forms share this design-grid origin. Header layout translates the entire
// body once, so shortcut text and wrapped actions cannot overlap the form.
const BODY_TOP:i32=194;

#[derive(Clone)]
enum FieldKind { Choice(Vec<Value>), Boolean, Text, Number(i64, i64), Lines, Color }
#[derive(Clone)]
struct Binding { id: usize, key: String, kind: FieldKind, program: Option<String> }

struct App {
    window: HWND, palette: Option<Palette>, controls: Vec<Control>, rich_text: Option<widgets::RichEdit>, worker: Option<Worker>,
    navigation: Navigation, pages: Vec<Page>, catalog: Value, snapshot: Value,
    draft: Option<Draft>, module_data: Value, tabs: HashMap<String, usize>, bindings: Vec<Binding>,
    invalid: HashMap<String, String>, loading: bool, generation: u64, status: String,
    scroll: i32, content_height: i32,
    body_offset:i32,current_body_offset:i32,rendering_body:bool,
    program_viewport:Option<crate::viewport::Viewport>,program_scroll:i32,
    selected_program: String, program_tab: usize, field_program: Option<String>,
    launching: bool,
    dialog_open: bool,
    color_request: Option<String>,
    icons: HashMap<String, Option<HICON>>,
    logs: log_view::Diagnostics,
    shortcut: shortcut_view::ShortcutState,
    management: management_view::Management,
    profiles: profile_view::ProfileState,
    recording_destination: Option<Destination>,
    capture: capture_view::CaptureState,
    permission: permission_view::PermissionState,
    updates:updates_view::UpdateState,
}
impl App {
    fn new() -> Self {
        Self { window: null_mut(), palette: None, controls: Vec::new(), rich_text: None, worker: None,
            navigation: Navigation::new("overview"), pages: vec![Page { id:"overview".into(), title:"首页".into() }, Page { id:"settings".into(), title:"设置".into() }], catalog: Value::Null, snapshot: Value::Null,
            draft: None, module_data: Value::Null, tabs: HashMap::new(), bindings: Vec::new(), invalid: HashMap::new(),
            loading: true, generation: 0, status: "正在读取设置…".into(), scroll: 0, content_height: 0,body_offset:0,current_body_offset:0,rendering_body:false,
            selected_program: String::new(), program_tab: 0, field_program: None, program_viewport:None,program_scroll:0,launching: false, dialog_open:false, color_request:None,
            icons: HashMap::new(), logs: Default::default(), shortcut: Default::default(), management: Default::default(), profiles: Default::default(), recording_destination: None, capture: Default::default(), permission:Default::default(),updates:Default::default() }
    }
    fn tr(&self, chinese: &str, english: &str) -> String {
        if self.snapshot["locale"] == "en-US" || self.snapshot["settings"]["language"] == "en-US" { english.into() } else { chinese.into() }
    }
    fn theme(&self) -> &str { self.snapshot["settings"]["theme"].as_str().unwrap_or("system") }
    fn tab(&self) -> usize { *self.tabs.get(self.navigation.page()).unwrap_or(&0) }
    fn dirty(&self) -> bool { self.draft.as_ref().is_some_and(Draft::is_dirty) || !self.invalid.is_empty() }
    fn any_dirty(&self) -> bool { self.dirty() || self.shortcut_dirty() || self.profiles.preview.is_object() }
    fn saving(&self) -> bool { self.permission_busy() || self.dialog_open || self.launching || self.draft.as_ref().is_some_and(Draft::is_saving) || self.shortcut.saving || self.management.busy || self.profiles.busy || self.capture.busy || self.capture.picking }
    fn valid_program_names(&self) -> bool {
        self.navigation.page() != "wintray" || self.draft.as_ref().and_then(|d| d.value()["managedApps"].as_array())
            .is_some_and(|items| items.iter().all(|p| p["name"].as_str().is_some_and(|s| !s.trim().is_empty())))
    }
    /// Icon requests settle per path for this window: a settled path is never
    /// re-requested, so failed or icon-less files cost at most one attempt.
    fn request_program_icons(&mut self) {
        let Some(paths) = self.draft.as_ref().and_then(|draft| draft.value()["managedApps"].as_array()).map(|items| {
            items.iter().filter_map(|program| program["exePath"].as_str()).map(str::to_string).collect::<Vec<_>>()
        }) else { return };
        for path in paths {
            if self.icons.contains_key(&path) { continue; }
            // A busy worker retries on the next reply; the path is recorded
            // only after submission so the backlog cannot be lost.
            if self.submit(Task::ProgramIcon { path: path.clone() }).is_err() { return; }
            self.icons.insert(path, None);
        }
    }
    fn receive_icon(&mut self, reply: &worker::Reply) -> Result<bool, String> {
        let Task::ProgramIcon { path } = &reply.task else { return Ok(false); };
        let icon = reply.result.as_ref().ok()
            .and_then(|value| value["icon"].as_str())
            .and_then(crate::icon::base64_decode)
            .and_then(|pixels| crate::icon::create_icon(&pixels));
        let previous=self.icons.insert(path.clone(), icon).flatten();
        self.request_program_icons();
        if self.navigation.page() == "wintray" && self.tab() == 0 && self.draft.is_some() { self.refresh_program_icons(); }
        // Replace the native image reference before releasing the prior icon.
        if let Some(previous)=previous {crate::icon::destroy(previous);}
        Ok(true)
    }
    fn submit(&self, task: Task) -> Result<(), String> {
        self.worker.as_ref().ok_or("后台连接未就绪")?.submit(task).map_err(|e| e.to_string())
    }
    fn add(&mut self, class: &str, title: &str, id: usize, style: u32, area: Area, rect: [i32; 4], heading: bool) -> Result<HWND, String> {
        let area = if self.field_program.is_some() && matches!(area, Area::Content) { Area::ProgramDetail } else { area };
        let mut rect=rect;if !matches!(area,Area::ProgramDetail){rect[1]+=self.body_offset;}
        let parent=if matches!(area,Area::ProgramDetail){self.program_viewport.as_ref().ok_or("程序详情窗口未初始化")?.window}else{self.window};
        let palette = self.palette.as_ref().ok_or("界面未初始化")?;
        let mut control = Control::new(parent, class, "", id, style, if heading { palette.heading } else { palette.font }, area, rect)?;
        control.body=self.rendering_body;
        widgets::apply_control_theme(control.window,class,palette)?;
        if matches!(area,Area::ProgramDetail){crate::viewport::observe_focus(control.window)?;}
        // A wrapping edit must have its real width before receiving long text.
        // Feeding it at the constructor's 1px size can stall the UI for seconds.
        let mut client = RECT::default(); unsafe { GetClientRect(self.window, &mut client); }
        if matches!(area,Area::ProgramDetail){unsafe{GetClientRect(parent,&mut client);}control.place_local(palette,(client.right as f32/palette.scale)as i32,self.program_scroll);}
        else{control.place(palette, client, self.scroll);}
        let window = control.window; widgets::set_text(window, title);
        self.controls.push(control); Ok(window)
    }
    fn label(&mut self, text: &str, y: i32, heading: bool) -> Result<(), String> {
        self.add("STATIC", text, 0, SS_LEFT as u32, Area::Content, [0, y, 0, if heading { 36 } else { 38 }], heading)?; Ok(())
    }
    fn button(&mut self, title: &str, id: usize, area: Area, rect: [i32; 4]) -> Result<HWND, String> {
        let area = if self.field_program.is_some() && matches!(area, Area::Content) { Area::ProgramDetail } else { area };
        let mut rect=rect;if !matches!(area,Area::ProgramDetail){rect[1]+=self.body_offset;}
        let parent=if matches!(area,Area::ProgramDetail){self.program_viewport.as_ref().ok_or("程序详情窗口未初始化")?.window}else{self.window};
        let mut control = Control::button(parent, title, id, self.palette.as_ref().ok_or("界面未初始化")?, area, rect)?;
        if matches!(area,Area::ProgramDetail){
            crate::viewport::observe_focus(control.window)?;
            let mut client=RECT::default();unsafe{GetClientRect(parent,&mut client);}
            let palette=self.palette.as_ref().unwrap();control.place_local(palette,(client.right as f32/palette.scale)as i32,self.program_scroll);
        }
        control.body=self.rendering_body;
        let window = control.window; self.controls.push(control); Ok(window)
    }
    fn render(&mut self) -> Result<(), String> {
        self.body_offset=0;self.current_body_offset=0;self.rendering_body=false;
        self.controls.clear();self.program_viewport.take(); self.bindings.clear(); self.field_program = None; self.content_height = 500;
        self.add("STATIC", "WinOne", 0, 0, Area::Sidebar, [12, 20, 0, 36], true)?;
        for (index, page) in self.pages.clone().into_iter().enumerate() {
            let area = if page.id == "settings" { Area::Bottom } else { Area::Sidebar };
            self.button(&page.title, NAV + index, area, [0, 78 + index as i32 * 48, 0, 42])?;
        }
        let title = self.pages.iter().find(|p| p.id == self.navigation.page()).map(|p| p.title.clone()).unwrap_or_else(|| self.tr("首页", "Home"));
        self.label(&title, 28, true)?;
        if self.navigation.pending().is_some() {
            self.label(&self.tr("尚有未保存的修改。", "You have unsaved changes."), 78, false)?;
            self.button(&self.tr("继续编辑", "Keep editing"), STAY, Area::Content, [0, 112, 120, 36])?;
            self.button(&self.tr("放弃修改", "Discard changes"), DISCARD, Area::Content, [136, 112, 140, 36])?;
        } else {
            match self.navigation.page() {
                "overview" => self.render_management()?,
                "settings" => self.render_settings()?,
                _ => self.render_module()?,
            }
        }
        self.add("STATIC", &self.status.clone(), STATUS, 0, Area::Content, [0, self.content_height - 68, 0, 62], false)?;
        self.refresh_buttons(); self.layout();
        unsafe { InvalidateRect(self.window, null(), 1); }
        Ok(())
    }
    fn render_settings(&mut self) -> Result<(), String> {
        let names = [self.tr("外观与语言", "Appearance"), self.tr("启动设置", "Startup"), self.tr("配置迁移", "Configuration"), self.tr("诊断日志", "Diagnostics"),self.tr("更新","Updates")];
        for (index, name) in names.iter().enumerate() { self.button(name, TAB+index, Area::Tabs { index, count:names.len() }, [0,96,0,36])?; }
        self.render_body(154,Self::render_settings_body)
    }
    fn render_body(&mut self,top:i32,render:impl FnOnce(&mut Self)->Result<(),String>)->Result<(),String>{
        self.body_offset=top-BODY_TOP;
        self.current_body_offset=self.body_offset;self.rendering_body=true;
        let result=render(self);
        self.content_height+=self.body_offset;self.body_offset=0;self.rendering_body=false;
        result
    }
    fn render_settings_body(&mut self)->Result<(),String>{
        if self.tab() == 2 { return self.render_profiles(); }
        if self.tab() == 3 { return self.render_logs(); }
        if self.tab() == 4 {return self.render_updates();}
        if self.draft.is_some() {
            if self.tab() == 0 {
                self.choice("theme", &self.tr("主题", "Theme"), 194, &["system", "light", "dark"], &[self.tr("跟随系统", "System"), self.tr("浅色", "Light"), self.tr("深色", "Dark")])?;
                self.choice("language", &self.tr("语言", "Language"), 280, &["system", "zh-CN", "en-US"], &[self.tr("跟随系统", "System"), "简体中文".into(), "English".into()])?;
            } else {
                self.check("startup.runAtLogon", &self.tr("登录时启动 WinOne", "Start WinOne at sign-in"), 194)?;
                self.check("startup.startMinimized", &self.tr("启动时不显示设置窗口", "Start without opening settings"), 242)?;
                self.actions(298)?;
                return self.render_permissions();
            }
            self.actions(390)?;
        } else {
            self.label(&self.tr("设置尚未载入。", "Settings are not loaded."), 194, false)?;
            self.button(&self.tr("重试", "Retry"), RELOAD, Area::Content, [0, 252, 112, 36])?;
        }
        self.content_height = 520; Ok(())
    }
    fn render_module(&mut self) -> Result<(), String> {
        let page = self.navigation.page().to_string();
        let names: Vec<String> = match page.as_str() {
            "always-on-top" => vec![self.tr("常规设置", "General"), self.tr("高级设置", "Advanced"), self.tr("诊断日志", "Diagnostics")],
            "capture" => vec![self.tr("截图后", "After capture"), self.tr("文件名模板", "File name"), self.tr("滚动截图", "Scrolling"), self.tr("诊断日志", "Diagnostics")],
            "wintray" => vec![self.tr("程序列表", "Programs"), self.tr("启动设置", "Startup"), self.tr("诊断日志", "Diagnostics")],
            _ => vec![self.tr("常规设置", "General"), self.tr("诊断日志", "Diagnostics")],
        };
        let tabs_top=if self.shortcut_page(){self.render_shortcut()?+12}else{96};
        for (index, name) in names.iter().enumerate() { self.button(name, TAB + index, Area::Tabs { index, count: names.len() }, [0, tabs_top, 0, 36])?; }
        self.render_body(tabs_top+58,|app|app.render_module_body(&page,names.len()))
    }
    fn render_module_body(&mut self,page:&str,tabs:usize)->Result<(),String>{
        if self.tab() == tabs - 1 {
            self.render_logs()?;
        } else if self.draft.is_some() {
            match page {
                "always-on-top" => self.render_pin()?,
                "capture" => self.render_capture()?,
                "wintray" => self.render_programs()?,
                _ => self.label(&self.tr("模块已接入，设置页面待迁移。", "Module connected. Its settings page is not migrated yet."), 194, false)?,
            }
        } else {
            self.label(&self.tr("正在读取设置…", "Loading settings…"), 194, false)?;
            self.button(&self.tr("重试", "Retry"), RELOAD, Area::Content, [0, 252, 112, 36])?;
        }
        if page=="capture" { self.render_capture_actions()?; }
        Ok(())
    }
    fn render_pin(&mut self) -> Result<(), String> {
        if self.tab() == 0 {
            self.check("frame", &self.tr("显示边框", "Show border"), 194)?;
            self.check("useAccent", &self.tr("使用系统强调色", "Use system accent color"), 238)?;
            self.label(&self.tr("边框颜色", "Border color"),282,false)?;
            let id=FIELD+self.bindings.len();
            let color=self.field_value("frameColor").as_str().unwrap_or("").to_owned();
            self.button(&format!("{}  {color}",self.tr("选择颜色","Choose color")),id,Area::Content,[0,316,280,36])?;
            self.bindings.push(Binding{id,key:"frameColor".into(),kind:FieldKind::Color,program:None});
            self.check("roundCorners", &self.tr("圆角边框", "Rounded border"), 368)?;
            self.edit("frameThickness", &self.tr("边框宽度（1–15）", "Border width (1–15)"), 418, FieldKind::Number(1,15))?;
            self.edit("frameOpacity", &self.tr("边框不透明度（10–100）", "Border opacity (10–100)"), 502, FieldKind::Number(10,100))?;
            self.check("sound", &self.tr("播放提示音", "Play a sound"), 594)?;
            self.actions(652)?; self.content_height = 776;
        } else {
            self.check("blockInGameMode", &self.tr("游戏模式下禁用", "Disable in Game Mode"), 194)?;
            self.edit("excludedApps", &self.tr("排除的程序（每行一个）", "Excluded applications (one per line)"), 246, FieldKind::Lines)?;
            self.actions(438)?; self.content_height = 560;
        }
        Ok(())
    }
    fn render_capture(&mut self) -> Result<(), String> {
        match self.tab() {
            0 => {
                self.check("copyToClipboard", &self.tr("复制到剪贴板", "Copy to clipboard"), 194)?;
                self.check("askWhereToSave", &self.tr("选择保存位置", "Choose where to save"), 238)?;
                self.choice_values("openEditor", &self.tr("完成截图后", "After selecting a capture"), 282, &[json!(true),json!(false)],
                    &[self.tr("截图后编辑","Edit after capture"),self.tr("截完保存","Save after capture")])?;
                self.check("pinAfterCapture", &self.tr("贴图", "Pin image"), 378)?;
                self.edit("outputDirectory", &self.tr("保存文件夹", "Save folder"), 428, FieldKind::Text)?;
                self.button(&self.tr("选择文件夹","Choose folder"),capture_view::PICK_OUTPUT,Area::Content,[0,512,160,36])?;
                self.actions(570)?; self.content_height = 702;
            }
            1 => self.render_filename()?,
            _ => { self.edit("scrollDelay", &self.tr("滚动间隔（100–2000 毫秒）", "Scroll interval (100–2000 ms)"), 194, FieldKind::Number(100,2000))?;
                self.check("scrollFromTop", &self.tr("从顶部开始", "Start at the top"), 290)?; self.actions(356)?; self.content_height = 490; }
        }
        Ok(())
    }
    fn render_programs(&mut self) -> Result<(), String> {
        if self.tab() == 0 {
            self.render_program_editor()?;
        } else {
            self.check("exitAfterManagedAppsCompleted", &self.tr("登录任务完成后，无常驻功能时退出 WinOne", "Exit WinOne after sign-in tasks when no tools need it"), 194)?;
            self.edit("startupIntervalSeconds", &self.tr("程序之间的启动间隔（0–120 秒）", "Delay between programs (0–120 seconds)"), 250, FieldKind::Number(0,120))?;
            self.edit("closeWindowRetrySeconds", &self.tr("等待程序窗口（0–120 秒）", "Wait for program windows (0–120 seconds)"), 338, FieldKind::Number(0,120))?;
            self.actions(444)?; self.content_height = 568;
        }
        Ok(())
    }
    fn choice(&mut self, key: &str, label: &str, y: i32, values: &[&str], labels: &[String]) -> Result<(), String> {
        self.choice_values(key,label,y,&values.iter().map(|value|json!(value)).collect::<Vec<_>>(),labels)
    }
    fn choice_values(&mut self, key: &str, label: &str, y: i32, values: &[Value], labels: &[String]) -> Result<(), String> {
        self.label(label, y, false)?;
        let id = FIELD + self.bindings.len();
        let value = self.field_value(key);
        let window = self.add("COMBOBOX", "", id, WS_TABSTOP | CBS_DROPDOWNLIST as u32 | WS_VSCROLL, Area::Content, [0,y+30,280,220], false)?;
        for label in labels { unsafe { SendMessageW(window, CB_ADDSTRING, 0, wide(label).as_ptr() as isize); } }
        unsafe { SendMessageW(window, CB_SETCURSEL, values.iter().position(|v| *v == value).unwrap_or(0), 0); }
        self.bindings.push(Binding { id, key: key.into(), kind: FieldKind::Choice(values.to_vec()), program: self.field_program.clone() });
        Ok(())
    }
    fn check(&mut self, key: &str, label: &str, y: i32) -> Result<(), String> {
        let id = FIELD + self.bindings.len();
        let value = self.field_value(key) == true;
        let window = self.add("BUTTON", label, id, WS_TABSTOP | BS_AUTOCHECKBOX as u32, Area::Content, [0,y,0,34], false)?;
        unsafe { SendMessageW(window, BM_SETCHECK, if value { BST_CHECKED } else { BST_UNCHECKED } as usize, 0); }
        self.bindings.push(Binding { id, key: key.into(), kind: FieldKind::Boolean, program: self.field_program.clone() }); Ok(())
    }
    fn edit(&mut self, key: &str, label: &str, y: i32, kind: FieldKind) -> Result<(), String> {
        self.label(label, y, false)?;
        let id = FIELD + self.bindings.len();
        let value = self.field_value(key);
        let storage_key = self.field_program.as_ref().map(|id| format!("{id}/{key}")).unwrap_or_else(|| key.into());
        let text = if let Some(raw) = self.invalid.get(&storage_key) { raw.clone() } else { match &kind {
            FieldKind::Lines => value.as_array().map(|a| a.iter().filter_map(Value::as_str).collect::<Vec<_>>().join("\r\n")).unwrap_or_default(),
            _ => value.as_str().map(str::to_owned).unwrap_or_else(|| if value.is_null() { String::new() } else { value.to_string() }),
        }};
        let multiline = matches!(kind, FieldKind::Lines);
        let style = WS_TABSTOP | WS_BORDER | if multiline { ES_MULTILINE as u32 | ES_AUTOVSCROLL as u32 | WS_VSCROLL } else { ES_AUTOHSCROLL as u32 };
        self.add("EDIT", &text, id, style, Area::Content, [0,y+32,0,if multiline { 130 } else { 34 }], false)?;
        self.bindings.push(Binding { id, key: key.into(), kind, program: self.field_program.clone() }); Ok(())
    }
    fn actions(&mut self, y: i32) -> Result<(), String> {
        self.button(&self.tr("保存修改", "Save changes"), SAVE, Area::Content, [0,y,132,36])?;
        self.button(&self.tr("撤销修改", "Undo changes"), RELOAD, Area::Content, [148,y,132,36])?; Ok(())
    }
    fn refresh_buttons(&self) {
        for control in &self.controls {
            if control.id == SAVE { widgets::enable(control.window, self.dirty() && !self.saving() && self.invalid.is_empty() && self.valid_program_names()); }
            else if control.id == RELOAD { widgets::enable(control.window, !self.saving() && !self.loading); }
            else if (FIELD..1000).contains(&control.id) { widgets::enable(control.window, !self.saving()); }
            else if (354..=356).contains(&control.id) || control.id == 452 { widgets::enable(control.window, !self.saving()); }
            else if control.id == 357 { widgets::enable(control.window, !self.saving() && !self.dirty() && self.program_module_enabled()); }
            if control.id == STATUS { widgets::set_text(control.window, &self.status); }
        }
        self.refresh_shortcut_buttons();
        self.refresh_profile_buttons();
        self.refresh_capture_buttons();
        self.refresh_permission_buttons();
        self.refresh_update_buttons();
        if self.navigation.page()=="always-on-top" {
            for binding in &self.bindings {
                if matches!(binding.key.as_str(),"frameColor"|"frameThickness"|"frameOpacity"|"useAccent"|"roundCorners") {
                    if let Some(control)=self.controls.iter().find(|c|c.id==binding.id) {
                        let enabled=!self.saving() && !self.loading && self.field_value("frame")==true
                            && (binding.key!="frameColor" || self.field_value("useAccent")!=true);
                        widgets::enable(control.window,enabled);
                    }
                }
            }
        }
    }
    fn field_changed(&mut self, id: usize, notification: u32, window: HWND) {
        if self.saving(){return;}
        let Some(binding) = self.bindings.iter().find(|b| b.id == id).cloned() else { return; };
        let storage_key = binding.program.as_ref().map(|id| format!("{id}/{}", binding.key)).unwrap_or_else(|| binding.key.clone());
        let value = match &binding.kind {
            FieldKind::Choice(values) if notification == CBN_SELCHANGE => {
                let index = unsafe { SendMessageW(window, CB_GETCURSEL, 0, 0) };
                let Some(value) = values.get(index as usize) else { return; }; value.clone()
            }
            FieldKind::Boolean if notification == BN_CLICKED => json!(unsafe { SendMessageW(window, BM_GETCHECK, 0, 0) } == BST_CHECKED as isize),
            FieldKind::Text | FieldKind::Number(_, _) | FieldKind::Lines if notification == EN_CHANGE => {
                let raw = widgets::text(window);
                match binding.kind {
                    FieldKind::Number(min, max) => match raw.parse::<i64>() {
                        Ok(value) if value >= min && value <= max => { self.invalid.remove(&storage_key); json!(value) }
                        _ => { self.invalid.insert(storage_key, raw); self.status = self.tr("请输入范围内的整数。", "Enter a whole number within the range."); self.refresh_buttons(); return; }
                    },
                    FieldKind::Lines => json!(raw.lines().map(str::trim).filter(|v| !v.is_empty()).collect::<Vec<_>>()),
                    _ => json!(raw),
                }
            }
            _ => return,
        };
        if let Some(draft) = &mut self.draft {
            let result = if let Some(id) = &binding.program {
                winone_desktop::programs::updated(draft.value(), id, &binding.key, value).and_then(|items| draft.set("managedApps", items))
            } else if let Some(key) = binding.key.strip_prefix("startup.") {
                let mut startup = draft.value()["startup"].clone();
                if !startup.is_object() { startup = json!({}); }
                startup[key] = value;
                draft.set("startup", startup)
            } else { draft.set(&binding.key, value) };
            if let Err(error) = result { self.status = error.to_string(); }
            else { self.status = if !self.valid_program_names() { self.tr("请在更多设置中填写程序名称。", "Enter the program name in More settings.") }
                else if self.dirty() { self.tr("有未保存的修改", "Unsaved changes") } else { String::new() }; }
        }
        self.refresh_buttons();
        if self.navigation.page()=="capture" && binding.key=="fileNameTemplate" {self.update_filename_preview();}
        if binding.program.is_some() && matches!(binding.key.as_str(), "mode" | "schedule.enabled" | "schedule.frequencyEnabled") {
            if let Err(error) = self.refresh_program_details(false) { self.status = error; self.refresh_buttons(); }
        }
    }
    fn pick_frame_color(&mut self)->Result<(),String>{
        if self.saving() || self.loading || self.field_value("frame")!=true || self.field_value("useAccent")==true {return Ok(());}
        let current=self.field_value("frameColor").as_str().ok_or("边框颜色无效")?.to_owned();
        self.dialog_open=true; self.color_request=Some(current);self.refresh_buttons();
        // Open after this command releases its App borrow. The native dialog's
        // message loop must still paint the owner and receive worker replies.
        if unsafe{PostMessageW(self.window,COLOR_DIALOG_MESSAGE,0,0)}==0{
            self.dialog_open=false;self.color_request=None;self.refresh_buttons();
            return Err(std::io::Error::last_os_error().to_string());
        }
        Ok(())
    }
    fn finish_frame_color(&mut self,selected:Result<Option<String>,String>)->Result<(),String>{
        self.dialog_open=false;
        match selected {
            Ok(Some(color))=>{if let Some(draft)=&mut self.draft{draft.set("frameColor",json!(color)).map_err(|e|e.to_string())?;}self.status=self.tr("有未保存的修改","Unsaved changes");},
            Ok(None)=>{},
            Err(error)=>self.status=error,
        }
        self.render()
    }
    fn navigate(&mut self, destination: Destination) -> Result<(), String> {
        // Leaving the page must restore Host registrations before navigating.
        if self.recording() {
            self.recording_destination = Some(destination);
            self.finish_recording(None);
            self.status = self.tr("正在恢复快捷键…", "Restoring shortcuts…");
            self.refresh_buttons();
            return Ok(());
        }
        match self.navigation.request(destination.clone(), self.any_dirty(), self.saving()) {
            NavigationResult::Busy => {
                // Both operations protect navigation, but the feedback must name
                // the operation the user is actually waiting for.
                self.status = if self.launching { self.tr("正在启动程序，请稍候。", "Starting program. Please wait.") }
                    else if self.permission_busy(){self.tr("正在更新运行权限，请稍候。","Updating permissions. Please wait.")}
                    else if self.dialog_open {self.tr("请先完成颜色选择。","Finish choosing the color first.")}
                    else if self.management.busy || self.profiles.busy || self.capture.busy || self.capture.picking { self.tr("正在处理，请稍候。", "Working. Please wait.") }
                    else { self.tr("正在保存，请稍候。", "Saving. Please wait.") };
                self.refresh_buttons();
            }
            NavigationResult::Confirm => self.render()?,
            NavigationResult::Navigate => {
                if destination == Destination::Close { self.leave_updates();unsafe { DestroyWindow(self.window); } }
                else { self.open_page()?; }
            }
        }
        Ok(())
    }
    fn open_page(&mut self) -> Result<(), String> {
        self.leave_updates();
        self.generation += 1; self.draft = None; self.invalid.clear(); self.module_data = Value::Null; self.scroll = 0;self.program_scroll=0;
        self.reset_logs();
        self.reset_shortcut();
        self.capture=Default::default();
        self.profiles = Default::default();
        match self.navigation.page() {
            "overview" => { self.status.clear(); self.load_management()?; }
            // Refresh the revision from Host on every entry/reload. Reusing the
            // bootstrap snapshot would make conflict recovery repeat the stale save.
            page => { self.loading = true; self.status = self.tr("正在读取设置…", "Loading settings…"); self.submit(Task::Load { page: page.into(), generation: self.generation })?; }
        }
        // The shortcut section edits the global settings object, so it needs a
        // fresh snapshot and registration status on every entry.
        if self.shortcut_page() {
            self.submit(Task::ShortcutSnapshot)?;
            self.submit(Task::ShortcutStatus)?;
        }
        if self.showing_logs() { self.load_logs(); }
        self.render()
    }
    fn command(&mut self, id: usize, notification: u32, window: HWND) -> Result<(), String> {
        if id == 452 && notification == CBN_SELCHANGE {
            if self.saving() { return Ok(()); }
            let position = unsafe { SendMessageW(window, CB_GETCURSEL, 0, 0) };
            if let Some(draft) = &mut self.draft {
                let items = winone_desktop::programs::reordered(draft.value(), &self.selected_program, position as usize).map_err(|e| e.to_string())?;
                draft.set("managedApps", items).map_err(|e| e.to_string())?;
            }
            self.render()?;
            return Ok(());
        }
        if id == 450 && notification == LBN_SELCHANGE {
            let index = unsafe { SendMessageW(window, LB_GETCURSEL, 0, 0) };
            if let Some(id) = self.draft.as_ref().and_then(|d| d.value()["managedApps"].as_array())
                .and_then(|items| items.get(index as usize)).and_then(|p| p["id"].as_str()) {
                self.selected_program = id.into(); self.refresh_program_details(true)?;
            }
            return Ok(());
        }
        if (shortcut_view::SHORTCUT_FIELD..=shortcut_view::SHORTCUT_HINT).contains(&id) {
            self.shortcut_command(id, notification)?;
            return Ok(());
        }
        // While a recording is active the recorder owns the keyboard; other page
        // actions would save or discard drafts the user cannot type into.
        if self.recording() && !((NAV..TAB).contains(&id) || (TAB..SAVE).contains(&id) || id == STAY || id == DISCARD) {
            self.status = self.tr("录入中，请先完成快捷键录入。", "Finish recording the shortcut first.");
            self.refresh_buttons();
            return Ok(());
        }
        if notification == BN_CLICKED && self.management_command(id)? { return Ok(()); }
        if notification == BN_CLICKED && self.capture_command(id)? { return Ok(()); }
        if notification == BN_CLICKED && self.permission_command(id,window)? {return Ok(());}
        if notification == BN_CLICKED && self.updates_command(id)? {return Ok(());}
        if self.profile_command(id, notification)? { return Ok(()); }
        if id >= FIELD {
            if notification==BN_CLICKED && self.bindings.iter().any(|b|b.id==id && matches!(b.kind,FieldKind::Color)){self.pick_frame_color()?;}
            else {self.field_changed(id, notification, window);}
            return Ok(());
        }
        if notification != BN_CLICKED { return Ok(()); }
        if (NAV..TAB).contains(&id) {
            if let Some(page) = self.pages.get(id - NAV) { self.navigate(Destination::Page(page.id.clone()))?; }
        } else if (TAB..SAVE).contains(&id) {
            if self.showing_updates() && id-TAB!=4{self.leave_updates();}
            self.tabs.insert(self.navigation.page().into(), id - TAB);
            if self.showing_logs() { self.load_logs(); }
            self.render()?;
        }
        else { match id {
            log_view::REFRESH_LOGS | log_view::OPEN_LOG_FOLDER | 330..=333 => self.logs_command(id)?,
            350 | 351 => { self.program_tab = id - 350; self.refresh_program_details(true)?; }
            354 => self.remove_selected_program()?,
            357 => {
                if !self.saving() && !self.dirty() && self.program_module_enabled() && !self.selected_program.is_empty() {
                    self.submit(Task::Launch { id:self.selected_program.clone() })?;
                    self.launching = true; self.status = self.tr("正在启动程序…", "Starting program…"); self.refresh_buttons();
                }
            }
            355 | 356 => {
                if !self.saving() {
                    let paths = crate::dialogs::pick_programs(self.window, &self.tr("选择程序", "Choose programs"), id == 356)?;
                    self.apply_program_paths(&paths, id == 355)?;
                }
            }
            SAVE => {
                if !self.saving() && self.invalid.is_empty() && self.valid_program_names() {
                    if let Some(draft) = &mut self.draft {
                        let value = draft.begin_save().map_err(|e| e.to_string())?;
                        // Program management revisions belong to the snapshot,
                        // not its settings object. Preserve that envelope on save.
                        let value = if self.navigation.page() == "wintray" {
                            json!({"revision":self.module_data["revision"], "settings":value})
                        } else { value };
                        if let Err(error) = self.submit(Task::Save { page: self.navigation.page().into(), value }) { self.draft.as_mut().unwrap().save_failed(); return Err(error); }
                        self.status = self.tr("正在保存…", "Saving…"); self.refresh_buttons();
                    }
                }
            }
            RELOAD => {
                if self.navigation.page() == "overview" { self.status.clear(); self.load_management()?; self.render()?; }
                else { self.navigate(Destination::Page(self.navigation.page().into()))?; }
            }
            STAY => { self.navigation.cancel(); self.render()?; }
            DISCARD => if let Some(destination) = self.navigation.discard(self.saving()) {
                if destination == Destination::Close { unsafe { DestroyWindow(self.window); } } else { self.open_page()?; }
            },
            _ => {}
        }}
        Ok(())
    }
    fn replies(&mut self) -> Result<(), String> {
        while let Some(reply) = self.worker.as_ref().and_then(|w| w.replies.try_recv().ok()) {
            self.flush_update_cancel();
            if self.receive_logs(&reply)? { continue; }
            if self.receive_shortcut(&reply)? { continue; }
            if self.receive_icon(&reply)? { continue; }
            if self.receive_management(&reply)? { continue; }
            if self.receive_profile(&reply)? { continue; }
            if self.receive_capture(&reply)? { continue; }
            if self.receive_permission(&reply)? {if unsafe{IsWindow(self.window)}==0{return Ok(());}continue;}
            if self.receive_updates(&reply)?{continue;}
            if let Task::Load { page, generation } = &reply.task {
                if page != self.navigation.page() || *generation != self.generation { continue; }
            }
            self.loading = false;
            if matches!(reply.task, Task::Launch { .. }) { self.launching = false; }
            match reply.result {
                Err(error) => { if let Some(draft) = &mut self.draft { draft.save_failed(); } self.status = error.to_string(); }
                Ok(value) => match reply.task {
                    Task::Capture {..}|Task::CaptureSnapshot {..}|Task::CaptureAction {..}|Task::CaptureFileName {..}=>unreachable!("capture replies are consumed above"),
                    Task::Permission {..}=>unreachable!("permission replies are consumed above"),
                    Task::UpdateInfo{..}|Task::UpdateCheck{..}|Task::UpdateStatus{..}|Task::UpdateCancel{..}|Task::UpdateRelease{..}=>unreachable!("update replies are consumed above"),
                    Task::ModulesSnapshot { .. } | Task::ModulesSetEnabled { .. } | Task::ModulesRecover
                        | Task::ProfileExport { .. } | Task::ProfilePreview { .. } | Task::ProfileApply { .. } => unreachable!("management and profile replies are consumed above"),
                    Task::Logs { .. } | Task::LogFolder { .. } => unreachable!(),
                    Task::ProgramIcon { .. } | Task::ShortcutRecord | Task::ShortcutStop { .. } | Task::ShortcutStatus
                        | Task::ShortcutSnapshot | Task::ShortcutSave { .. } => unreachable!("icon and shortcut replies are consumed before this match"),
                    Task::Launch { .. } => { self.status = self.tr("程序已启动", "Program started"); }
                    Task::Bootstrap => {
                        self.snapshot = value["snapshot"].clone(); self.catalog = value["catalog"].clone();
                        self.pages = build_navigation(&self.catalog, self.snapshot["locale"].as_str().unwrap_or("zh-CN")).map_err(|e| e.to_string())?;
                        self.controls.clear(); self.palette = Some(Palette::new(self.window, self.theme()));
                        self.status.clear();
                        if self.navigation.page() == "overview" { self.load_management()?; }
                    }
                    Task::Load { page, .. } => {
                        self.module_data = value.clone();
                        if page == "settings" {
                            self.snapshot = value.clone();
                            self.pages = build_navigation(&self.catalog, self.snapshot["locale"].as_str().unwrap_or("zh-CN")).map_err(|e| e.to_string())?;
                            self.controls.clear(); self.palette = Some(Palette::new(self.window, self.theme()));
                        }
                        self.draft = Some(Draft::new(if page == "wintray" || page == "settings" { value["settings"].clone() } else { value }).map_err(|e| e.to_string())?); self.status.clear();
                        if page == "wintray" { self.request_program_icons(); }
                    }
                    Task::Save { page, .. } => {
                        let value = if page == "wintray" {
                            if value["state"] != "completed" || !value["result"]["settings"].is_object() || !value["result"]["revision"].is_string() {
                                if let Some(draft) = &mut self.draft { draft.save_failed(); }
                                return Err(self.tr("无法确认保存结果，请重新读取设置。", "Cannot confirm the save. Reload settings."));
                            }
                            self.module_data = value["result"].clone();
                            value["result"]["settings"].clone()
                        } else { value };
                        if let Some(draft) = &mut self.draft { draft.save_succeeded(value.clone()).map_err(|e| e.to_string())?; }
                        if page == "settings" {
                            self.snapshot["settings"] = value;
                            let locale = self.snapshot["settings"]["language"].as_str().unwrap_or("system");
                            if locale != "system" { self.snapshot["locale"] = json!(locale); }
                            self.pages = build_navigation(&self.catalog, self.snapshot["locale"].as_str().unwrap_or("zh-CN")).map_err(|e| e.to_string())?;
                            self.controls.clear(); self.palette = Some(Palette::new(self.window, self.theme()));
                        }
                        self.status = self.tr("已保存", "Saved");
                    }
                },
            }
            self.render()?;
        }
        Ok(())
    }
    fn layout(&mut self) {
        if self.program_viewport.is_some(){self.layout_program_workspace();return;}
        let Some(palette) = &self.palette else { return; };
        let mut client = RECT::default(); unsafe { GetClientRect(self.window, &mut client); }
        let visible = (client.bottom as f32 / palette.scale) as i32;
        self.scroll = self.scroll.clamp(0, (self.content_height - visible).max(0));
        let info = SCROLLINFO { cbSize: std::mem::size_of::<SCROLLINFO>() as u32, fMask: SIF_RANGE | SIF_PAGE | SIF_POS, nMin:0, nMax:self.content_height - 1, nPage:visible.max(1) as u32, nPos:self.scroll, nTrackPos:0 };
        unsafe { SetScrollInfo(self.window, SB_VERT, &info, 1); }
        unsafe{GetClientRect(self.window,&mut client);}
        for control in &self.controls { control.place(palette, client, self.scroll); }
    }
    fn resized(&mut self)->Result<(),String>{
        if self.shortcut_page()&&self.navigation.pending().is_none(){
            let tabs_top=self.reposition_shortcut()?+12;
            let offset=tabs_top+58-BODY_TOP;let delta=offset-self.current_body_offset;
            for control in &mut self.controls{
                if (TAB..SAVE).contains(&control.id){control.y=tabs_top;}
                else if control.body||control.id==STATUS{control.y+=delta;}
            }
            self.content_height+=delta;self.current_body_offset=offset;
        }
        self.layout();Ok(())
    }
}

impl Drop for App {
    fn drop(&mut self) {
        for icon in self.icons.values().flatten() { crate::icon::destroy(*icon); }
    }
}

unsafe extern "system" fn window_proc(window: HWND, message: u32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
    if message == WM_NCCREATE {
        let create = unsafe { &*(lparam as *const CREATESTRUCTW) };
        unsafe { SetWindowLongPtrW(window, GWLP_USERDATA, create.lpCreateParams as isize); }
    }
    let pointer = unsafe { GetWindowLongPtrW(window, GWLP_USERDATA) } as *const RefCell<App>;
    if message==COLOR_DIALOG_MESSAGE && !pointer.is_null(){
        let current={
            let Ok(mut app)=(unsafe{&*pointer}).try_borrow_mut() else{return 0;};
            app.color_request.take()
        };
        if let Some(current)=current{
            let selected=crate::dialogs::pick_color(window,&current);
            if let Ok(mut app)=(unsafe{&*pointer}).try_borrow_mut(){
                if let Err(error)=app.finish_frame_color(selected){app.status=error;app.refresh_buttons();}
            }
        }
        return 0;
    }
    // The owner-draw program list is created while App is borrowed for a render;
    // WM_MEASUREITEM must be answered without re-entering that borrow.
    if message == WM_MEASUREITEM && lparam != 0 {
        let item = unsafe { &mut *(lparam as *mut MEASUREITEMSTRUCT) };
        if item.CtlType == ODT_LISTBOX as u32 {
            let scale = unsafe { GetDpiForWindow(window) } as f32 / 96.0;
            item.itemHeight = (44.0 * scale).round() as u32;
            return 1;
        }
    }
    if !pointer.is_null() {
        // Creating, destroying and resizing native controls can reenter this procedure.
        // RefCell makes those notifications fall through instead of aliasing mutable App.
        if let Ok(mut app) = unsafe { &*pointer }.try_borrow_mut() {
            if app.palette.is_some() {
                let result = match message {
                    crate::permissions::GUARD_MESSAGE=>return if app.permission_guard(){1}else{0},
                    WM_COMMAND => app.command(wparam & 0xffff, (wparam >> 16) as u32, lparam as HWND),
                    worker::READY => app.replies(),
                    crate::lifecycle::WAKE => {
                        // Opening settings is a normal navigation request; a
                        // second launch never discards the active page's draft.
                        unsafe { ShowWindow(window, if IsIconic(window) != 0 { SW_RESTORE } else { SW_SHOW }); SetForegroundWindow(window); }
                        if wparam != 0 && app.navigation.page() != "settings" { app.navigate(Destination::Page("settings".into())) } else { Ok(()) }
                    }
                    WM_CLOSE => app.navigate(Destination::Close),
                    WM_SIZE => app.resized(),
                    crate::viewport::SCROLL|crate::viewport::WHEEL=>{app.program_scroll_message(message,wparam,lparam as HWND);Ok(())},
                    crate::viewport::FOCUS=>{app.reveal_program_control(lparam as HWND);Ok(())},
                    WM_TIMER if wparam == shortcut_view::SHORTCUT_TIMER => { app.recording_tick(); Ok(()) }
                    WM_TIMER if wparam == updates_view::UPDATE_TIMER=>{app.update_tick();Ok(())}
                    WM_MOUSEWHEEL => { if let Some(pane)=app.program_viewport.as_ref(){let pane=pane.window;app.program_scroll_message(crate::viewport::WHEEL,wparam,pane);}else{app.scroll -= ((wparam >> 16) as u16 as i16 as i32 / 120) * 72; app.layout();} Ok(()) }
                    WM_VSCROLL => {
                        match (wparam & 0xffff) as i32 { SB_LINEUP => app.scroll -= 36, SB_LINEDOWN => app.scroll += 36, SB_PAGEUP => app.scroll -= 300, SB_PAGEDOWN => app.scroll += 300,
                            SB_THUMBTRACK | SB_THUMBPOSITION => { let mut info = SCROLLINFO { cbSize:std::mem::size_of::<SCROLLINFO>() as u32, fMask:SIF_TRACKPOS, ..Default::default() }; unsafe { GetScrollInfo(window, SB_VERT, &mut info); } app.scroll = info.nTrackPos; }, _ => {} }
                        app.layout(); Ok(())
                    }
                    WM_LBUTTONDOWN if app.navigation.pending().is_some() => { app.navigation.cancel(); app.render() }
                    WM_DPICHANGED => {
                        let rect = unsafe { &*(lparam as *const RECT) };
                        unsafe { SetWindowPos(window, null_mut(), rect.left, rect.top, rect.right-rect.left, rect.bottom-rect.top, SWP_NOZORDER | SWP_NOACTIVATE); }
                        app.controls.clear(); app.palette = Some(Palette::new(window, app.theme())); app.render()
                    }
                    WM_SETTINGCHANGE | WM_THEMECHANGED => { app.controls.clear(); app.palette = Some(Palette::new(window, app.theme())); app.render() }
                    WM_ERASEBKGND | WM_PRINTCLIENT => {
                        let palette = app.palette.as_ref().unwrap(); let mut rect = RECT::default();
                        unsafe { GetClientRect(window, &mut rect); FillRect(wparam as HDC, &rect, palette.brush);
                            rect.right = palette.px(widgets::SIDEBAR_WIDTH);
                            let brush = CreateSolidBrush(palette.sidebar); FillRect(wparam as HDC, &rect, brush); DeleteObject(brush); }
                        return 1;
                    }
                    WM_CTLCOLORSTATIC | WM_CTLCOLOREDIT | WM_CTLCOLORLISTBOX | WM_CTLCOLORBTN => {
                        let palette = app.palette.as_ref().unwrap();
                        let sidebar=app.controls.iter().any(|c|c.window==lparam as HWND&&matches!(c.area,Area::Sidebar|Area::Bottom));
                        unsafe { SetTextColor(wparam as HDC, palette.foreground); SetBkColor(wparam as HDC, if sidebar{palette.sidebar}else{palette.background}); }
                        return if sidebar{palette.sidebar_brush}else{palette.brush} as isize;
                    }
                    WM_DRAWITEM if lparam != 0 => {
                        let item = unsafe { &*(lparam as *const DRAWITEMSTRUCT) };
                        if item.CtlType == ODT_LISTBOX as u32 {
                            let (icon, label) = app.program_list_item(item.itemID as usize);
                            program_view::draw_program_item(item, app.palette.as_ref().unwrap(), icon, &label);
                            return 1;
                        }
                        let id = item.CtlID as usize;
                        let selected = if (NAV..TAB).contains(&id) { app.pages.get(id - NAV).is_some_and(|p| p.id == app.navigation.page()) } else { (TAB..SAVE).contains(&id) && id - TAB == app.tab() };
                        let sidebar=app.controls.iter().any(|c|c.id==id&&matches!(c.area,Area::Sidebar|Area::Bottom));
                        widgets::draw_button(item, app.palette.as_ref().unwrap(), selected,sidebar);
                        if app.bindings.iter().any(|b|b.id==id && matches!(b.kind,FieldKind::Color)) {
                            if let Some(color)=app.field_value("frameColor").as_str().and_then(|v|crate::dialogs::color_ref(v).ok()) {
                                widgets::draw_color_swatch(item,app.palette.as_ref().unwrap(),color);
                            }
                        }
                        return 1;
                    }
                    WM_DESTROY => { unsafe { PostQuitMessage(0); } return 0; }
                    _ => {
                        // Default processing can synchronously request owner
                        // painting, colors and focus. Do not block those callbacks
                        // with the mutable App borrow from this outer message.
                        drop(app);
                        return unsafe { DefWindowProcW(window, message, wparam, lparam) };
                    },
                };
                if let Err(error) = result { app.loading = false; app.status = error; app.refresh_buttons(); }
                return 0;
            }
        } else if message==crate::permissions::GUARD_MESSAGE {return 2;}
        else if message==WM_CLOSE { return 0; } // A nested native dialog must not bypass navigation protection.
    }
    if message == WM_DESTROY { unsafe { PostQuitMessage(0); } return 0; }
    unsafe { DefWindowProcW(window, message, wparam, lparam) }
}

pub fn run(pipe: &str, data_directory: &str, open_settings: bool, plan: crate::startup::Plan) -> Result<(), String> {
    let Some(mut session) = crate::lifecycle::Instance::claim(pipe, open_settings)? else { return Ok(()); };
    unsafe { SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2); }
    let app = Box::new(RefCell::new(App::new()));
    let instance = unsafe { GetModuleHandleW(null()) };
    let class = WNDCLASSW { lpfnWndProc:Some(window_proc), hInstance:instance, lpszClassName:w!("WinOne.Desktop"), hCursor:unsafe { LoadCursorW(null_mut(), IDC_ARROW) }, ..Default::default() };
    if unsafe { RegisterClassW(&class) } == 0 { return Err(std::io::Error::last_os_error().to_string()); }
    let window = unsafe { CreateWindowExW(WS_EX_CONTROLPARENT, class.lpszClassName, w!("WinOne · 迁移预览"), WS_OVERLAPPEDWINDOW | WS_CLIPCHILDREN | WS_VSCROLL,
        CW_USEDEFAULT, CW_USEDEFAULT, 1100, 780, null_mut(), null_mut(), instance, (&*app as *const RefCell<App>).cast()) };
    if window.is_null() { return Err(std::io::Error::last_os_error().to_string()); }
    let initialize = (|| {
        let mut app = app.borrow_mut(); app.window = window; app.palette = Some(Palette::new(window, "system"));
        app.worker = Some(Worker::start(pipe, data_directory, window, plan).map_err(|e| e.to_string())?);
        session.attach(window)?;
        app.submit(Task::Bootstrap)?; app.render()
    })();
    if let Err(error) = initialize { unsafe { DestroyWindow(window); } return Err(error); }
    unsafe { ShowWindow(window, SW_SHOWNORMAL); }
    if open_settings { unsafe { PostMessageW(window, crate::lifecycle::WAKE, 1, 0); } }
    let mut message = MSG::default();
    loop {
        let result = unsafe { GetMessageW(&mut message, null_mut(), 0, 0) };
        if result == -1 { return Err(std::io::Error::last_os_error().to_string()); }
        if result == 0 { break; }
        // While a shortcut recording is active the recorder owns the keyboard:
        // swallow key messages here so they reach neither dialog navigation
        // nor the focused child control.
        let consumed = (WM_KEYDOWN..=WM_SYSDEADCHAR).contains(&message.message)
            && app.borrow_mut().record_key(message.message, message.wParam);
        if !consumed {
            if unsafe { IsDialogMessageW(window, &message) } == 0 { unsafe { TranslateMessage(&message); DispatchMessageW(&message); } }
        }
    }
    // Cancel and join I/O after the message loop, without holding a RefCell borrow.
    app.borrow_mut().worker.take();
    Ok(())
}
