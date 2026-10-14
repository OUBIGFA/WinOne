use serde_json::Value;
use winone_desktop::{protocol::RpcError,transport::Client};
/// Assemble a profile preview from the Host's chunked preview store. The
/// descriptor digest binds the assembled bytes; the token is always released
/// and a release failure never masks the preview result (the Host TTL reclaims).
pub fn receive(client: &Client, path: &str, bindings: &Value) -> Result<Value, RpcError> {
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(30);
    let descriptor = client.call("profiles.import.preview.start", serde_json::json!({"path":path, "bindings":bindings}))?;
    let token = descriptor["token"].as_str().unwrap_or_default().to_string();
    let size = descriptor["size"].as_u64().unwrap_or_default() as usize;
    let digest = descriptor["sha256"].as_str().unwrap_or_default().to_string();
    let preview = (|| {
        if token.len() != 32 || size == 0 || size > 64 << 20 || digest.len() != 64 {
            return Err(RpcError::new("protocol", "配置预览传输信息无效"));
        }
        let mut data = Vec::with_capacity(size);
        while data.len() < size {
            if std::time::Instant::now() >= deadline { return Err(RpcError::new("timeout", "配置预览超时，请重试")); }
            let chunk = client.call("profiles.import.preview.read", serde_json::json!({"token":token, "offset":data.len()}))?;
            let bytes = chunk["data"].as_str().and_then(crate::icon::base64_decode)
                .ok_or_else(|| RpcError::new("protocol", "配置预览分块无效"))?;
            if chunk["token"].as_str() != Some(token.as_str()) || chunk["offset"].as_u64() != Some(data.len() as u64)
                || bytes.is_empty() || bytes.len() > 128 << 10 || data.len() + bytes.len() > size {
                return Err(RpcError::new("protocol", "配置预览分块不匹配"));
            }
            data.extend_from_slice(&bytes);
        }
        if crate::icon::sha256_hex(&data) != digest { return Err(RpcError::new("protocol", "配置预览校验失败")); }
        serde_json::from_slice(&data).map_err(|_| RpcError::new("protocol", "配置预览格式无效"))
    })();
    if let Err(error) = client.call("profiles.import.preview.release", serde_json::json!({"token":token})) {
        eprintln!("preview cleanup: {error}");
    }
    preview
}

