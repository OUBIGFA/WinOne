import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  getSnapshot,
  saveSettings,
  type Snapshot,
  type Settings,
} from "./bridge";
import "./style.css";

const pages = [
  { id: "overview", title: "概览", glyph: "◫" },
  { id: "capture", title: "截图", glyph: "⌗" },
  { id: "always-on-top", title: "窗口置顶", glyph: "↟" },
  { id: "wintray", title: "启动与托盘", glyph: "▤" },
  { id: "settings", title: "设置", glyph: "⚙" },
];

function App() {
  const [page, setPage] = useState("overview");
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  async function refresh() {
    setBusy(true);
    setError("");
    try {
      setSnapshot(await getSnapshot());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void refresh();
  }, []);
  useEffect(() => {
    const query = matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme =
        snapshot?.settings.theme === "system" || !snapshot
          ? query.matches
            ? "dark"
            : "light"
          : snapshot.settings.theme;
    };
    apply();
    query.addEventListener("change", apply);
    return () => query.removeEventListener("change", apply);
  }, [snapshot?.settings.theme]);
  async function changeTheme(theme: Settings["theme"]) {
    if (!snapshot) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const settings = await saveSettings({ ...snapshot.settings, theme });
      setSnapshot({ ...snapshot, settings });
      setNotice("已保存");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  const selected = pages.find((p) => p.id === page)!;
  const module = snapshot?.modules.find((m) => m.id === page);
  return (
    <div className="shell">
      <aside>
        <div className="brand">
          <span className="mark">W</span>
          <span>WinOne</span>
        </div>
        <nav aria-label="功能导航">
          {pages.map((p) => (
            <button
              key={p.id}
              aria-current={page === p.id ? "page" : undefined}
              onClick={() => {
                setPage(p.id);
                setNotice("");
              }}
            >
              <span aria-hidden>{p.glyph}</span>
              {p.title}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          开发预览 <span>0.1</span>
        </div>
      </aside>
      <main>
        <header>
          <div>
            <p className="eyebrow">你的 Windows 工具集</p>
            <h1>{selected.title}</h1>
          </div>
          <button
            className="secondary"
            disabled={busy}
            onClick={() => void refresh()}
          >
            刷新状态
          </button>
        </header>
        {error && (
          <div className="error" role="alert">
            <strong>操作未完成</strong>
            <p>{error}</p>
            <button disabled={busy} onClick={() => void refresh()}>
              重试连接
            </button>
          </div>
        )}
        {page === "overview" && (
          <>
            <p className="intro">常用工具，一个入口</p>
            <div className="feature-list">
              {pages.slice(1, 4).map((p) => (
                <button
                  key={p.id}
                  className="feature"
                  onClick={() => setPage(p.id)}
                >
                  <span className="feature-icon" aria-hidden>
                    {p.glyph}
                  </span>
                  <span>
                    <strong>{p.title}</strong>
                    <small>
                      {snapshot?.modules.find((m) => m.id === p.id)?.detail ??
                        "等待后台连接"}
                    </small>
                  </span>
                  <span aria-hidden>→</span>
                </button>
              ))}
            </div>
            <section className="footnote">
              <h2>开发进度</h2>
              <p>
                当前可验证后台连接、设置保存和主题切换，功能模块接入后将在这里启用
              </p>
            </section>
          </>
        )}
        {page === "settings" && (
          <section>
            <h2>外观</h2>
            <div className="setting-row">
              <div>
                <label htmlFor="theme">主题</label>
                <p>选择浅色、深色或跟随 Windows</p>
              </div>
              <select
                id="theme"
                disabled={busy || !snapshot}
                value={snapshot?.settings.theme ?? "system"}
                onChange={(e) =>
                  void changeTheme(e.target.value as Settings["theme"])
                }
              >
                <option value="system">跟随系统</option>
                <option value="light">浅色</option>
                <option value="dark">深色</option>
              </select>
            </div>
            <p className="saved" role="status">
              {notice}
            </p>
            <h2>后台</h2>
            <div className="setting-row">
              <div>
                <span>连接状态</span>
                <p>关闭设置窗口后，后台继续运行</p>
              </div>
              <span>{snapshot && !error ? "已连接" : "未连接"}</span>
            </div>
          </section>
        )}
        {page !== "overview" && page !== "settings" && (
          <section className="empty">
            <span className="empty-icon" aria-hidden>
              {selected.glyph}
            </span>
            <h2>{module?.detail ?? "等待后台连接"}</h2>
            <p>组件通过验证后，将在此提供设置与操作</p>
            <button className="secondary" onClick={() => setPage("overview")}>
              返回概览
            </button>
          </section>
        )}
      </main>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
