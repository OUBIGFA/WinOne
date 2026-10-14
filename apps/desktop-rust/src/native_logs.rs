//! Diagnostics have their own request state: a failed settings read must never
//! hide logs, discard a draft, or change the selected tab.
use super::*;
use serde::Deserialize;

pub(super) const REFRESH_LOGS: usize = 320;
pub(super) const OPEN_LOG_FOLDER: usize = 321;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Logs { folder_available: bool, entries: Vec<LogEntry> }
#[derive(Deserialize)]
struct LogEntry { name: String, available: bool, truncated: bool, text: String }
#[derive(Default)]
pub(super) struct Diagnostics {
    page: String, generation: u64, data: Option<Logs>,
    loading: bool, opening: bool, error: String,
    selected_module: usize,
}

impl App {
    pub(super) fn showing_logs(&self) -> bool {
        let tab = match self.navigation.page() { "capture" | "settings" => 3, "always-on-top" | "wintray" => 2, _ => return false };
        self.tab() == tab
    }
    pub(super) fn reset_logs(&mut self) {
        self.logs = Diagnostics { generation: self.logs.generation + 1, ..Default::default() };
    }
    pub(super) fn load_logs(&mut self) {
        self.logs.generation += 1;
        self.logs.page = if self.navigation.page() == "settings" {
            ["host","capture","always-on-top","wintray"][self.logs.selected_module].into()
        } else { self.navigation.page().into() };
        self.logs.loading = true; self.logs.opening = false; self.logs.error.clear(); self.logs.data = None;
        if let Err(error) = self.submit(Task::Logs { page: self.logs.page.clone(), generation: self.logs.generation }) {
            self.logs.loading = false; self.logs.error = error;
        }
    }
    pub(super) fn logs_command(&mut self, id: usize) -> Result<(), String> {
        if !self.showing_logs() { return Ok(()); }
        if (330..=333).contains(&id) && self.navigation.page() == "settings" {
            self.logs.selected_module = id-330; self.load_logs();
        }
        if id == REFRESH_LOGS && !self.logs.loading && !self.logs.opening { self.load_logs(); }
        if id == OPEN_LOG_FOLDER && !self.logs.opening && self.logs.data.as_ref().is_some_and(|data| data.folder_available) {
            self.logs.opening = true; self.logs.error.clear();
            if let Err(error) = self.submit(Task::LogFolder { page: self.logs.page.clone(), generation: self.logs.generation }) {
                self.logs.opening = false; self.logs.error = error;
            }
        }
        self.render()
    }
    pub(super) fn receive_logs(&mut self, reply: &worker::Reply) -> Result<bool, String> {
        let (page, generation) = match &reply.task {
            Task::Logs { page, generation } | Task::LogFolder { page, generation } => (page, generation),
            _ => return Ok(false),
        };
        if !self.showing_logs() || page != &self.logs.page || *generation != self.logs.generation { return Ok(true); }
        match &reply.task {
            Task::Logs { .. } => {
                self.logs.loading = false;
                match &reply.result {
                    Ok(value) => match serde_json::from_value::<Logs>(value.clone()) {
                        Ok(logs) => self.logs.data = Some(logs),
                        Err(_) => self.logs.error = self.tr("无法读取日志结果，请刷新重试。", "Cannot read the log response. Refresh to retry."),
                    },
                    Err(error) => self.logs.error = error.to_string(),
                }
            }
            Task::LogFolder { .. } => {
                self.logs.opening = false;
                let result = reply.result.as_ref().map_err(|error| error.to_string())
                    .and_then(|value| value["path"].as_str().ok_or_else(|| self.tr("日志文件夹不可用。", "The log folder is unavailable.")))
                    .and_then(|path| crate::dialogs::open_folder(self.window, path));
                if let Err(error) = result { self.logs.error = error; }
            }
            _ => unreachable!(),
        }
        self.render()?;
        Ok(true)
    }
    pub(super) fn render_logs(&mut self) -> Result<(), String> {
        let offset = if self.navigation.page() == "settings" {
            let labels = [self.tr("WinOne", "WinOne"),self.tr("截图", "Capture"),self.tr("置顶功能", "Always on Top"),self.tr("程序管理", "Programs")];
            for (index,label) in labels.iter().enumerate() { self.button(label, 330+index, Area::Tabs {index,count:labels.len()}, [0,194,0,36])?; }
            50
        } else { 0 };
        let refresh = self.button(&self.tr("刷新", "Refresh"), REFRESH_LOGS, Area::Content, [0,194+offset,112,36])?;
        let open = self.button(&self.tr("打开日志文件夹", "Open log folder"), OPEN_LOG_FOLDER, Area::Content, [128,194+offset,200,36])?;
        widgets::enable(refresh, !self.logs.loading && !self.logs.opening);
        widgets::enable(open, !self.logs.opening && self.logs.data.as_ref().is_some_and(|data| data.folder_available));
        let mut y = 246+offset;
        if self.logs.loading { self.label(&self.tr("正在读取日志…", "Loading logs…"), y, false)?; y += 48; }
        if !self.logs.error.is_empty() { self.label(&self.logs.error.clone(), y, false)?; y += 48; }
        // Copy just the display strings before creating controls, not another
        // editable draft. Native edit controls provide selection and copying.
        let entries = self.logs.data.as_ref().map(|logs| logs.entries.iter().map(|entry|
            (entry.name.clone(), entry.available, entry.truncated, entry.text.clone())).collect::<Vec<_>>());
        if let Some(entries) = entries {
            if entries.is_empty() { self.label(&self.tr("暂无日志", "No logs yet"), y, false)?; y += 48; }
            for (name, available, truncated, text) in entries {
                self.label(&if name == "activity" { self.tr("运行日志", "Activity log") } else { self.tr("进程日志", "Process log") }, y, false)?; y += 40;
                if truncated { self.label(&self.tr("仅显示最近的日志", "Showing the most recent log entries"), y, false)?; y += 40; }
                if !available || text.is_empty() { self.label(&self.tr("暂无日志", "No logs yet"), y, false)?; y += 48; }
                else {
                    let display = text.replace("\r\n", "\n").replace('\n', "\r\n").replace('\0', "␀");
                    if self.rich_text.is_none() { self.rich_text = Some(widgets::RichEdit::load()?); }
                    let viewer = self.add("RICHEDIT50W", "", 0, WS_TABSTOP | WS_VSCROLL | ES_MULTILINE as u32 | ES_READONLY as u32 | ES_AUTOVSCROLL as u32,
                        Area::Content, [0,y,0,230], false)?;
                    self.rich_text.as_ref().unwrap().configure_log(viewer, self.palette.as_ref().unwrap())?;
                    widgets::set_text(viewer, &display); y += 250;
                }
            }
        }
        self.content_height = (y + 90).max(440);
        Ok(())
    }
}
