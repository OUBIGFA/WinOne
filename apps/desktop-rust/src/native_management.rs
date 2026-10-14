//! Home owns module lifecycle actions. Every change is confirmed by the Host;
//! refreshing afterwards reconciles failures as well as successful commits.
use super::*;

pub(super) const MODULE_TOGGLE: usize = 1000;
pub(super) const MODULE_RETRY: usize = 2000;
pub(super) const RECOVER: usize = 390;

#[derive(Default)]
pub(super) struct Management {
    pub(super) snapshot: Value,
    pub(super) busy: bool,
}

impl App {
    pub(super) fn load_management(&mut self) -> Result<(), String> {
        self.submit(Task::ModulesSnapshot { generation: self.generation })?;
        self.loading = true;
        Ok(())
    }

    pub(super) fn render_management(&mut self) -> Result<(), String> {
        let snapshot = self.management.snapshot.clone();
        let mut y = 92;
        if let Some(warning)=self.snapshot["runtimeWarning"].as_str().filter(|s|!s.is_empty()).map(str::to_owned) {
            self.label(&warning,y,false)?; y+=50;
        }
        if let Some(error) = snapshot["configurationError"].as_str().filter(|s| !s.is_empty()) {
            self.label(error, y, false)?;
            self.button(&self.tr("重试恢复配置", "Retry configuration recovery"), RECOVER, Area::Content, [0,y+44,230,36])?;
            y += 108;
        }
        self.label(&self.tr("管理工具", "Manage tools"), y, true)?;
        y += 48;
        let modules = snapshot["modules"].as_array().cloned().unwrap_or_default();
        for (index, entry) in modules.iter().enumerate() {
            let id = entry["manifest"]["id"].as_str().unwrap_or("");
            let title = self.module_title(id);
            let pending = entry["pendingStop"] == true;
            let state = if pending { self.tr("关闭未完成", "Disable incomplete") }
                else { match entry["runtime"]["state"].as_str().unwrap_or("") {
                    "faulted" => self.tr("需要处理", "Needs attention"),
                    "unavailable" => self.tr("不可用", "Unavailable"),
                    "starting" | "stopping" => self.tr("正在处理", "Working"),
                    _ if entry["enabled"] == true => self.tr("已开启", "Enabled"),
                    _ => self.tr("已关闭", "Disabled"),
                }};
            self.label(&format!("{title}    {state}"), y, false)?;
            let toggle = self.button(&self.tr(if entry["enabled"] == true { "关闭" } else { "开启" },
                if entry["enabled"] == true { "Disable" } else { "Enable" }), MODULE_TOGGLE+index, Area::Content, [0,y+38,110,36])?;
            widgets::enable(toggle, self.management_mutable() && !pending);
            if pending && snapshot["configurationError"].as_str().unwrap_or("").is_empty() {
                let retry = self.button(&self.tr("重试关闭", "Retry disabling"), MODULE_RETRY+index, Area::Content, [126,y+38,150,36])?;
                widgets::enable(retry, self.management_mutable());
            }
            if let Some(detail) = entry["runtime"]["detail"].as_str().filter(|s| !s.is_empty()) {
                self.label(detail, y+82, false)?;
                y += 40;
            }
            y += 96;
        }
        if modules.is_empty() { self.label(&self.tr(if self.loading { "正在读取模块…" } else { "暂无模块" }, if self.loading { "Loading modules…" } else { "No modules" }), y, false)?; y += 50; }
        self.button(&self.tr("刷新", "Refresh"), RELOAD, Area::Content, [0,y+8,112,36])?;
        self.content_height = (y+136).max(440);
        Ok(())
    }

    pub(super) fn module_title(&self, id: &str) -> String {
        self.pages.iter().find(|p| p.id == id).map(|p| p.title.clone()).unwrap_or_else(|| id.into())
    }

    fn management_mutable(&self) -> bool {
        !self.management.busy && !self.loading && self.management.snapshot["revision"].is_u64()
            && self.management.snapshot["configurationError"].as_str().unwrap_or("").is_empty()
    }

    pub(super) fn management_command(&mut self, id: usize) -> Result<bool, String> {
        if self.navigation.page() != "overview" { return Ok(false); }
        let task = if id == RECOVER {
            if self.management.busy || self.loading { return Ok(true); }
            Task::ModulesRecover
        } else if (MODULE_TOGGLE..MODULE_RETRY+1000).contains(&id) {
            if !self.management_mutable() { return Ok(true); }
            let retry = id >= MODULE_RETRY;
            let index = id - if retry { MODULE_RETRY } else { MODULE_TOGGLE };
            let Some(entry) = self.management.snapshot["modules"].as_array().and_then(|m| m.get(index)) else { return Ok(true); };
            if !retry && entry["pendingStop"] == true { return Ok(true); }
            Task::ModulesSetEnabled { id: entry["manifest"]["id"].as_str().ok_or("模块标识无效")?.into(),
                enabled: !retry && entry["enabled"] != true, revision: self.management.snapshot["revision"].as_u64().ok_or("模块版本无效")? }
        } else { return Ok(false); };
        self.submit(task)?;
        self.management.busy = true;
        self.status = self.tr("正在更新模块…", "Updating modules…");
        self.render()?;
        Ok(true)
    }

    pub(super) fn receive_management(&mut self, reply: &worker::Reply) -> Result<bool, String> {
        match &reply.task {
            Task::ModulesSnapshot { generation } => {
                if *generation != self.generation || self.navigation.page() != "overview" { return Ok(true); }
                self.loading = false;
                match &reply.result {
                    Ok(value) if value["revision"].is_u64() && value["modules"].is_array() => {
                        self.management.snapshot = value.clone();
                        // Module pages consult this runtime snapshot when gating actions.
                        self.snapshot["modules"] = json!(value["modules"].as_array().unwrap().iter().map(|e| e["runtime"].clone()).collect::<Vec<_>>());
                    }
                    Ok(_) => { self.management.snapshot = Value::Null; self.status = self.tr("模块状态无效，请重试。", "Invalid module status. Retry."); }
                    Err(error) => { self.management.snapshot = Value::Null; self.status = error.to_string(); }
                }
            }
            Task::ModulesSetEnabled { .. } | Task::ModulesRecover => {
                self.management.busy = false;
                self.status = match &reply.result {
                    Ok(value) if value["state"] == "completed" => self.tr("模块状态已更新", "Module state updated"),
                    Ok(_) => self.tr("无法确认模块状态，请重新读取。", "Cannot confirm module state. Refresh it."),
                    Err(error) => error.to_string(),
                };
                if let Err(error) = self.load_management() { self.status.push_str(&format!("\n{error}")); }
            }
            _ => return Ok(false),
        }
        self.render()?;
        Ok(true)
    }
}
