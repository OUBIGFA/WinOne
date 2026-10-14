//! Configuration transfer remains a reviewed transaction: choosing and mapping
//! files only builds a preview; only Apply can commit the reviewed revisions.
use super::*;
use std::collections::HashSet;

const EXPORT: usize = 380;
const IMPORT: usize = 381;
const REVIEW: usize = 382;
const APPLY: usize = 383;
const CANCEL: usize = 384;
const GLOBAL: usize = 385;
const MODULE: usize = 4000;
const PATH: usize = 6000;

#[derive(Default)]
pub(super) struct ProfileState {
    pub(super) preview: Value,
    pub(super) busy: bool,
    source: String,
    bindings: Value,
    selected: HashSet<String>,
    global: bool,
    needs_review: bool,
}

impl App {
    pub(super) fn render_profiles(&mut self) -> Result<(), String> {
        self.label(&self.tr("导出可迁移设置；导入前核对设置和本机路径。", "Export portable settings; review settings and local paths before importing."), 192, false)?;
        self.button(&self.tr("导出配置", "Export configuration"), EXPORT, Area::Content, [0,240,160,36])?;
        self.button(&self.tr("导入配置", "Import configuration"), IMPORT, Area::Content, [176,240,160,36])?;
        let preview = self.profiles.preview.clone();
        if !preview.is_object() { self.content_height = 430; return Ok(()); }
        self.label(preview["name"].as_str().unwrap_or(""), 300, true)?;
        let mut y = 346;
        self.profile_check(GLOBAL, &self.tr("全局设置（含登录启动）", "Global settings (including sign-in startup)"), self.profiles.global, y)?;
        y += 42;
        if self.profiles.global {
            let prefs = &preview["preferences"];
            let enabled = self.tr("开启", "On"); let disabled = self.tr("关闭", "Off");
            let on_off = |key: &str| if prefs[key] == true { &enabled } else { &disabled };
            let theme = match prefs["theme"].as_str().unwrap_or("system") { "dark" => self.tr("深色", "Dark"), "light" => self.tr("浅色", "Light"), _ => self.tr("跟随系统", "System") };
            let language = match prefs["language"].as_str().unwrap_or("system") { "zh-CN" => "简体中文".into(), "en-US" => "English".into(), _ => self.tr("跟随系统", "System") };
            self.label(&format!("{}: {theme}    {}: {language}", self.tr("主题", "Theme"), self.tr("语言", "Language")), y, false)?;
            y += 40;
            self.label(&format!("{}: {}    {}: {}", self.tr("登录启动", "Run at sign-in"), on_off("runAtLogon"), self.tr("静默启动", "Start minimized"), on_off("startMinimized")), y, false)?;
            y += 44;
        }
        let modules = preview["plan"]["modules"].as_array().cloned().unwrap_or_default();
        for (index, module) in modules.iter().enumerate() {
            let id = module["id"].as_str().unwrap_or("");
            let selected = self.profiles.selected.contains(id);
            self.profile_check(MODULE+index, &self.module_title(id), selected, y)?;
            y += 38;
            let state = match module["state"].as_str().unwrap_or("") {
                "ready" => self.tr("可导入", "Ready to import"),
                "missing_module" => self.tr("未安装；保留设置供以后使用", "Not installed; keep settings for later"),
                "needs_mapping" => self.tr("需要重新选择位置", "Choose local locations"),
                _ => module["error"].as_str().unwrap_or("Invalid configuration").into(),
            };
            self.label(&state, y, false)?; y += 40;
            if selected {
                if id == "wintray" {
                    if let Some(items) = module["settings"]["managedApps"].as_array() {
                        self.label(&self.tr("将替换当前程序列表（{count} 个程序）。", "This replaces the current program list ({count} programs).").replace("{count}", &items.len().to_string()), y, false)?;
                        y += 40;
                    }
                }
                if let Some(enabled) = preview["preferences"]["enabled"][id].as_bool() {
                    self.label(&self.tr(if enabled { "导入后开启" } else { "导入后关闭" }, if enabled { "Enabled after import" } else { "Disabled after import" }), y, false)?;
                    y += 40;
                }
                if let Some(shortcuts) = preview["preferences"]["shortcuts"].as_object() {
                    for (action, value) in shortcuts {
                        if action.starts_with(&format!("{id}.")) {
                            self.label(&format!("{}: {}", self.tr("快捷键", "Shortcut"), value.as_str().unwrap_or("")), y, false)?;
                            y += 40;
                        }
                    }
                }
                // The host exposes only validated built-in settings here;
                // unknown module payloads remain opaque and are kept for later.
                if module["settings"].is_object() {
                    let text = profile_settings_text(&module["settings"], &preview["paths"], self.tr("zh", "en").as_str());
                    if self.rich_text.is_none() { self.rich_text = Some(widgets::RichEdit::load()?); }
                    let field = self.add("RICHEDIT50W", "", 0, WS_TABSTOP | WS_BORDER | ES_MULTILINE as u32 | ES_READONLY as u32 | ES_AUTOVSCROLL as u32 | WS_VSCROLL,
                        Area::Content, [0,y,0,180], false)?;
                    self.rich_text.as_ref().unwrap().configure_log(field, self.palette.as_ref().unwrap())?;
                    widgets::set_text(field, &text);
                    y += 194;
                }
            }
        }
        let paths = preview["paths"].as_array().cloned().unwrap_or_default();
        for (index, path) in paths.iter().enumerate() {
            let needed = modules.iter().any(|m| self.profiles.selected.contains(m["id"].as_str().unwrap_or(""))
                && m["requiredPaths"].as_array().is_some_and(|ids| ids.contains(&path["id"])));
            if !needed { continue; }
            self.label(path["label"].as_str().unwrap_or(""), y, false)?; y += 38;
            let label = path["value"].as_str().filter(|s| !s.is_empty()).map(str::to_string).unwrap_or_else(|| self.tr("尚未选择位置", "No location selected"));
            self.label(&label, y, false)?; y += 40;
            self.button(&self.tr("选择位置", "Choose location"), PATH+index, Area::Content, [0,y,180,36])?;
            y += 44;
            if let Some(error) = path["error"].as_str() { self.label(error, y, false)?; y += 40; }
        }
        for issue in self.profile_issues() { self.label(&issue, y, false)?; y += 42; }
        self.button(&self.tr("确认导入", "Apply import"), APPLY, Area::Content, [0,y+8,150,36])?;
        self.button(&self.tr("重新预览", "Review again"), REVIEW, Area::Content, [166,y+8,150,36])?;
        self.button(&self.tr("取消导入", "Cancel import"), CANCEL, Area::Content, [332,y+8,150,36])?;
        self.content_height = y+142;
        Ok(())
    }

