import { useEffect, useState } from "react";
import { pickProgram, wintrayCall } from "../bridge";

interface Program {
  id: string;
  name: string;
  exePath: string;
  args: string;
  runOnStartup: boolean;
  collectTrayIcon: boolean;
  launchViaLogonTask: boolean;
  launchHiddenInBackground: boolean;
  trayBehavior: {
    autoMinimizeAndHideOnLaunch: boolean;
    closeDelaySeconds: number;
  };
  schedule: {
    enabled: boolean;
    startDelayMinutes: number;
    autoExitMinutes: number;
    frequencyEnabled: boolean;
    frequencyDays: number;
    frequencyRuns: number;
  };
}
interface TraySettings {
  schemaVersion: number;
  language: string;
  runAtLogon: boolean;
  startMinimizedToTray: boolean;
  exitAfterManagedAppsCompleted: boolean;
  closeWindowRetrySeconds: number;
  startupIntervalSeconds: number;
  managedApps: Program[];
}
interface TraySnapshot {
  revision: string;
  settings: TraySettings;
  recoveryError?: string;
  backgroundError?: string;
}
interface Job {
  id: string;
  state: string;
  error?: string;
  result?: unknown;
}

function NumberField({
  label,
  value,
  min = 0,
  max,
  onChange,
}: {
  label: string;
  value: number;
  min?: number;
  max: number;
  onChange: (n: number) => void;
}) {
  return (
    <label className="number-field">
      {label}
      <input
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
      />
    </label>
  );
}

