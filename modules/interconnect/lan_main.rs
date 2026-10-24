//! Private stdio adapter for the unmodified FileShare transfer operations.
//! EOF/stop terminates this runtime, including any connection that failed to drain.
use std::path::PathBuf;
use serde::Deserialize;
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use winone_fileshare::{network, server};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Request { id: String, method: String, #[serde(default)] params: Value }

fn decode<T: serde::de::DeserializeOwned>(value: Value) -> Result<T, String> {
    serde_json::from_value(value).map_err(|error| error.to_string())
}

async fn handle(request: &Request) -> Result<Value, String> {
    let params = request.params.clone();
    match request.method.as_str() {
        "addresses" => Ok(json!(network::lan_ipv4_addresses().into_iter().map(|a| json!({"name":a.name,"ip":a.ip})).collect::<Vec<_>>())),
        "start" => Ok(json!(server::start(decode(params)?).await?)),
        "snapshot" => server::admin_snapshot().await,
        "add_files" => {
            let paths: Vec<PathBuf> = decode(params)?;
            if paths.len() > 200 { return Err("每次最多选择 200 个文件".into()); }
            Ok(json!(server::add_admin_local_files(paths).await?))
        }
        "add_text" => {
            let content: String = decode(params)?;
            if content.len() > 1024 * 1024 { return Err("文本最大为 1 MiB".into()); }
            server::add_admin_text(content).await?; Ok(Value::Null)
        }
        "remove" => { server::remove_admin_item(decode(params)?).await?; Ok(Value::Null) }
        "item_path" => Ok(json!(server::item_file_path(&decode::<String>(params)?).await?)),
        "download_name" => Ok(json!(server::download_filename(&decode::<String>(params)?).await?)),
        "save_as" => {
            #[derive(Deserialize)]
            #[serde(deny_unknown_fields)]
            struct Save { id: String, path: PathBuf, #[serde(default)] replace: bool }
            let save: Save = decode(params)?;
            if !save.path.is_absolute() { return Err("保存位置必须是绝对路径".into()); }
            if save.replace { server::copy_item_to_path(&save.id, &save.path).await?; }
            else { server::copy_item_to_new_path(&save.id, &save.path).await?; }
            Ok(Value::Null)
        }
        "stop" => { server::stop().await?; Ok(Value::Null) }
        _ => Err("不支持的共享操作".into()),
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut input = BufReader::new(tokio::io::stdin());
    let mut output = tokio::io::stdout();
    loop {
        // The only caller is the verified Host. Still bound malformed frames.
        let mut bytes = Vec::new();
        let length = (&mut input).take(2 * 1024 * 1024).read_until(b'\n', &mut bytes).await?;
        if length == 0 { break; }
        if bytes.last() != Some(&b'\n') { return Err("共享命令过长或不完整".into()); }
        let request: Request = serde_json::from_slice(&bytes)?;
        let response = match handle(&request).await {
            Ok(result) => json!({"id":request.id, "result":result}),
            Err(error) => json!({"id":request.id, "error":error}),
        };
        output.write_all(&serde_json::to_vec(&response)?).await?;
        output.write_all(b"\n").await?;
        output.flush().await?;
        if request.method == "stop" { break; }
    }
    server::stop().await.map_err(std::io::Error::other)?;
    Ok(())
}