    fn profile_check(&mut self, id: usize, label: &str, selected: bool, y: i32) -> Result<(), String> {
        let window = self.add("BUTTON", label, id, WS_TABSTOP | BS_AUTOCHECKBOX as u32, Area::Content, [0,y,0,34], false)?;
        unsafe { SendMessageW(window, BM_SETCHECK, if selected { BST_CHECKED } else { BST_UNCHECKED } as usize, 0); }
        Ok(())
    }

    fn profile_issues(&self) -> Vec<String> {
        let mut issues = Vec::new();
        if self.profiles.needs_review { issues.push(self.tr("请重新预览后再导入。", "Review again before importing.")); }
        if !self.profiles.global && self.profiles.selected.is_empty() { issues.push(self.tr("请选择要导入的设置。", "Select settings to import.")); }
        if let Some(modules) = self.profiles.preview["plan"]["modules"].as_array() {
            for module in modules {
                if self.profiles.selected.contains(module["id"].as_str().unwrap_or("")) && !matches!(module["state"].as_str(), Some("ready" | "missing_module")) {
                    issues.push(format!("{}: {}", self.module_title(module["id"].as_str().unwrap_or("")), self.tr("请先处理设置或路径问题。", "Resolve configuration or path issues first.")));
                }
            }
        }
        // Evaluate the effective combination, including unchanged shortcuts of
        // unselected modules. The Host validates the same combination on apply.
        let mut shortcuts = self.profiles.preview["currentShortcuts"].as_object().cloned().unwrap_or_default();
        if let Some(incoming) = self.profiles.preview["preferences"]["shortcuts"].as_object() {
            for (action, value) in incoming {
                if self.profiles.selected.iter().any(|id| action.starts_with(&format!("{id}."))) { shortcuts.insert(action.clone(), value.clone()); }
            }
        }
        let mut keys = HashSet::new();
        for value in shortcuts.values().filter_map(Value::as_str).filter(|s| !s.is_empty()) {
            if !keys.insert(value.to_lowercase()) { issues.push(self.tr("快捷键冲突，请取消选择冲突模块或修改源配置。", "Shortcut conflict. Deselect a conflicting module or edit the source profile.")); break; }
        }
        if let Some(problems) = self.profiles.preview["plan"]["shortcutIssues"].as_array() {
            for problem in problems {
                if problem["kind"] != "conflict" && self.profiles.selected.iter().any(|id| problem["action"].as_str().unwrap_or("").starts_with(&format!("{id}."))) {
                    if let Some(error) = problem["error"].as_str() { issues.push(error.into()); }
                }
            }
        }
        issues
    }