export function WinTrayPage() {
  const [saved, setSaved] = useState<TraySnapshot>();
  const [draft, setDraft] = useState<TraySettings>();
  const [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [dragged, setDragged] = useState("");
  async function reload(preserveDraft = false) {
    if (!preserveDraft) setError("");
    try {
      const next = await wintrayCall<TraySnapshot>("snapshot");
      setSaved((current) =>
        preserveDraft && current
          ? {
              ...current,
              recoveryError: next.recoveryError,
              backgroundError: next.backgroundError,
            }
          : next,
      );
      if (!preserveDraft) setDraft(next.settings);
      if (next.recoveryError || next.backgroundError)
        setError(next.recoveryError || next.backgroundError || "");
    } catch (e) {
      setError(String(e));
    }
  }
  useEffect(() => {
    void reload();
  }, []);
  const apps = draft?.managedApps ?? [];
  const active = apps.find((a) => a.id === selected);
  const dirty =
    !!saved && JSON.stringify(saved.settings) !== JSON.stringify(draft);
  function update(patch: Partial<Program>) {
    setDraft((current) =>
      current
        ? {
            ...current,
            managedApps: current.managedApps.map((a) =>
              a.id === selected ? { ...a, ...patch } : a,
            ),
          }
        : current,
    );
  }
  function reorder(from: string, to: string) {
    if (from === to) return;
    setDraft((current) => {
      if (!current) return current;
      const next = [...current.managedApps];
      const source = next.findIndex((a) => a.id === from),
        target = next.findIndex((a) => a.id === to);
      if (source < 0 || target < 0) return current;
      next.splice(target, 0, ...next.splice(source, 1));
      return { ...current, managedApps: next };
    });
  }
  async function add() {
    setError("");
    try {
      const path = await pickProgram();
      if (!path) return;
      const exe = /\.exe$/i.test(path);
      const entry: Program = {
        id: crypto.randomUUID(),
        name: path
          .split(/[\\/]/)
          .pop()!
          .replace(/\.[^.]+$/, ""),
        exePath: path,
        args: "",
        runOnStartup: true,
        collectTrayIcon: false,
        launchViaLogonTask: false,
        launchHiddenInBackground: !exe,
        trayBehavior: {
          autoMinimizeAndHideOnLaunch: exe,
          closeDelaySeconds: 0,
        },
        schedule: {
          enabled: false,
          startDelayMinutes: 0,
          autoExitMinutes: 30,
          frequencyEnabled: false,
          frequencyDays: 1,
          frequencyRuns: 1,
        },
      };
      setDraft((s) =>
        s ? { ...s, managedApps: [...s.managedApps, entry] } : s,
      );
      setSelected(entry.id);
    } catch (e) {
      setError(String(e));
    }
  }
  async function runJob(method: string, params: unknown) {
    setBusy(true);
    setError("");
    setMessage("正在处理");
    try {
      let job = await wintrayCall<Job>(method, params);
      const deadline = Date.now() + 5 * 60_000;
      while (job.state === "running") {
        if (Date.now() > deadline)
          throw new Error("任务仍在处理中，请稍后刷新状态，不要重复提交");
        await new Promise((resolve) => setTimeout(resolve, 250));
        job = await wintrayCall<Job>("jobs.get", { id: job.id });
      }
      if (job.state !== "completed") throw new Error(job.error ?? "操作失败");
      setMessage(
        method === "launch"
          ? "程序已启动"
          : method === "recover"
            ? "设置已恢复"
            : "已保存",
      );
      await reload();
    } catch (e) {
      setError(String(e));
      setMessage("");
      await reload(true);
    } finally {
      setBusy(false);
    }
  }
  if (!draft)
    return (
      <section>
        <p role="alert">{error || "正在连接程序管理模块"}</p>
        <button onClick={() => void reload()}>重新连接</button>
      </section>
    );
  const mode = active?.launchViaLogonTask
    ? "task"
    : active?.launchHiddenInBackground
      ? "hidden"
      : active?.trayBehavior.autoMinimizeAndHideOnLaunch
        ? "tray"
        : "normal";
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
      <div className="module-toolbar">
        <button
          disabled={busy || !!saved?.recoveryError}
          onClick={() => void add()}
        >
          添加程序
        </button>
        <span role="status">
          {busy ? message : dirty ? "有未保存的修改" : message}
        </span>
        <button
          disabled={busy || !dirty || !!saved?.recoveryError}
          onClick={() =>
            void runJob("settings.save", {
              revision: saved!.revision,
              settings: draft,
            })
          }
        >
          保存修改
        </button>
      </div>
      {saved?.recoveryError && (
        <button disabled={busy} onClick={() => void runJob("recover", null)}>
          恢复上次设置
        </button>
      )}
      <fieldset
        disabled={busy || !!saved?.recoveryError}
        className="plain-fieldset"
      >
        <label className="check-row">
          <input
            type="checkbox"
            checked={draft.runAtLogon}
            onChange={(e) =>
              setDraft({ ...draft, runAtLogon: e.target.checked })
            }
          />
          登录时运行 WinOne 并启动已启用的程序
        </label>
        <div className="program-layout">
          <div className="program-list" aria-label="程序列表">
            {apps.length === 0 ? (
              <p className="muted">还没有程序，添加后可设置启动方式</p>
            ) : (
              apps.map((a, index) => (
                <div
                  className="program-row"
                  key={a.id}
                  draggable
                  onDragStart={() => setDragged(a.id)}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={() => reorder(dragged, a.id)}
                >
                  <button
                    className={selected === a.id ? "selected" : ""}
                    onClick={() => setSelected(a.id)}
                  >
                    {a.name}
                    <small>{a.runOnStartup ? "已启用" : "已暂停"}</small>
                  </button>
                  <button
                    aria-label={`上移 ${a.name}`}
                    disabled={index === 0}
                    onClick={() => reorder(a.id, apps[index - 1].id)}
                  >
                    ↑
                  </button>
                  <button
                    aria-label={`下移 ${a.name}`}
                    disabled={index === apps.length - 1}
                    onClick={() => reorder(a.id, apps[index + 1].id)}
                  >
                    ↓
                  </button>
                </div>
              ))
            )}
          </div>
          {active ? (
            <div className="program-detail">
              <label>
                名称
                <input
                  value={active.name}
                  onChange={(e) => update({ name: e.target.value })}
                />
              </label>
              <p className="program-path">{active.exePath}</p>
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={active.runOnStartup}
                  onChange={(e) => update({ runOnStartup: e.target.checked })}
                />
                开机启动此程序
              </label>
              <label>
                启动方式
                <select
                  value={mode}
                  onChange={(e) =>
                    update({
                      launchViaLogonTask: e.target.value === "task",
                      launchHiddenInBackground: e.target.value === "hidden",
                      trayBehavior: {
                        ...active.trayBehavior,
                        autoMinimizeAndHideOnLaunch: e.target.value === "tray",
                      },
                    })
                  }
                >
                  {/\.exe$/i.test(active.exePath) && (
                    <option value="tray">收进托盘</option>
                  )}
                  <option value="hidden">后台启动</option>
                  {/\.exe$/i.test(active.exePath) && (
                    <>
                      <option value="task">开机即启动</option>
                      <option value="normal">正常启动</option>
                    </>
                  )}
                </select>
              </label>
              {mode === "tray" && (
                <NumberField
                  label="收起前等待（秒）"
                  value={active.trayBehavior.closeDelaySeconds}
                  max={600}
                  onChange={(n) =>
                    update({
                      trayBehavior: {
                        ...active.trayBehavior,
                        closeDelaySeconds: n,
                      },
                    })
                  }
                />
              )}
              <label>
                启动参数
                <input
                  value={active.args}
                  onChange={(e) => update({ args: e.target.value })}
                />
              </label>
              {/\.exe$/i.test(active.exePath) && (
                <label className="check-row">
                  <input
                    type="checkbox"
                    checked={active.collectTrayIcon}
                    onChange={(e) =>
                      update({ collectTrayIcon: e.target.checked })
                    }
                  />
                  托盘收纳
                </label>
              )}
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={active.schedule.enabled}
                  onChange={(e) =>
                    update({
                      schedule: {
                        ...active.schedule,
                        enabled: e.target.checked,
                      },
                    })
                  }
                />
                自定义时间
              </label>
              {active.schedule.enabled && (
                <div className="time-fields">
                  <NumberField
                    label="延后启动（分钟）"
                    value={active.schedule.startDelayMinutes}
                    max={1440}
                    onChange={(n) =>
                      update({
                        schedule: { ...active.schedule, startDelayMinutes: n },
                      })
                    }
                  />
                  <NumberField
                    label="自动退出（分钟，0 为不退出）"
                    value={active.schedule.autoExitMinutes}
                    max={1440}
                    onChange={(n) =>
                      update({
                        schedule: { ...active.schedule, autoExitMinutes: n },
                      })
                    }
                  />
                  <label className="check-row">
                    <input
                      type="checkbox"
                      checked={active.schedule.frequencyEnabled}
                      onChange={(e) =>
                        update({
                          schedule: {
                            ...active.schedule,
                            frequencyEnabled: e.target.checked,
                          },
                        })
                      }
                    />
                    限制自动运行次数
                  </label>
                  {active.schedule.frequencyEnabled && (
                    <>
                      <NumberField
                        label="最近天数"
                        value={active.schedule.frequencyDays}
                        min={1}
                        max={365}
                        onChange={(n) =>
                          update({
                            schedule: { ...active.schedule, frequencyDays: n },
                          })
                        }
                      />
                      <NumberField
                        label="最多运行次数"
                        value={active.schedule.frequencyRuns}
                        min={1}
                        max={1000}
                        onChange={(n) =>
                          update({
                            schedule: { ...active.schedule, frequencyRuns: n },
                          })
                        }
                      />
                    </>
                  )}
                </div>
              )}
              <div className="detail-actions">
                <button
                  disabled={dirty}
                  onClick={() => void runJob("launch", { id: active.id })}
                >
                  立即启动
                </button>
                <button
                  onClick={() => {
                    setDraft({
                      ...draft,
                      managedApps: apps.filter((a) => a.id !== active.id),
                    });
                    setSelected("");
                  }}
                >
                  移除程序
                </button>
              </div>
            </div>
          ) : (
            <div className="program-detail muted">选择一个程序编辑设置</div>
          )}
        </div>
        <h2>启动时间</h2>
        <div className="time-fields">
          <NumberField
            label="程序之间的启动间隔（秒）"
            value={draft.startupIntervalSeconds}
            max={120}
            onChange={(n) => setDraft({ ...draft, startupIntervalSeconds: n })}
          />
          <NumberField
            label="最长等待程序窗口（秒）"
            value={draft.closeWindowRetrySeconds}
            max={120}
            onChange={(n) => setDraft({ ...draft, closeWindowRetrySeconds: n })}
          />
        </div>
      </fieldset>
    </section>
  );
}
