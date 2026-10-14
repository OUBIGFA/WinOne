//! Program fields are addressed by stable ID so list changes cannot redirect edits.
use crate::protocol::RpcError;
use serde_json::{Value, json};

fn path_name(path: &str) -> Result<String, RpcError> {
    let name = path.rsplit(['\\','/']).next().unwrap_or("");
    if name.is_empty() || path.contains('\0') { return Err(RpcError::new("invalid", "请选择程序文件")); }
    Ok(name.rsplit_once('.').map(|(stem,_)| stem).unwrap_or(name).into())
}

/// Defaults match the existing program manager. Adding only edits a draft;
/// programs are not registered or started until the user saves/runs them.
pub fn created(id: &str, path: &str) -> Result<Value, RpcError> {
    let name = path_name(path)?;
    if id.is_empty() { return Err(RpcError::new("invalid", "程序标识无效")); }
    let executable = path.to_ascii_lowercase().ends_with(".exe");
    Ok(json!({"id":id,"name":name,"exePath":path,"args":"","runOnStartup":true,"collectTrayIcon":false,
        "launchViaLogonTask":false,"launchHiddenInBackground":!executable,
        "trayBehavior":{"autoMinimizeAndHideOnLaunch":executable,"closeDelaySeconds":0},
        "schedule":{"enabled":false,"startDelayMinutes":0,"autoExitMinutes":30,"frequencyEnabled":false,"frequencyDays":1,"frequencyRuns":1}}))
}

/// A new target retains identity and scheduling but cannot inherit collected
/// icons; script targets also cannot retain executable-only launch modes.
pub fn replaced(program: &Value, path: &str) -> Result<Value, RpcError> {
    let name = path_name(path)?;
    if !program.is_object() { return Err(RpcError::new("invalid", "程序设置格式无效")); }
    if program["exePath"].as_str().is_some_and(|old| old.replace('/',"\\").eq_ignore_ascii_case(&path.replace('/',"\\"))) { return Ok(program.clone()); }
    let mut next = program.clone();
    next["exePath"] = json!(path); next["name"] = json!(name); next["collectTrayIcon"] = json!(false);
    if !path.to_ascii_lowercase().ends_with(".exe") {
        next["launchViaLogonTask"] = json!(false); next["launchHiddenInBackground"] = json!(true);
        next["trayBehavior"]["autoMinimizeAndHideOnLaunch"] = json!(false);
    }
    Ok(next)
}

/// Reordering changes only sequence; stable IDs retain unsaved fields and selection.
pub fn reordered(settings: &Value, id: &str, position: usize) -> Result<Value, RpcError> {
    let mut items = settings["managedApps"].as_array().cloned().ok_or_else(|| RpcError::new("invalid", "程序列表格式无效"))?;
    let source = items.iter().position(|p| p["id"] == id).ok_or_else(|| RpcError::new("missing", "程序已移除，请重新选择"))?;
    if position >= items.len() { return Err(RpcError::new("invalid", "排序位置无效")); }
    let program = items.remove(source);
    items.insert(position, program);
    Ok(Value::Array(items))
}

pub fn field(settings: &Value, id: &str, path: &str) -> Value {
    let Some(program) = settings["managedApps"].as_array().and_then(|items| items.iter().find(|p| p["id"] == id)) else { return Value::Null; };
    if path == "mode" {
        return json!(if program["launchViaLogonTask"] == true { "task" }
            else if program["launchHiddenInBackground"] == true { "hidden" }
            else if program["trayBehavior"]["autoMinimizeAndHideOnLaunch"] == true { "tray" } else { "normal" });
    }
    path.split('.').fold(program, |value, key| &value[key]).clone()
}

/// Return only the new program array; the caller retains the surrounding draft,
/// revision and unknown settings. Missing IDs/fields never create partial records.
pub fn updated(settings: &Value, id: &str, path: &str, value: Value) -> Result<Value, RpcError> {
    let mut programs = settings["managedApps"].as_array().cloned().ok_or_else(|| RpcError::new("invalid", "程序列表格式无效"))?;
    let program = programs.iter_mut().find(|p| p["id"] == id).ok_or_else(|| RpcError::new("missing", "程序已移除，请重新选择"))?;
    if path == "mode" {
        let mode = value.as_str().filter(|v| matches!(*v, "task" | "hidden" | "tray" | "normal"))
            .ok_or_else(|| RpcError::new("invalid", "启动方式无效"))?;
        let executable = program["exePath"].as_str().is_some_and(|s| s.to_ascii_lowercase().ends_with(".exe"));
        if !executable && mode != "hidden" { return Err(RpcError::new("invalid", "脚本仅支持后台启动")); }
        program["launchViaLogonTask"] = json!(mode == "task");
        program["launchHiddenInBackground"] = json!(mode == "hidden");
        program["trayBehavior"]["autoMinimizeAndHideOnLaunch"] = json!(mode == "tray");
    } else {
        let pointer = format!("/{}", path.replace('.', "/"));
        if matches!(path, "id" | "exePath") { return Err(RpcError::new("invalid", "请使用更换程序操作")); }
        let target = program.pointer_mut(&pointer).ok_or_else(|| RpcError::new("invalid", "程序字段不存在"))?;
        *target = value;
    }
    Ok(Value::Array(programs))
}