    pub(super) fn refresh_profile_buttons(&self) {
        for control in &self.controls {
            if matches!(control.id, EXPORT | IMPORT | REVIEW | APPLY | CANCEL | GLOBAL) || control.id >= MODULE {
                let ready = !self.saving() && !self.dirty() && !self.loading;
                widgets::enable(control.window, ready && (control.id != APPLY || self.profile_issues().is_empty()));
            }
        }
    }

    fn request_profile_preview(&mut self, path: String, bindings: Value) -> Result<(), String> {
        // Mapping changes invalidate the review even if the request queue is
        // full; never apply new paths using an older displayed preview.
        self.profiles.needs_review = true;
        self.submit(Task::ProfilePreview { path, bindings })?;
        self.profiles.busy = true;
        self.status = self.tr("正在预览配置…", "Reviewing configuration…");
        self.refresh_buttons();
        Ok(())
    }

    pub(super) fn profile_command(&mut self, id: usize, notification: u32) -> Result<bool, String> {
        if self.navigation.page() != "settings" || !(matches!(id, EXPORT | IMPORT | REVIEW | APPLY | CANCEL | GLOBAL) || id >= MODULE) { return Ok(false); }
        if notification != BN_CLICKED || self.saving() || self.dirty() || self.loading { return Ok(true); }
        if id == CANCEL { self.profiles = Default::default(); self.status.clear(); self.render()?; return Ok(true); }
        if id == GLOBAL { self.profiles.global = !self.profiles.global; self.render()?; return Ok(true); }
        if (MODULE..PATH).contains(&id) {
            if let Some(module) = self.profiles.preview["plan"]["modules"].as_array().and_then(|items| items.get(id-MODULE)) {
                let key = module["id"].as_str().unwrap_or("").to_string();
                if !self.profiles.selected.remove(&key) { self.profiles.selected.insert(key); }
            }
            self.render()?; return Ok(true);
        }
        match id {
            REVIEW => self.request_profile_preview(self.profiles.source.clone(), self.profiles.bindings.clone())?,
            APPLY => {
                if !self.profiles.preview.is_object() || !self.profile_issues().is_empty() { return Ok(true); }
                let preview = &self.profiles.preview;
                self.submit(Task::ProfileApply { params: json!({"path":self.profiles.source,"bindings":self.profiles.bindings,
                    "sourceHash":preview["sourceHash"],"configurationRevision":preview["configurationRevision"],
                    "moduleRevision":preview["moduleRevision"],"global":self.profiles.global,"modules":self.profiles.selected}) })?;
                self.profiles.busy = true;
                self.status = self.tr("正在导入配置…", "Importing configuration…");
                self.refresh_buttons();
            }
            _ => {
                // Native dialogs run a nested message loop. Mark the operation
                // busy before entering it so close/navigation cannot discard it.
                self.profiles.busy = true; self.refresh_buttons();
                let choice = match id {
                    EXPORT => crate::dialogs::save_profile(self.window, &self.tr("导出配置", "Export configuration")),
                    IMPORT => crate::dialogs::pick_profile(self.window, &self.tr("导入配置", "Import configuration")),
                    _ => {
                        let path = self.profiles.preview["paths"].as_array().and_then(|items| items.get(id.saturating_sub(PATH)));
                        match path.and_then(|p| p["kind"].as_str()) {
                            Some("program") => crate::dialogs::pick_programs(self.window, &self.tr("选择程序", "Choose program"), false).map(|v| v.into_iter().next()),
                            Some("file") => crate::dialogs::pick_file(self.window, &self.tr("选择文件", "Choose file")),
                            Some("directory") => crate::dialogs::pick_folder(self.window, &self.tr("选择文件夹", "Choose folder")),
                            _ => Err(self.tr("位置类型无效。", "Invalid location type.")),
                        }
                    }
                };
                self.profiles.busy = false; self.refresh_buttons();
                if let Some(value) = choice? {
                    if id == EXPORT {
                        self.submit(Task::ProfileExport { path: value, name: "WinOne".into() })?;
                        self.profiles.busy = true; self.status = self.tr("正在导出配置…", "Exporting configuration…"); self.refresh_buttons();
                    } else if id == IMPORT { self.request_profile_preview(value, json!({}))?; }
                    else {
                        let key = self.profiles.preview["paths"][id-PATH]["id"].as_str().ok_or("位置标识无效")?.to_string();
                        self.profiles.bindings[&key] = json!(value);
                        self.request_profile_preview(self.profiles.source.clone(), self.profiles.bindings.clone())?;
                    }
                }
            }
        }
        Ok(true)
    }

