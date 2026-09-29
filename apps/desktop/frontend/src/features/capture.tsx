import { useEffect, useState } from "react";
import { captureCall, pickDirectory, reveal } from "../bridge";

interface CaptureSettings {
  schemaVersion: number;
  revision: number;
  outputDirectory: string;
  copyToClipboard: boolean;
  askWhereToSave: boolean;
  openEditor: boolean;
  pinAfterCapture: boolean;
  theme: string;
  scrollDelay: number;
  scrollFromTop: boolean;
}
interface CaptureOutput {
  path: string;
  width: number;
  height: number;
  copied: boolean;
  pinned: boolean;
}
interface Snapshot {
  settings: CaptureSettings;
  lastCapture?: CaptureOutput;
  busy: boolean;
  pins: number;
}
interface Job {
  id: string;
  state: string;
  error?: string;
}

export function CapturePage() {
  const [state, setState] = useState<Snapshot>();
  const [draft, setDraft] = useState<CaptureSettings>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  async function reload(preserveDraft = false) {
    if (!preserveDraft) setError("");
    try {
      const next = await captureCall<Snapshot>("snapshot");
      setState((current) =>
        preserveDraft && current
          ? { ...next, settings: current.settings }
          : next,
      );
      if (!preserveDraft) setDraft(next.settings);
    } catch (e) {
      setError(String(e));
    }
  }
  useEffect(() => {
    void reload();
  }, []);
  async function act(method: string, params: unknown = null) {
    const hidden = method === "capture.start" || method === "edit.last";
    setBusy(true);
    setError("");
    setMessage("正在处理");
    try {
      let job = await captureCall<Job>(method, params);
      while (job.state === "running") {
        await new Promise((resolve) => setTimeout(resolve, 300));
        job = await captureCall<Job>("jobs.get", { id: job.id });
      }
      if (job.state === "failed") throw new Error(job.error || "操作失败");
      if (job.state !== "completed" && job.state !== "cancelled")
        throw new Error("无法确认操作结果，请刷新状态");
      setMessage(job.state === "cancelled" ? "已取消" : "操作完成");
      await reload(JSON.stringify(draft) !== JSON.stringify(state?.settings));
    } catch (e) {
      setError(String(e));
      setMessage("");
      await reload(true);
    } finally {
      setBusy(false);
      if (hidden) await reveal();
    }
  }
  async function save() {
    if (!draft) return;
    setBusy(true);
    setError("");
    try {
      const next = await captureCall<CaptureSettings>("settings.save", draft);
      setDraft(next);
      setState((s) => (s ? { ...s, settings: next } : s));
      setMessage("已保存");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function browse() {
    try {
      const path = await pickDirectory();
      if (path) setDraft((s) => (s ? { ...s, outputDirectory: path } : s));
    } catch (e) {
      setError(String(e));
    }
  }
  if (!draft)
    return (
      <section>
        <p role="alert">{error || "正在启动截图组件"}</p>
        <button onClick={() => void reload()}>重新连接</button>
      </section>
    );
  const dirty = JSON.stringify(draft) !== JSON.stringify(state?.settings);
  return (
    <section>
      {error && (
        <div className="error" role="alert">
          {error}
          <div>
            <button disabled={busy} onClick={() => void reload()}>
              放弃修改并刷新
            </button>
          </div>
        </div>
      )}
      <div className="capture-actions">
        {[
          ["region", "区域截图"],
          ["window", "窗口截图"],
          ["fullscreen", "全屏截图"],
          ["scroll", "滚动截图"],
        ].map(([mode, title]) => (
          <button
            key={mode}
            disabled={busy || dirty}
            onClick={() => void act("capture.start", { mode })}
          >
            {title}
          </button>
        ))}
      </div>
      <p className="muted" role="status">
        {dirty ? "请先保存设置" : message || "区域截图支持标注，按 Esc 取消"}
      </p>
      {state?.lastCapture && (
        <section className="last-capture">
          <h2>最近截图</h2>
          <p className="program-path">{state.lastCapture.path}</p>
          <p className="muted">
            {state.lastCapture.width} × {state.lastCapture.height}
          </p>
          <div className="capture-actions">
            <button disabled={busy} onClick={() => void act("copy.last")}>
              复制
            </button>
            <button
              disabled={busy || dirty}
              onClick={() => void act("edit.last")}
            >
              编辑
            </button>
            <button disabled={busy} onClick={() => void act("pin.last")}>
              贴到屏幕
            </button>
            <button disabled={busy} onClick={() => void act("pins.close")}>
              关闭全部贴图
            </button>
          </div>
        </section>
      )}
      <h2>截图后</h2>
      <fieldset className="plain-fieldset" disabled={busy}>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.copyToClipboard}
            onChange={(e) =>
              setDraft({ ...draft, copyToClipboard: e.target.checked })
            }
          />
          复制到剪贴板
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.openEditor}
            onChange={(e) =>
              setDraft({ ...draft, openEditor: e.target.checked })
            }
          />
          打开编辑器
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.pinAfterCapture}
            onChange={(e) =>
              setDraft({ ...draft, pinAfterCapture: e.target.checked })
            }
          />
          贴到屏幕
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.askWhereToSave}
            onChange={(e) =>
              setDraft({ ...draft, askWhereToSave: e.target.checked })
            }
          />
          每次选择保存位置
        </label>
        <div className="setting-row">
          <div>
            <label htmlFor="capture-path">保存目录</label>
            <p className="program-path" id="capture-path">
              {draft.outputDirectory}
            </p>
          </div>
          <button onClick={() => void browse()}>选择目录</button>
        </div>
        <div className="setting-row">
          <label htmlFor="capture-theme">编辑器主题</label>
          <select
            id="capture-theme"
            value={draft.theme}
            onChange={(e) => setDraft({ ...draft, theme: e.target.value })}
          >
            <option value="system">跟随系统</option>
            <option value="light">浅色</option>
            <option value="dark">深色</option>
          </select>
        </div>
        <h2>滚动截图</h2>
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.scrollFromTop}
            onChange={(e) =>
              setDraft({ ...draft, scrollFromTop: e.target.checked })
            }
          />
          从窗口顶部开始
        </label>
        <label className="number-field">
          滚动间隔（毫秒）
          <input
            type="number"
            min={100}
            max={2000}
            value={draft.scrollDelay}
            onChange={(e) =>
              setDraft({ ...draft, scrollDelay: Number(e.target.value) })
            }
          />
        </label>
        <button disabled={!dirty} onClick={() => void save()}>
          保存设置
        </button>
      </fieldset>
    </section>
  );
}
