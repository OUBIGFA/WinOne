//! Length-prefixed JSON compatible with internal/protocol and internal/host.
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::io::{self, Read, Write};

const MAX_FRAME: usize = 1024 * 1024;

#[derive(Debug, PartialEq)]
pub enum ModuleJob { Running(String), Completed(Value), Cancelled }

/// Worker operation tickets are separate from Host configuration tickets. Never
/// accept a different ID or infer success from a missing/unknown state.
pub fn module_job(expected: Option<&str>, value: &Value) -> Result<ModuleJob, RpcError> {
    let id = value["id"].as_str().filter(|s| !s.is_empty());
    if expected.is_some() && expected != id { return Err(RpcError::new("protocol", "任务标识不匹配")); }
    match value["state"].as_str() {
        Some("running") => id.map(|id| ModuleJob::Running(id.into())).ok_or_else(|| RpcError::new("protocol", "任务缺少标识")),
        Some("completed") => Ok(ModuleJob::Completed(value["result"].clone())),
        Some("cancelled") => Ok(ModuleJob::Cancelled),
        Some("failed") => Err(RpcError::new("operation_failed",value["error"].as_str().unwrap_or("操作失败"))),
        _ => Err(RpcError::new("protocol", "无法确认任务状态")),
    }
}

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
pub struct RpcError {
    pub code: String,
    pub message: String,
}
impl RpcError {
    pub fn new(code: &str, message: impl Into<String>) -> Self {
        Self { code: code.into(), message: message.into() }
    }
}
impl std::fmt::Display for RpcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{} ({})", self.message, self.code)
    }
}
impl std::error::Error for RpcError {}
impl From<io::Error> for RpcError {
    fn from(error: io::Error) -> Self { Self::new("transport", error.to_string()) }
}
impl From<serde_json::Error> for RpcError {
    fn from(error: serde_json::Error) -> Self { Self::new("protocol", error.to_string()) }
}

pub fn read_frame(reader: &mut impl Read) -> io::Result<Vec<u8>> {
    let mut header = [0; 4];
    reader.read_exact(&mut header)?;
    let size = u32::from_le_bytes(header) as usize;
    // Check before allocating so a faulty peer cannot force unbounded memory.
    if size == 0 || size > MAX_FRAME {
        return Err(io::Error::new(io::ErrorKind::InvalidData, "无效的消息长度"));
    }
    let mut data = vec![0; size];
    reader.read_exact(&mut data)?;
    Ok(data)
}

pub fn write_frame(writer: &mut impl Write, data: &[u8]) -> io::Result<()> {
    if data.is_empty() || data.len() > MAX_FRAME {
        return Err(io::Error::new(io::ErrorKind::InvalidInput, "无效的消息长度"));
    }
    writer.write_all(&(data.len() as u32).to_le_bytes())?;
    writer.write_all(data)
}

pub fn decode_response(id: &str, bytes: &[u8]) -> Result<Value, RpcError> {
    let response: Value = serde_json::from_slice(bytes)?;
    let object = response.as_object().ok_or_else(|| RpcError::new("protocol", "后台响应格式无效"))?;
    if object.get("id").and_then(Value::as_str) != Some(id) {
        return Err(RpcError::new("protocol", "后台响应标识不匹配"));
    }
    // C# module errors carry result:null. The Go Host retains that raw null
    // while forwarding the error; preserve its reason instead of hiding it
    // behind a protocol error. Non-null result + error remains ambiguous.
    match (object.get("result"), object.get("error").filter(|value|!value.is_null())) {
        (Some(value), None) => Ok(value.clone()),
        (None|Some(Value::Null), Some(error)) => Err(serde_json::from_value(error.clone())?),
        _ => Err(RpcError::new("protocol", "后台响应缺少结果或包含冲突状态")),
    }
}
