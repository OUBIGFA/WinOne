//! UI state is independent of HWNDs so async completion cannot discard newer edits.
use crate::protocol::RpcError;
use serde_json::Value;
use std::collections::HashSet;

pub struct Draft { saved: Value, edited: Value, saving: bool }
impl Draft {
    pub fn new(value: Value) -> Result<Self, RpcError> {
        if !value.is_object() { return Err(RpcError::new("invalid", "设置格式无效")); }
        Ok(Self { saved: value.clone(), edited: value, saving: false })
    }
    pub fn value(&self) -> &Value { &self.edited }
    pub fn is_dirty(&self) -> bool { self.edited != self.saved }
    pub fn is_saving(&self) -> bool { self.saving }
    pub fn set(&mut self, key: &str, value: Value) -> Result<(), RpcError> {
        if self.saving { return Err(RpcError::new("busy", "正在保存，请稍候")); }
        if matches!(key, "revision" | "schemaVersion") {
            return Err(RpcError::new("invalid", "不能修改设置版本"));
        }
        self.edited[key] = value;
        Ok(())
    }
    pub fn begin_save(&mut self) -> Result<Value, RpcError> {
        if self.saving { return Err(RpcError::new("busy", "正在保存，请稍候")); }
        self.saving = true;
        // Preserve unknown fields and the exact revision supplied by the Host.
        Ok(self.edited.clone())
    }
    pub fn save_failed(&mut self) { self.saving = false; }
    pub fn save_succeeded(&mut self, value: Value) -> Result<(), RpcError> {
        if !self.saving || !value.is_object() {
            return Err(RpcError::new("protocol", "无法确认保存结果"));
        }
        self.saved = value.clone(); self.edited = value; self.saving = false;
        Ok(())
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Destination { Page(String), Close }
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum NavigationResult { Navigate, Confirm, Busy }
pub struct Navigation { page: String, pending: Option<Destination> }
impl Navigation {
    pub fn new(page: &str) -> Self { Self { page: page.into(), pending: None } }
    pub fn page(&self) -> &str { &self.page }
    pub fn pending(&self) -> Option<&Destination> { self.pending.as_ref() }
    pub fn request(&mut self, destination: Destination, dirty: bool, busy: bool) -> NavigationResult {
        self.pending = None;
        if busy { return NavigationResult::Busy; }
        if dirty { self.pending = Some(destination); return NavigationResult::Confirm; }
        if let Destination::Page(page) = destination { self.page = page; }
        NavigationResult::Navigate
    }
    pub fn cancel(&mut self) { self.pending = None; }
    pub fn discard(&mut self, busy: bool) -> Option<Destination> {
        if busy { return None; }
        let destination = self.pending.take()?;
        if let Destination::Page(page) = &destination { self.page = page.clone(); }
        Some(destination)
    }
}

#[derive(Debug, Clone)]
pub struct Page { pub id: String, pub title: String }
pub fn build_navigation(catalog: &Value, locale: &str) -> Result<Vec<Page>, RpcError> {
    let english = locale == "en-US";
    let manifests = catalog.as_array().ok_or_else(|| RpcError::new("protocol", "模块目录格式无效"))?;
    let mut seen = HashSet::from(["overview", "settings"]);
    let mut modules = Vec::new();
    for manifest in manifests {
        let id = manifest["id"].as_str().ok_or_else(|| RpcError::new("protocol", "模块缺少标识"))?;
        if id.is_empty() || !id.bytes().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == b'.' || c == b'-') || !seen.insert(id) {
            return Err(RpcError::new("protocol", "模块标识无效或重复"));
        }
        let title = manifest["name"][if english { "en-US" } else { "zh-CN" }].as_str()
            .filter(|s| !s.trim().is_empty()).ok_or_else(|| RpcError::new("protocol", "模块缺少名称"))?;
        modules.push(Page { id: id.into(), title: title.into() });
    }
    modules.sort_by_key(|page| match page.id.as_str() { "capture" => 0, "always-on-top" => 1, "wintray" => 2, _ => 3 });
    let mut pages = vec![Page { id: "overview".into(), title: if english { "Home" } else { "首页" }.into() }];
    pages.extend(modules);
    pages.push(Page { id: "settings".into(), title: if english { "Settings" } else { "设置" }.into() });
    Ok(pages)
}