    pub(super) fn receive_profile(&mut self, reply: &worker::Reply) -> Result<bool, String> {
        if !matches!(reply.task, Task::ProfileExport { .. } | Task::ProfilePreview { .. } | Task::ProfileApply { .. }) { return Ok(false); }
        self.profiles.busy = false;
        match (&reply.task, &reply.result) {
            (_, Err(error)) => { self.status = error.to_string(); if !matches!(reply.task, Task::ProfileExport { .. }) { self.profiles.needs_review = true; } }
            (Task::ProfileExport { path, .. }, Ok(value)) if value["path"].as_str() == Some(path) => self.status = self.tr("已导出：{path}", "Exported: {path}").replace("{path}", path),
            (Task::ProfilePreview { path, bindings }, Ok(value)) if value["sourceHash"].as_str().is_some_and(|v| v.len()==64)
                && value["configurationRevision"].is_u64() && value["moduleRevision"].is_u64()
                && value["plan"]["modules"].as_array().is_some_and(|m| m.len()<2000) && value["paths"].is_array() && value["preferences"].is_object() => {
                let changed = self.profiles.source != *path;
                let ids: HashSet<String> = value["plan"]["modules"].as_array().unwrap().iter().filter_map(|m| m["id"].as_str().map(str::to_owned)).collect();
                if changed || !self.profiles.preview.is_object() {
                    self.profiles.selected = value["plan"]["modules"].as_array().unwrap().iter().filter(|m| m["state"] != "invalid").filter_map(|m| m["id"].as_str().map(str::to_owned)).collect();
                    self.profiles.global = true;
                } else { self.profiles.selected.retain(|id| ids.contains(id)); }
                self.profiles.source = path.clone(); self.profiles.bindings = bindings.clone(); self.profiles.preview = value.clone(); self.profiles.needs_review = false; self.status.clear();
            }
            (Task::ProfileApply { .. }, Ok(value)) if value["revision"].is_u64() && value["modules"].is_array() => {
                self.profiles = Default::default();
                self.submit(Task::Load { page: "settings".into(), generation: self.generation })?;
                self.loading = true;
                self.status = self.tr("已导入配置，正在刷新设置。", "Configuration imported. Refreshing settings.");
            }
            _ => { self.profiles.needs_review = true; self.status = self.tr("无法确认配置操作结果，请重新预览。", "Cannot confirm the result. Review again."); }
        }
        self.render()?;
        Ok(true)
    }
}

/// Show nested settings as ordinary labels and values, not executable markup
/// or serialized JSON. Labels match the existing configuration review page.
fn profile_settings_text(settings: &Value, paths: &Value, locale: &str) -> String {
    static LABELS: std::sync::OnceLock<Value> = std::sync::OnceLock::new();
    let labels = &LABELS.get_or_init(|| serde_json::from_str(include_str!("profile_labels.json")).expect("bundled profile labels"))[locale];
    let mut lines = Vec::new();
    fn append(value: &Value, key: &str, prefix: &str, labels: &Value, paths: &Value, english: bool, lines: &mut Vec<String>) {
        if key == "id" { return; }
        let label = labels[key].as_str().unwrap_or(key);
        let heading = if prefix.is_empty() { label.into() } else if label.is_empty() { prefix.into() } else { format!("{prefix} / {label}") };
        match value {
            Value::Object(items) => for (key, value) in items { append(value, key, &heading, labels, paths, english, lines); },
            Value::Array(items) if !items.is_empty() => for (index, value) in items.iter().enumerate() { append(value, "", &format!("{heading} {}", index+1), labels, paths, english, lines); },
            _ => {
                let text = if matches!(key, "programRef" | "pathRef" | "outputDirectoryRef") {
                    paths.as_array().and_then(|items| items.iter().find(|p| p["id"] == *value)).and_then(|p| p["value"].as_str()).filter(|s| !s.is_empty())
                        .unwrap_or(if english { "Choose a local location" } else { "待选择本机位置" }).to_string()
                } else if key == "openEditor" && value.is_boolean() {
                    if value == true { if english { "Edit after capture" } else { "截图后编辑" } } else { if english { "Save after capture" } else { "截完保存" } }.into()
                } else { match value {
                    Value::Bool(true) => if english { "On" } else { "开启" }.into(),
                    Value::Bool(false) => if english { "Off" } else { "关闭" }.into(),
                    Value::String(s) if !s.is_empty() => s.clone(),
                    Value::Number(n) => n.to_string(),
                    _ => if english { "Not set" } else { "未设置" }.into(),
                }};
                lines.push(format!("{heading}: {text}"));
            }
        }
    }
    append(settings, "", "", labels, paths, locale == "en", &mut lines);
    lines.join("\r\n")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_failed_preview_submission_invalidates_the_previous_review() {
        let mut app = App::new();
        app.profiles.preview = json!({"name":"existing"});
        assert!(app.request_profile_preview("existing.winone".into(), json!({"capture.output":"new location"})).is_err());
        assert!(app.profiles.needs_review, "changed mappings must be reviewed even when submission fails");
        assert_eq!(app.profiles.preview["name"], "existing");
    }

    #[test]
    fn review_text_shows_labels_local_paths_and_program_settings_in_both_languages() {
        let value = json!({"openEditor":false,"managedApps":[{"id":"private-id","name":"Editor","schedule":{"autoExitMinutes":0},"programRef":"app.1"}]});
        let paths = json!([{"id":"app.1","value":"C:\\Apps\\Editor.exe"}]);
        let chinese = profile_settings_text(&value, &paths, "zh");
        let english = profile_settings_text(&value, &paths, "en");
        assert!(chinese.contains("截完保存") && chinese.contains("名称: Editor") && chinese.contains("程序位置: C:\\Apps\\Editor.exe"));
        assert!(english.contains("Save after capture") && english.contains("Name: Editor"));
        assert!(!chinese.contains("private-id") && !english.contains("autoExitMinutes"));
    }
}
